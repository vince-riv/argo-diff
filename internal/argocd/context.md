# internal/argocd/

Wrapper around the **`argocd` CLI** — not an HTTP API client. Everything here ends up as an
`exec.CommandContext` invocation of `argocd app list|get|manifests|diff` or `argocd version`.

## Files

| File | Contents |
| ---- | -------- |
| `argocd_client.go` | CLI argv construction, `execArgoCdCli`, and the parsers for its output |
| `helper.go` | The public entry points: `ConnectivityCheck()`, `GetApplicationChanges()`, plus application matching (`filterApplications`, `checkSource`, `gitRepoMatch`) and app-of-apps handling |
| `application.go` | Trimmed-down copies of ArgoCD's `Application` types — only the fields used here, so the ArgoCD source tree isn't a dependency |
| `filter_manifest_paths.go` | `FilterApplicationsByPath()` — the `argocd.argoproj.io/manifest-generate-paths` filter |
| `types.go` | `AppResource`, `ApplicationResourcesWithChanges`, `K8sManifest` |

## CLI invocation

`init()` builds `commonCliArgv` once from the environment: `--server`, `--auth-token`, and
optionally `--insecure`, `--plaintext`, `--grpc-web`, `--grpc-web-root-path`. It also captures
`ARGOCD_OPTS` and validates `ARGOCD_APP_DIFF_SERVER_SIDE_DIFF` (must be `true`/`false`).

`execArgoCdCli` is a **package-level `var`, so tests can replace it** — that is the mocking seam
for this whole package. It prepends `commonCliArgv`, sets `KUBECTL_EXTERNAL_DIFF=diff -u` (hence
`diffutils` in the Dockerfile), and re-injects `ARGOCD_OPTS`.

`argocdCmdFromEnv()` honors `ARGOCD_CLI_CMD_NAME` (default `argocd`).

### Output parsing

- **`argocd app diff` exits 1 when there are differences.** That is the success path: an
  `*exec.ExitError` with code 1 *and* non-empty stdout is parsed into `[]AppResource`. Any other
  exit is a real failure and is wrapped with stderr. Do not "fix" this by treating exit 1 as an
  error.
- Per-resource diffs are split on `"\n\n====="`, then the header line
  `===== group/Kind namespace/name ======` is parsed by `extractKubernetesFields()`.
- `argocd app manifests` output is split on `"\n---"` into `K8sManifest` values that keep both the
  raw YAML and an `unstructured.Unstructured` decode.
- `parseArgoCDVersion()` reads the `argocd:` / `argocd-server:` lines and trims the `+sha` suffix.

## Matching applications to a change

`GetApplicationChanges(ctx, eventInfo)` is the one entry point `process_event` calls. It:

1. Lists all applications (`argocd app list -o json`).
2. Filters to single-source apps matching the event (`filterApplications(..., multiSource=false)`),
   diffs each one, and then looks inside each diff for nested `argoproj.io/Application` resources
   (**app-of-apps**): those nested apps are re-diffed as multi-source apps against the changed
   revision, via `getMultiSrcAppChanges()`.
3. Filters again with `multiSource=true` and diffs anything not already covered.

Matching rules worth knowing:

- `gitRepoMatch()` matches `github.com/owner/repo[.git]` (and the scp-style `github.com:owner/repo`)
  by suffix, then falls back to a **host-agnostic, case-insensitive** `/owner/repo` or `:owner/repo`
  suffix match for GitHub Enterprise, CodeConnections, mirrors, etc. That fallback is disabled by
  `ARGO_DIFF_DISABLE_NON_GITHUB_REPO_MATCH=true`, and never applies to Helm chart/OCI sources
  (`source.chart != ""`), which are not git remotes.
- `checkSource()` compares the PR's base ref against the source's `targetRevision`, normalizing
  `refs/heads/x` → `x` (`normalizeBranchRef`). `targetRevision: HEAD` means "the repo default
  branch". For pushes (no base ref) it filters out auto-sync apps. `ARGO_DIFF_SKIP_REF_CHECK=true`
  bypasses all of this — the k3s e2e test relies on it.
- `filterApplications()` **breaks after the first matching source**. A multi-source app in a
  monorepo commonly matches twice (chart source + values source); all matching source positions are
  passed to one `argocd app diff --revisions ... --source-positions ...` call, so appending the app
  twice just doubles the CLI calls (fixed in 608c9d2 — don't reintroduce).
- `FilterApplicationsByPath()` then applies `argocd.argoproj.io/manifest-generate-paths`. No
  annotation, or `/`, means "always include". Relative patterns are joined with `source.path`,
  absolute ones are repo-root relative; glob patterns (`*?[`) go through `filepath.Match`, plain
  ones are treated as directory prefixes.

## Timeouts and partial results

`GetApplicationChanges()` returns `([]ApplicationResourcesWithChanges, notDiffed []string, error)`.
When `ctx` expires it **stops issuing diffs** rather than firing calls that can only fail, and
records the skipped application names in `notDiffed` so the caller can report an honest partial
result. A diff error while `ctx.Err() != nil` is attributed to the deadline, not to the
application. When the deadline hits while enumerating an app-of-apps' children, a single
`"nested apps of <name>"` entry is recorded — without it the run would look complete while every
nested diff was missing.

`minVersion` is `2.12.0`; `ConnectivityCheck()` fails if either the client or server is older.

## Tests

`argocd_testdata/` holds two kinds of fixtures:

- `output-argocd-*` — captured stdout from the CLI (the ones actually in use).
- `payload-GET-*.json` — leftovers from the pre-CLI HTTP API era; most are referenced only from
  commented-out constants in `helper_test.go`. `payload-GET-applications-brief.json` is still used.

`getMockedArgoCdCli()` (in `argocd_list_test.go`) builds a stub from a fixture file; tests swap
`execArgoCdCli` and restore it with `defer`. `makeExitError()` (in `argocd_client_test.go`)
produces a genuine `*exec.ExitError` with exit code 1 for the diff-parsing tests.
