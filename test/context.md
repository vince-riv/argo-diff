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
| `argocd-meta-any-ns.yaml` | The `any-ns` AppProject and app-of-apps Application (`any-ns-meta`) that exercises ArgoCD's app-in-any-namespace feature |
| `argocd-applications-any-ns/` | The `any-ns-deployment` child Application, assembled by `kustomization.yaml` |
| `any-ns-deployment/` | Plain Kustomize manifests synced by the `any-ns-deployment` Application |
| `run-argo-diff-pod.sh` | Runs argo-diff as a pod and asserts its exit code and logs |

`helm-deployment` is deliberately **multi-source**: the chart comes from
`charts/test-basic-deployment` in this repo and the values from `test/helm-deployment.values.yaml`,
which is what exercises the `--revisions` / `--source-positions` code path in `internal/argocd`.

## App-in-any-namespace coverage

`argocd-meta.yaml` and `argocd-applications/` cover the default case: every Application lives in the
`argocd` control-plane namespace. `argocd-meta-any-ns.yaml` and `argocd-applications-any-ns/` run a
second, parallel app-of-apps tree that instead exercises argo-diff's `--app-namespace` support
(added for ArgoCD's "app-in-any-namespace" feature): the app-of-apps Application `any-ns-meta` lives
in namespace `argocd-apps`, its child `any-ns-deployment` lives in namespace `apps`, and the child's
actual workload deploys into namespace `any-ns-deployment` — three different namespaces, none of
them `argocd`. Both `argocd-apps` and `apps` are created by the workflow and passed to ArgoCD via
`application.namespaces` in `argocd-helmchart.yaml` (the argo-helm passthrough to
`argocd-cmd-params-cm`, without which ArgoCD ignores Applications outside its own namespace). A
single shared AppProject (`any-ns`) covers both Applications; unlike `meta`/`test`, there's no need
to split it, since that split is a style choice rather than a requirement. `any-ns-deployment`'s
workload is deliberately self-contained (default ServiceAccount, no ClusterRole/Secret) so it can't
collide with `basic-deployment`'s cluster-scoped `test-clusterrole`/`test-sa-rolebinding`.

`argocd-helmchart.yaml` also sets `configs.rbac.policy.default: role:admin`. This isn't optional for
the any-ns apps: `server.disable.auth` only bypasses *authentication*, not RBAC (a known upstream
quirk, [argoproj/argo-cd#332](https://github.com/argoproj/argo-cd/issues/332)), and the built-in
`role:readonly` default policy only matches the legacy `<project>/<app>` object format used by apps
in the `argocd` namespace. Apps in any other namespace are addressed as
`<project>/<namespace>/<app>`, which no built-in `readonly` policy line matches, so `argocd app
diff`/`app wait` against `any-ns-meta`/`any-ns-deployment` fail with a gRPC `PermissionDenied` unless
the default role is bumped to `admin`. Since auth is already fully disabled on this test cluster,
that's a no-op from a security standpoint here — the apps in the default `argocd` namespace were
already unauthenticated-admin-equivalent for anything the diff/wait/list read paths need.

Pinning the in-cluster ArgoCD chart/app version (currently "latest", same as the runner's own
`argocd` CLI used to drive `argocd app wait`) is a possible future enhancement, not covered here —
only the argocd CLI bundled into the argo-diff image is pinnable today (see below).

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

The script reads the pod's logs **once** into a variable and both prints and matches that copy. Do
not turn the `REQUIRE_LOG` check back into `kubectl logs | grep -q`: the script runs under `set -o
pipefail`, `grep -q` exits at the first match, and `kubectl` then dies of SIGPIPE (silently, exit
141) writing the remainder — which pipefail reports as a failed assertion even when the substring
was there. That produced a spurious e2e failure on a ~19 KB pod log where the matched line was near
the end.

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
  changes in a fork PR are not exercised by the dispatch. It also takes an optional `argocd_version`
  input — the version of the `argocd` CLI to bundle into the argo-diff image under test (e.g.
  `v3.5.2`). Leave it blank to build with the version pinned in the `Dockerfile`'s `ARGOCD_IMAGE`
  build arg; this only affects the CLI argo-diff itself shells out to, not the in-cluster ArgoCD
  install or the runner's own `argocd` CLI (both still float on "latest"). A non-blank
  `argocd_version` also changes `ARGO_DIFF_CONTEXT_STR` (from `ephemeral-environment-test`/
  `-failure` to `ephemeral-environment-test-argocd-<version>`/`-failure-argocd-<version>`) and sets
  `ARGO_DIFF_COMMENT_PREAMBLE` to call out the overridden version. `ARGO_DIFF_CONTEXT_STR` is the key
  argo-diff uses to find its own comment on a PR (see `internal/github/context.md`), so a
  pinned-version dispatch posts its own comment instead of overwriting the standard run's.

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

It runs two scenarios: the healthy apps (`EXPECT_EXIT=0`, which includes waiting on `any-ns-meta`
and `any-ns-deployment` alongside `meta`/`basic-deployment`/`helm-deployment`), then — after deleting
`meta` (finalizer removed first, since its `selfHeal` would resurrect its children) and applying the
broken app — the failure case (`EXPECT_EXIT=nonzero`,
`REQUIRE_LOG="Failed to diff application broken-deployment"`). The any-namespace apps are left
running through the failure scenario too, since only `meta`'s children are swapped out.

Both runs comment on the PR, so their output is visible on the PR under test.
