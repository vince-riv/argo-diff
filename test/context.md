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

Required env: `POD_NAME_PREFIX`, `IMAGE`, `ARGO_DIFF_CONTEXT_STR`, `ARGO_DIFF_SHA`,
`ARGO_DIFF_HEAD_REF`, `ARGO_DIFF_REPOSITORY`, `GITHUB_TOKEN`, `PR_REF`, `EXPECT_EXIT`
(`0` or `nonzero`). Optional `REQUIRE_LOG` asserts a substring appears in the pod logs.

`IMAGE` is a full image reference (`argo-diff:e2e`). `k3s.yml` builds it from the PR's own source
and side-loads it into k3d, so the pod runs with `imagePullPolicy: Never` and there is no registry
dependency.

The `ARGO_DIFF_*` names exist because `GITHUB_SHA`, `GITHUB_HEAD_REF`, and `GITHUB_REPOSITORY` are
GitHub Actions' reserved defaults — a step's own `env:` block cannot override them, so the script
takes them under different names and passes them into the pod under the real names.

The pod sets `ARGO_DIFF_SKIP_REF_CHECK=true` (the target revisions don't match a real PR base),
`GITHUB_ACTIONS=true` (one-shot mode, commit statuses skipped, exit code is the verdict), and a
dummy `ARGOCD_AUTH_TOKEN` since the test ArgoCD has auth disabled. It waits for a terminal pod
phase, prints the logs, and compares the container's exit code against `EXPECT_EXIT`.

## Workflow shape

`k3s.yml` has two triggers:

- **`pull_request`** — same-repo branches only, on paths that affect the test: `test/**`, the
  workflow itself, `charts/test-basic-deployment/**`, any `.go` file, `go.mod`/`go.sum`, `Dockerfile`.
  The job `if:` skips fork PRs, because a fork PR gets a read-only `GITHUB_TOKEN` and cannot post the
  argo-diff comment.
- **`workflow_dispatch`** with a required `pr` input — the manual path for **fork PRs**. A maintainer
  runs it against a PR number; the run gets a normal write token. `github.event.workflow_run` was
  never usable for forks (its `pull_requests` array is always empty), so there is no automatic fork
  path. Note a dispatch runs the workflow file from its target ref (usually `main`), so `k3s.yml`
  changes in a fork PR are not exercised by the dispatch.

  **Security:** a dispatch checks out and runs PR-authored code (`go build`, `docker build`,
  `run-argo-diff-pod.sh`) with the job's `pull-requests: write` token. Review the fork's diff before
  dispatching. The token has no `contents` / `packages` write, which bounds the damage.

The `pr_info` step resolves the PR number, head/base refs, and head sha — from the event payload for
`pull_request`, or via `gh pr view` for `workflow_dispatch` (which also normalizes `#123` / a PR URL
to the integer). Checkout uses the resolved **head sha**, not `refs/pull/<n>/merge`: the merge ref is
absent on a conflicted PR and can move mid-run, and the real diff is computed against the live PR via
the API anyway.

The image is built in-workflow: `go build` a linux/amd64 binary into `temp/`, `docker build` the
`Dockerfile`, then `k3d image import` into the cluster. Nothing is pushed to a registry, so the test
no longer depends on the `Docker build` workflow.

It runs two scenarios: the healthy apps (`EXPECT_EXIT=0`), then — after deleting `meta` (finalizer
removed first, since its `selfHeal` would resurrect its children) and applying the broken app — the
failure case (`EXPECT_EXIT=nonzero`, `REQUIRE_LOG="Failed to diff application broken-deployment"`).

Both runs comment on the PR, so their output is visible on the PR under test.
