# internal/config/

Cross-package operator configuration — settings more than one internal package needs to read.
The connectivity-check bypass and the PR comment notice channel; not a dumping ground for every
env var (most packages still read their own directly in `init()`, see each package's `context.md`).

## Files

| File | Contents |
| ---- | -------- |
| `connectivity.go` | `ARGO_DIFF_BYPASS_CONNECTIVITY_CHECKS` parsing: `BypassConnectivityCheck()`, `LogBypassConfig()` |
| `collapse.go` | `ARGO_DIFF_COMMENT_COLLAPSE` parsing: `CommentCollapseMode()` and the `CollapseAuto`/`CollapseExpanded`/`CollapseCollapsed` constants |
| `providers.go` | `ARGO_DIFF_SCM_PROVIDERS` parsing: `ScmProviders()` |
| `notice.go` | `ARGO_DIFF_COMMENT_NOTICE` plus the in-process notice channel: `Notices()`, `AddNotice()` |

## `ARGO_DIFF_BYPASS_CONNECTIVITY_CHECKS`

Comma-separated, case-insensitive, whitespace-trimmed list of components whose connectivity check
to skip: `github`, `gitlab`, `argocd`, or `true`/`all` for every one. `false`/`none`/empty tokens are recognized
no-ops. Unrecognized tokens are logged as a warning and otherwise ignored — same
warn-and-continue posture as `ARGO_DIFF_MAX_WORKERS` in `internal/argocd/concurrency.go`.

`BypassConnectivityCheck(component string) bool` is a plain function, not `init()`-cached, so
tests can `t.Setenv` it and so it's cheap to call from a per-event path (`getExistingComments()`
calls it every event, not just at startup).

`LogBypassConfig()` logs the resolved bypass state once. `cmd/main.go` calls it exactly once at
startup — don't call it from a per-event path, or unknown-token warnings spam the log.

## `ARGO_DIFF_SCM_PROVIDERS`

Which source control providers argo-diff runs for: a comma-separated, case-insensitive,
whitespace-trimmed list of provider names; **unset or empty means `github`**
(`DefaultScmProviders`). `ScmProviders(known)` takes the known names from `cmd/main.go`, so this
package does not list providers itself. Empty entries and duplicates are dropped. Unlike the bypass
list, an **unknown name is an error** and `true`/`all` are not accepted: this list decides what
runs, so a typo must fail startup rather than quietly turn a provider off. It also returns whether
the value was the default, so `cmd/main.go` can tell an operator who never set it what to set.

## `ARGO_DIFF_COMMENT_COLLAPSE`

`CommentCollapseMode()` returns `auto`, `expanded` (the default) or `collapsed`. It is
case-insensitive and whitespace-trimmed; an unknown value logs a warning and means `expanded`. Read on
call, so tests can `t.Setenv` it. `internal/comment` reads it to decide which blocks render folded.

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

The first caller is `supportsManifestsAppNamespace()` in `internal/argocd/argocd_client.go`: it
raises a notice on both its false branches (client version undetectable, client older than
`appNamespaceManifestsMinVersion`), so the PR comment explains why the children of app-of-apps
parents outside ArgoCD's own namespace are missing rather than leaving the reader to guess.

A notice is **advisory**, and renders differently from a warning or a fatal error. See the severity
table in `internal/comment/context.md`.

## Consumers

- `internal/process_event/code_change.go` — sets `CommentMarkdown.Notices` from `Notices()`.
- `cmd/main.go` — guards `argocd.ConnectivityCheck()` with `BypassConnectivityCheck(ComponentArgoCD)`.
- `internal/github/` — guards `ConnectivityCheck()` and `Provider.CurrentUser()` (which decides
  whether comments are matched by author) with `BypassConnectivityCheck(ComponentGithub)`. See that package's `context.md` for what bypassing
  `github` does to comment matching (it degrades to the same marker-only match GitHub Actions mode
  already uses).
- `internal/gitlab/` — guards `ConnectivityCheck()` with `BypassConnectivityCheck(ComponentGitlab)`.
