# internal/argocd/

Wrapper around the **`argocd` CLI** — not an HTTP API client. Everything here ends up as an
`exec.CommandContext` invocation of `argocd app list|get|manifests|diff` or `argocd version`.

## Files

| File | Contents |
| ---- | -------- |
| `argocd_client.go` | CLI argv construction, `execArgoCdCli`, and the parsers for its output |
| `helper.go` | The public entry points: `ConnectivityCheck()`, `GetApplicationChanges()`, plus application matching (`filterApplications`, `checkSource`, `gitRepoMatch`) and app-of-apps handling |
| `concurrency.go` | `runWithLimit()` — the bounded worker pool `GetApplicationChanges()` diffs applications through — and `maxWorkers()`, which reads `ARGO_DIFF_MAX_WORKERS` |
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

**`commonCliArgv` must never be combined with per-call `args` via a plain `append`.** Depending on
how many optional flags `init()` set, `commonCliArgv` can end up with spare capacity; `append`
would then write the per-call args into that shared backing array, so a call running concurrently
on another goroutine can observe (or clobber) another call's argv — silently diffing the wrong
application. `execArgoCdCli` uses `slices.Concat(commonCliArgv, args)` instead, which always
allocates a fresh backing array. `TestExecArgoCdCliConcurrentArgvIsolation` in
`argocd_client_test.go` is the only test in this package that exercises the real, unmocked argv
construction (everything else replaces `execArgoCdCli` wholesale) — it reproduces a
`commonCliArgv` with spare capacity and asserts many concurrent calls each observe exactly their
own args, via a fake `argocd` shell script pointed to by `ARGOCD_CLI_CMD_NAME`. Run with `-race`.

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
2. Filters to single-source apps matching the event (`filterApplications(..., multiSource=false)`)
   and diffs each one through a bounded worker pool (wave 1). Any diff that turns up nested
   `argoproj.io/Application` resources (**app-of-apps**) queues those nested apps rather than
   diffing them inline.
3. Diffs all queued nested apps, flattened across every parent, through the same bounded pool
   (wave 2), via `getMultiSrcAppChanges()`.
4. Filters again with `multiSource=true` and diffs anything not already covered, again through the
   pool (wave 3).

Each wave runs to completion (all its goroutines finish) before the next starts, so at most
`maxWorkers()` (`ARGO_DIFF_MAX_WORKERS`, default `4`, capped at `32`) `argocd` CLI calls are ever in
flight at once, system-wide — see `concurrency.go`. Each wave's results are written into a
pre-sized, index-addressed slice (one slot per app/job) so the merge back into `appResList` is
deterministic regardless of which worker finishes first. Wave 2 (nested apps) itself runs as one
flattened, pool-bounded batch across every parent rather than per-parent — but each `nestedJob`
carries the `parentIdx` of the wave-1 app that queued it, and `GetApplicationChanges()` walks
wave 1 and wave 2's results together afterward to append each parent's nested-app entries
immediately after its own, so both `appResList` and `notDiffed` still read "parent, its nested
apps, next parent, ..." — the same order the old sequential loop produced. This single merge loop
drives both slices together on purpose: an earlier version merged `appResList` this way but still
built `notDiffed` as "all wave-1 skips, then all wave-2 skips," which only reads as flat
mis-ordering when a later top-level app is skipped outright (deadline already passed) while an
earlier parent's nested child is also skipped — the common case of one parent's own skip next to
another parent's skip happens to coincide either way.
`TestGetApplicationChangesNestedAppsGroupedWithParent` covers the `appResList` side: multiple
parents with nested apps, diffed concurrently with artificial jitter, must still come back grouped.
`TestGetApplicationChangesNotDiffedGroupedWithParent` covers the `notDiffed` side with the
deadline-ordering scenario above. A pre-existing inconsistency was preserved rather than fixed during the
original parallelization refactor: a wave-2 (nested app) diff error while `ctx` still has time left
sets `WarnStr` but is not appended to `appResList`, unlike the equivalent wave-1/wave-3 cases which
do append their `WarnStr` result.

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

The pool-behavior tests in `helper_test.go` (`TestGetApplicationChangesConcurrencyBound`,
`TestGetApplicationChangesOrderStable`, `TestGetApplicationChangesNestedAppsGroupedWithParent`,
`TestGetApplicationChangesNotDiffedGroupedWithParent`) don't use a fixture file — `buildTestApps()`
builds `[]Application` values in Go and
`json.Marshal`s them into the mocked `argocd app list` response, since the number of apps needs to
vary with the test. `TestGetApplicationChangesConcurrencyBound` asserts observed max concurrency is
`>1` and `<=` the configured limit rather than exactly equal, since an exact count can under-report
on a loaded CI runner. `TestGetApplicationChangesOutOfTime` and
`TestGetApplicationChangesOutOfTimeEnumeratingNestedApps` set `ARGO_DIFF_MAX_WORKERS=1` so wave 1
runs one app at a time — the second test's mock relies on strict ordering, since it calls the
test's own `cancel()` from inside the diff callback to simulate the deadline landing mid-run.
