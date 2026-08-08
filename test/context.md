# test/

Fixtures and the runner for the **end-to-end test** in `.github/workflows/k3s.yml`: a k3s cluster
with a real ArgoCD, real applications, and argo-diff running as a pod against them. Nothing here is
used by `go test`.

## Layout

| Path | Purpose |
| ---- | ------- |
| `argocd-helmchart.yaml` | k3s `HelmChart` CR that installs ArgoCD (auth disabled, single replicas, no dex/redis-ha) |
| `argocd-applications/` | The `test` AppProject plus the `basic-deployment` and `helm-deployment` Applications, assembled by `kustomization.yaml` |
| `argocd-meta.yaml` | The `meta` AppProject and app-of-apps Application that owns `argocd-applications/` |
| `argocd-broken-app.yaml` | An Application pointing at `test/does-not-exist`, used to prove argo-diff exits non-zero on manifest-generation failures |
| `basic-deployment/` | Plain Kustomize manifests synced by `basic-deployment` |
| `helm-deployment.values.yaml` | Values consumed by the multi-source `helm-deployment` app |
| `run-argo-diff-pod.sh` | Runs argo-diff as a pod and asserts its exit code and logs |

`helm-deployment` is deliberately **multi-source**: the chart comes from
`charts/test-basic-deployment` in this repo and the values from `test/helm-deployment.values.yaml`,
which is what exercises the `--revisions` / `--source-positions` code path in `internal/argocd`.

## The `k3s-test` branch

The applications pin `targetRevision: k3s-test` (and `run-argo-diff-pod.sh` sets
`GITHUB_BASE_REF` / `REPO_DEFAULT_REF` to `k3s-test`) so the cluster syncs a stable branch while the
PR under test supplies the *changed* revision. Editing `test/basic-deployment/` or the fixture chart
on a PR is what produces a diff — but the live state comes from `k3s-test`, so a fixture change that
should show up in the e2e diff has to be reflected there too.

## run-argo-diff-pod.sh

Required env: `POD_NAME_PREFIX`, `IMAGE_TAG`, `ARGO_DIFF_CONTEXT_STR`, `ARGO_DIFF_SHA`,
`ARGO_DIFF_HEAD_REF`, `ARGO_DIFF_REPOSITORY`, `GITHUB_TOKEN`, `PR_REF`, `EXPECT_EXIT`
(`0` or `nonzero`). Optional `REQUIRE_LOG` asserts a substring appears in the pod logs.

The `ARGO_DIFF_*` names exist because `GITHUB_SHA`, `GITHUB_HEAD_REF`, and `GITHUB_REPOSITORY` are
GitHub Actions' reserved defaults — a step's own `env:` block cannot override them, so the script
takes them under different names and passes them into the pod under the real names.

The pod sets `ARGO_DIFF_SKIP_REF_CHECK=true` (the target revisions don't match a real PR base),
`GITHUB_ACTIONS=true` (one-shot mode, commit statuses skipped, exit code is the verdict), and a
dummy `ARGOCD_AUTH_TOKEN` since the test ArgoCD has auth disabled. It waits for a terminal pod
phase, prints the logs, and compares the container's exit code against `EXPECT_EXIT`.

## Workflow shape

`k3s.yml` runs on PRs touching `test/**` or the workflow itself, and on a successful `Docker build`
`workflow_run` (so it can test the `pr-<n>` image built from that PR). It runs two scenarios:
the healthy apps (`EXPECT_EXIT=0`), then — after deleting `meta` (finalizer removed first, since its
`selfHeal` would resurrect its children) and applying the broken app — the failure case
(`EXPECT_EXIT=nonzero`, `REQUIRE_LOG="Failed to diff application broken-deployment"`).

Both runs comment on the PR, so their output is visible on the PR that triggered them.
