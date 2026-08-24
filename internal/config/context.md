# internal/config/

Cross-package operator configuration — settings more than one internal package needs to read.
Currently just the connectivity-check bypass; not a dumping ground for every env var (most
packages still read their own directly in `init()`, see each package's `context.md`).

## Files

| File | Contents |
| ---- | -------- |
| `connectivity.go` | `ARGO_DIFF_BYPASS_CONNECTIVITY_CHECKS` parsing: `BypassConnectivityCheck()`, `LogBypassConfig()` |

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

## Consumers

- `cmd/main.go` — guards `argocd.ConnectivityCheck()` with `BypassConnectivityCheck(ComponentArgoCD)`.
- `internal/github/comment.go` — guards `ConnectivityCheck()`, the `getCommentUser()` call inside
  `getExistingComments()`, and the comment-author match, all with
  `BypassConnectivityCheck(ComponentGithub)`. See that package's `context.md` for what bypassing
  `github` does to comment matching (it degrades to the same marker-only match GitHub Actions mode
  already uses).
