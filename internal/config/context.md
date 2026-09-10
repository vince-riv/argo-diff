# internal/config/

Cross-package operator configuration — settings more than one internal package needs to read.
The connectivity-check bypass and the PR comment notice channel; not a dumping ground for every
env var (most packages still read their own directly in `init()`, see each package's `context.md`).

## Files

| File | Contents |
| ---- | -------- |
| `connectivity.go` | `ARGO_DIFF_BYPASS_CONNECTIVITY_CHECKS` parsing: `BypassConnectivityCheck()`, `LogBypassConfig()` |
| `notice.go` | `ARGO_DIFF_COMMENT_NOTICE` plus the in-process notice channel: `Notices()`, `AddNotice()` |

## `ARGO_DIFF_BYPASS_CONNECTIVITY_CHECKS`

Comma-separated, case-insensitive, whitespace-trimmed list of components whose connectivity check
to skip: `github`, `argocd`, or `true`/`all` for both. `false`/`none`/empty tokens are recognized
no-ops. Unrecognized tokens are logged as a warning and otherwise ignored — same
warn-and-continue posture as `ARGO_DIFF_MAX_WORKERS` in `internal/argocd/concurrency.go`.

`BypassConnectivityCheck(component string) bool` is a plain function, not `init()`-cached, so
tests can `t.Setenv` it and so it's cheap to call from a per-event path (`getExistingComments()`
calls it every event, not just at startup).

`LogBypassConfig()` logs the resolved bypass state once. `cmd/main.go` calls it exactly once at
startup — don't call it from a per-event path, or unknown-token warnings spam the log.

## `ARGO_DIFF_COMMENT_NOTICE`

Advisory text rendered as a `> [!NOTE]` alert at the top of the PR comment — a deprecation warning,
or a caveat about this environment. Several notices are separated by `|`, deliberately not a comma
or a newline, since notice text is prose and both appear in ordinary sentences.

There are two producers, and `Notices()` returns them in this order:

1. The operator, through the env var. Read on call, not `init()`-cached, so `t.Setenv` reaches it.
2. argo-diff itself, through `AddNotice()` — for something the run discovers, such as an `argocd`
   CLI too old for a feature. Mutex-guarded (`internal/argocd` diffs applications concurrently),
   deduped so a per-application code path can call it unconditionally, and capped at `maxNotices`
   (10) so it can't crowd the diffs out of the comment budget.

Nothing calls `AddNotice()` yet — it is the seam for that first version-gated feature.
`ConnectivityCheck()` in `internal/argocd/helper.go` already parses the client and server versions
and then discards them, which is the obvious place to hook one in.

A notice is **advisory**, and renders differently from a warning or a fatal error. See the severity
table in `internal/github/context.md`.

## Consumers

- `internal/process_event/code_change.go` — sets `CommentMarkdown.Notices` from `Notices()`.
- `cmd/main.go` — guards `argocd.ConnectivityCheck()` with `BypassConnectivityCheck(ComponentArgoCD)`.
- `internal/github/comment.go` — guards `ConnectivityCheck()`, the `getCommentUser()` call inside
  `getExistingComments()`, and the comment-author match, all with
  `BypassConnectivityCheck(ComponentGithub)`. See that package's `context.md` for what bypassing
  `github` does to comment matching (it degrades to the same marker-only match GitHub Actions mode
  already uses).
