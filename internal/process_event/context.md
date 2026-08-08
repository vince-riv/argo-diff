# internal/process_event/

The orchestrator. `ProcessCodeChange()` in `code_change.go` is the whole business logic for one
event: resolve the PR, diff the matching ArgoCD applications, set a commit status, post a comment.

It is always launched in a goroutine with a `sync.WaitGroup` and a `*error` out-parameter, so the
webhook server can return `200 OK` immediately while the run-once modes wait and turn `*callerErr`
into an exit code.

## Flow

1. **PR-only guard.** `eventInfo.PrNum <= 0` is an immediate error — push events are not supported.
2. **Refresh.** When `eventInfo.Refresh` is set (GitHub Actions mode, or an `argo diff` PR comment),
   `github.GetPullRequest()` fills in `Sha`, `ChangeRef`, and `BaseRef` from the live PR.
3. **Changed files** via `github.ListPullRequestFiles()`, used downstream by the
   `manifest-generate-paths` filter. A failure here is recorded but not fatal.
4. Commit status → `pending`.
5. `argocd.GetApplicationChanges(diffCtx, eventInfo)`.
6. Build the markdown, choose the final status, comment.

## Timeout budget

- `processTimeout()` reads `ARGO_DIFF_TIMEOUT` (Go duration string; a bare integer means seconds;
  invalid or non-positive falls back to `defaultProcessTimeout` = 3m). It exists because cost scales
  with the number of matching applications — each is a round trip to the argocd server.
- `reportReserve(timeout)` holds back `defaultReportReserve` = 30s for the closing GitHub calls, or
  half the budget when the timeout is under 60s.
- Diffing runs on `diffCtx` = parent minus the reserve. **Reporting runs on a context derived from
  `context.Background()`**, not from the parent — that is deliberate: when the parent is already
  past its deadline, a partial comment is far more useful than no comment (see 19faab8). The
  consequence is that a run can exceed `ARGO_DIFF_TIMEOUT` by up to the reserve.

## Reporting rules

- `notDiffed` (applications skipped because time ran out) forces `StatusFailure` and a non-nil
  `*callerErr`, and prepends a `> [!WARNING]` block naming them — capped at 20 names by
  `timeoutMarkdown()` so a change matching hundreds of apps can't crowd out the diffs. Reporting
  success on a partial diff is worse than failing.
- An application with `WarnStr` (its diff failed) counts as an error → `StatusFailure`.
- No changes, no warnings, and nothing skipped → `github.Comment()` is called with an **empty**
  body list, which clears out any stale argo-diff comments.
- `unknownCount` is vestigial: it is declared and reported but never incremented.

## Tests

`code_change_test.go` covers the pure helpers only — `processTimeout()`, `reportReserve()`,
`timeoutMarkdown()`. Those read env on each call, so `t.Setenv` works. `ProcessCodeChange()` itself
has no test: it reaches the network through the `argocd` and `github` packages, which have no
injection point at this level.
