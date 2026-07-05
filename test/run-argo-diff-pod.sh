#!/usr/bin/env bash
# Runs argo-diff as a standalone pod against the k3s test cluster, waits for it
# to terminate, and asserts its exit code (and optionally its logs).
#
# Required env vars:
#   POD_NAME_PREFIX       - prefix for the pod name (a timestamp suffix is appended)
#   IMAGE_TAG             - argo-diff image tag to run
#   ARGO_DIFF_CONTEXT_STR - value for ARGO_DIFF_CONTEXT_STR (commit status / comment context)
#   ARGO_DIFF_SHA         - commit sha argo-diff should diff against
#   ARGO_DIFF_HEAD_REF    - PR head branch
#   ARGO_DIFF_REPOSITORY  - owner/repo
#   GITHUB_TOKEN          - GitHub token for commenting/status
#   PR_REF                - GITHUB_REF value (e.g. refs/pull/N/merge)
#   EXPECT_EXIT           - "0" to require success, "nonzero" to require failure
#
# Note: the ARGO_DIFF_SHA/HEAD_REF/REPOSITORY inputs are deliberately not named
# GITHUB_SHA/GITHUB_HEAD_REF/GITHUB_REPOSITORY - those are GitHub Actions'
# reserved default environment variables, and a step's own `env:` block cannot
# override them (the runner silently keeps its own context value instead).
#
# Optional env vars:
#   REQUIRE_LOG           - substring that must appear in the pod logs

set -euo pipefail

: "${POD_NAME_PREFIX:?}" "${IMAGE_TAG:?}" "${ARGO_DIFF_CONTEXT_STR:?}" \
  "${ARGO_DIFF_SHA:?}" "${ARGO_DIFF_HEAD_REF:?}" "${ARGO_DIFF_REPOSITORY:?}" \
  "${GITHUB_TOKEN:?}" "${PR_REF:?}" "${EXPECT_EXIT:?}"

pod_name="${POD_NAME_PREFIX}-$(date +%s)"

cat <<EOF | kubectl -n argocd apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: $pod_name
spec:
  restartPolicy: Never
  containers:
    - name: argo-diff
      command:
        - /app/argo-diff
      image: ghcr.io/vince-riv/argo-diff:${IMAGE_TAG}
      env:
        - name: ARGO_DIFF_COMMENT_PREAMBLE
          value: |
            ## Argo-Diff - Ephemeral Environment Test
            This argo-diff should have the same output every run
        - name: ARGO_DIFF_CONTEXT_STR
          value: "${ARGO_DIFF_CONTEXT_STR}"
        - name: ARGO_DIFF_SKIP_REF_CHECK
          value: "true"
        - name: ARGOCD_AUTH_TOKEN
          value: dummy_value
        - name: ARGOCD_SERVER_ADDR
          value: argocd-server.argocd.svc.cluster.local:80
        - name: ARGOCD_SERVER_INSECURE
          value: "true"
        - name: ARGOCD_SERVER_PLAINTEXT
          value: "true"
        - name: GITHUB_ACTIONS
          value: "true"
        - name: GITHUB_TOKEN
          value: "${GITHUB_TOKEN}"
        - name: GITHUB_SHA
          value: "${ARGO_DIFF_SHA}"
        - name: GITHUB_REF
          value: "${PR_REF}"
        - name: GITHUB_EVENT_NAME
          value: "pull_request"
        - name: GITHUB_HEAD_REF
          value: "${ARGO_DIFF_HEAD_REF}"
        - name: GITHUB_BASE_REF
          value: "k3s-test"
        - name: GITHUB_REPOSITORY
          value: "${ARGO_DIFF_REPOSITORY}"
        - name: REPO_DEFAULT_REF
          value: "k3s-test"
        - name: LOG_LEVEL
          value: "debug"
EOF

echo "Waiting for pod $pod_name to reach a terminal phase..."
phase=""
for _ in $(seq 1 60); do
  phase=$(kubectl -n argocd get pod "$pod_name" -o jsonpath='{.status.phase}' 2>/dev/null || true)
  if [[ "$phase" == "Succeeded" || "$phase" == "Failed" ]]; then
    break
  fi
  sleep 1
done

echo "--- Pod logs ($pod_name) ---"
kubectl -n argocd logs "$pod_name" || true

if [[ "$phase" != "Succeeded" && "$phase" != "Failed" ]]; then
  echo "::error::Pod $pod_name did not reach a terminal phase within timeout (last phase: $phase)"
  exit 1
fi

exit_code=$(kubectl -n argocd get pod "$pod_name" -o jsonpath='{.status.containerStatuses[0].state.terminated.exitCode}')
echo "Pod $pod_name terminated with exit code $exit_code"

case "$EXPECT_EXIT" in
  0)
    if [[ "$exit_code" != "0" ]]; then
      echo "::error::Expected pod $pod_name to exit 0, but it exited $exit_code"
      exit 1
    fi
    ;;
  nonzero)
    if [[ "$exit_code" == "0" ]]; then
      echo "::error::Expected pod $pod_name to exit non-zero, but it exited 0"
      exit 1
    fi
    ;;
  *)
    echo "::error::Unknown EXPECT_EXIT value: $EXPECT_EXIT (expected '0' or 'nonzero')"
    exit 1
    ;;
esac

if [[ -n "${REQUIRE_LOG:-}" ]]; then
  if ! kubectl -n argocd logs "$pod_name" | grep -qF -- "$REQUIRE_LOG"; then
    echo "::error::Expected pod $pod_name logs to contain: $REQUIRE_LOG"
    exit 1
  fi
fi

echo "OK: pod $pod_name exited as expected (EXPECT_EXIT=$EXPECT_EXIT)"
