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
- An application with `WarnStr` (its diff failed) is **fatal**: it counts as an error →
  `StatusFailure` + `*callerErr`, and its diffs are suppressed. Passed as
  `github.AppMarkdownOpts.ErrStr`, which renders as a red `> [!CAUTION]` alert.
- An application with `NoticeStr` is **advisory**: its diffs render as normal with the notice above
  them, and `errorCount`, `firstError`, the status and `*callerErr` are all untouched. Passed as
  `github.AppMarkdownOpts.NoticeStr`, which renders as a blue `> [!NOTE]` alert. Used when the diff
  is good but something alongside it degraded — today, an app-of-apps whose children couldn't be
  enumerated. It needs no term in the "should we comment at all" condition: `NoticeStr` is only ever
  set on an app that already has changed resources, so it implies `changeCount > 0`.
- The two travel in separate fields of `AppMarkdownOpts` rather than one string with an `"Error: "`
  prefix, which is what they used to share — severity a reader can see is worth a struct field.
- `CommentMarkdown.Notices` is set from `config.Notices()` — comment-level advisories that describe
  the environment, not this event. See `internal/config/context.md`.
- **A `NoticeStr` is a partial diff that deliberately does not fail the run**, which sits against
  the `notDiffed` rule above. The one producer is an app-of-apps whose children have changes and
  could not be enumerated (`internal/argocd/context.md`), so the comment is genuinely incomplete
  and the status is still `success`. That is a decision, not an oversight: the parent's own diff is
  good and worth showing, and the failure is usually an ArgoCD-side repository problem rather than
  anything about the PR. The notice says plainly that the child diffs are absent, which is what a
  reader needs to judge it. Revisit by routing it to `notDiffed` — but `timeoutMarkdown()`'s "ran
  out of time" wording would then be wrong for it and would need its own block.
- An app with a `NoticeStr` but no changed resources cannot happen today (`processTopLevelApp()`
  returns first), but the `else if` logs a warning rather than dropping the advisory in silence,
  since that invariant lives in another package.
- No changes, no warnings, and nothing skipped → `github.Comment()` is called with an **empty**
  body list, which clears out any stale argo-diff comments.

## Tests

`code_change_test.go` covers the pure helpers only — `processTimeout()`, `reportReserve()`,
`timeoutMarkdown()`. Those read env on each call, so `t.Setenv` works. `ProcessCodeChange()` itself
has no test: it reaches the network through the `argocd` and `github` packages, which have no
injection point at this level.
