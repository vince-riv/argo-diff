#!/usr/bin/env bash
# Runs argo-diff as a standalone pod against the k3s test cluster, waits for it
# to terminate, and asserts its exit code (and optionally its logs).
#
# Required env vars (all providers):
#   POD_NAME_PREFIX       - prefix for the pod name (a timestamp suffix is appended)
#   IMAGE                 - full argo-diff image reference to run (e.g. argo-diff:e2e).
#                          The workflow builds this locally and side-loads it into
#                          k3d, so the pod uses imagePullPolicy: Never.
#   ARGO_DIFF_CONTEXT_STR - value for ARGO_DIFF_CONTEXT_STR (commit status / comment context)
#   EXPECT_EXIT           - "0" to require success, "nonzero" to require failure
#
# Required when PROVIDER=github (the default):
#   ARGO_DIFF_SHA         - commit sha argo-diff should diff against
#   ARGO_DIFF_HEAD_REF    - PR head branch
#   ARGO_DIFF_REPOSITORY  - owner/repo
#   GITHUB_TOKEN          - GitHub token for commenting/status
#   PR_REF                - GITHUB_REF value (e.g. refs/pull/N/merge)
#
# Required when PROVIDER=gitlab:
#   GITLAB_TOKEN          - access token with api scope
#   GITLAB_OWNER          - project namespace path (e.g. vrivellino)
#   GITLAB_REPO           - project name (e.g. argo-diff)
#   MR_IID                - merge request IID
#
# Note: the ARGO_DIFF_SHA/HEAD_REF/REPOSITORY inputs are deliberately not named
# GITHUB_SHA/GITHUB_HEAD_REF/GITHUB_REPOSITORY - those are GitHub Actions'
# reserved default environment variables, and a step's own `env:` block cannot
# override them (the runner silently keeps its own context value instead).
#
# Optional env vars:
#   PROVIDER              - "github" (default) or "gitlab"
#   REQUIRE_LOG           - substring that must appear in the pod logs

set -euo pipefail

PROVIDER="${PROVIDER:-github}"

: "${POD_NAME_PREFIX:?}" "${IMAGE:?}" "${ARGO_DIFF_CONTEXT_STR:?}" "${EXPECT_EXIT:?}"

case "$PROVIDER" in
  github)
    : "${ARGO_DIFF_SHA:?}" "${ARGO_DIFF_HEAD_REF:?}" "${ARGO_DIFF_REPOSITORY:?}" \
      "${GITHUB_TOKEN:?}" "${PR_REF:?}"
    ;;
  gitlab)
    : "${GITLAB_TOKEN:?}" "${GITLAB_OWNER:?}" "${GITLAB_REPO:?}" "${MR_IID:?}"
    ;;
  *)
    echo "::error::Unknown PROVIDER value: $PROVIDER (expected 'github' or 'gitlab')"
    exit 1
    ;;
esac

pod_name="${POD_NAME_PREFIX}-$(date +%s)"

# Provider-specific pieces of the pod spec, spliced into the manifest below.
#
# github: fakes the GitHub Actions environment.
# gitlab: GitLab CI detection does not exist yet, so argo-diff runs from an event
# file mounted from a ConfigMap. "refresh" makes it read the MR's sha and
# branches back from the API.
provider_args=""
provider_mounts=""
provider_volumes=""
case "$PROVIDER" in
  github)
    provider_env="        - name: GITHUB_ACTIONS
          value: \"true\"
        - name: GITHUB_TOKEN
          value: \"${GITHUB_TOKEN}\"
        - name: GITHUB_SHA
          value: \"${ARGO_DIFF_SHA}\"
        - name: GITHUB_REF
          value: \"${PR_REF}\"
        - name: GITHUB_EVENT_NAME
          value: \"pull_request\"
        - name: GITHUB_HEAD_REF
          value: \"${ARGO_DIFF_HEAD_REF}\"
        - name: GITHUB_BASE_REF
          value: \"k3s-test\"
        - name: GITHUB_REPOSITORY
          value: \"${ARGO_DIFF_REPOSITORY}\""
    ;;
  gitlab)
    event_json=$(printf '{"provider":"gitlab","owner":"%s","repo":"%s","default_ref":"k3s-test","pr":%s,"refresh":true}' \
      "$GITLAB_OWNER" "$GITLAB_REPO" "$MR_IID")
    kubectl -n argocd create configmap "${pod_name}-event" --from-literal=event.json="$event_json"
    provider_args='      args: ["-f", "/event/event.json"]'
    provider_env="        - name: ARGO_DIFF_SCM_PROVIDERS
          value: \"gitlab\"
        - name: GITLAB_TOKEN
          value: \"${GITLAB_TOKEN}\""
    provider_mounts="      volumeMounts:
        - name: event
          mountPath: /event"
    provider_volumes="  volumes:
    - name: event
      configMap:
        name: ${pod_name}-event"
    ;;
esac

cat <<EOF | kubectl -n argocd apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: $pod_name
spec:
  restartPolicy: Never
${provider_volumes}
  containers:
    - name: argo-diff
      command:
        - /app/argo-diff
${provider_args}
      image: ${IMAGE}
      imagePullPolicy: Never
${provider_mounts}
      env:
        - name: ARGO_DIFF_COMMENT_PREAMBLE
          value: |
            ## Argo-Diff - Ephemeral Environment Test
        - name: ARGO_DIFF_COMMENT_NOTICE
          value: "This argo-diff should have the same output every run"
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
${provider_env}
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

# Read the logs once, up front: the REQUIRE_LOG assertion below matches against
# this copy rather than re-running kubectl. Piping `kubectl logs` straight into
# `grep -q` is a trap under `set -o pipefail` - grep exits at the first match,
# kubectl dies of SIGPIPE writing whatever is left (exit 141, and silently, as
# Go's runtime default for EPIPE on stdout), and pipefail turns that into a
# failed assertion even though the string was present.
pod_logs=$(kubectl -n argocd logs "$pod_name" 2>&1 || true)
echo "--- Pod logs ($pod_name) ---"
printf '%s\n' "$pod_logs"

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
  if ! grep -qF -- "$REQUIRE_LOG" <<<"$pod_logs"; then
    echo "::error::Expected pod $pod_name logs to contain: $REQUIRE_LOG"
    exit 1
  fi
fi

echo "OK: pod $pod_name exited as expected (EXPECT_EXIT=$EXPECT_EXIT)"
