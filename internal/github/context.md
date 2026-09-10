# internal/github/

Everything that talks to the GitHub API, plus the markdown rendering for PR comments. Built on
`github.com/google/go-github/v89` (the major version is in the import path — a Renovate bump
requires a code change).

## Files

| File | Contents |
| ---- | -------- |
| `comment.go` | Client construction, `Comment()`, `GetPullRequest()`, `ListPullRequestFiles()`, `IsRefreshComment()`, `ConnectivityCheck()` |
| `markdown.go` | `CommentMarkdown` / `ArgoAppMarkdown` — renders diffs into comment bodies and splits them across comments |
| `markdown_test.go` | Rendering, splitting and budget tests; no golden fixtures, assertions are on invariants |
| `status.go` | `Status()` — commit status checks |

## Clients

`comment.go` and `status.go` each build their **own** client in `init()`, using the first available
credential: `GITHUB_PERSONAL_ACCESS_TOKEN`, then `GITHUB_TOKEN`, then a GitHub App
(`GITHUB_APP_ID` + `GITHUB_APP_INSTALLATION_ID` + `GITHUB_APP_PRIVATE_KEY` via `ghinstallation`).
The App path also builds `appsClient` (JWT auth) to resolve the bot's own login. Client
construction failures only log — the nil client surfaces later as an error.

`getCommentUser()` caches the login (`commentLogin`) behind an `RWMutex`; the App path derives it
from `App.GetSlug()` (falling back to `App.GetName()` if the slug is empty) and appends `[bot]` —
GitHub builds the bot's real login from the App's slug, not its display name, so using `Name`
directly breaks comment reuse for any App whose name isn't already slug-shaped.

## Comment behavior

- Every comment ends with an HTML marker: `<!-- comment produced by argo-diff[<context>] -->`.
  Under GitHub Actions the marker also embeds `GITHUB_REF`, so concurrent PRs don't collide.
  `ARGO_DIFF_CONTEXT_STR` distinguishes multiple argo-diff instances (eg: one per cluster).
- `Comment()` no-ops when `sha` is no longer the PR head (`isPrHead()`), so a slow run can't
  overwrite a newer comment.
- It reuses existing argo-diff comments in order: body *i* edits existing comment *i*, extras are
  created, and leftovers are overwritten with `[Outdated argo-diff content]` rather than deleted.
- `isGithubAction` (`GITHUB_ACTIONS=true` **and** `ARGO_DIFF_CI != "true"`) skips the
  connectivity check and the comment-author check — under Actions the token's identity isn't
  resolvable the same way. `go.yml` sets `ARGO_DIFF_CI=true` so tests behave like the deployed
  service.
- `bypassGithubCheck()` (see `internal/config/context.md`) does the same thing on purpose, for the same
  reason, but by operator opt-in: `ARGO_DIFF_BYPASS_CONNECTIVITY_CHECKS=github` is for a GitHub
  App installation token passed via `GITHUB_TOKEN` outside of Actions (eg: minted per-PR by a
  Jenkins plugin), which 403s on `GET /user` the same way an Actions token would. It's checked at
  every `isGithubAction` site: `ConnectivityCheck()`, the `getCommentUser()` call in
  `getExistingComments()`, and the comment-author match — bypassing it means `commentLogin` stays
  empty and comments are matched by `commentIdentifier` alone, same as under Actions.
- `IsRefreshComment()` matches `argo diff` / `argo-diff`, optionally suffixed with the context
  string (case-insensitive, trimmed). This is what makes an `issue_comment` re-run the diff.

## Comment rendering

One comment reads top to bottom as: the preamble (change counts and timestamp, from
`process_event`), any `Notices`, the summary index table, then per application a `---` rule, that
application's alerts, and a `<details>` block holding one `<details>` block per changed resource.

- **Severity is carried by the GitHub alert type**, because a reader has to tell "this diff is
  untrustworthy" from "this diff is fine, but FYI" at a glance:
  | Field | Alert | Meaning |
  | ----- | ----- | ------- |
  | `CommentMarkdown.Notices` | `> [!NOTE]` (blue) | comment-level advisory, from `config.Notices()` |
  | `ArgoAppMarkdown.NoticeStr` | `> [!NOTE]` (blue) | this app's diff is good, something alongside it degraded |
  | `ArgoAppMarkdown.ErrStr` | `> [!CAUTION]` (red) | this app's diff **failed**; no resources are shown |
  | `timeoutMarkdown()` in `process_event` | `> [!WARNING]` (yellow) | applications left undiffed |
  `ErrStr` and `NoticeStr` are separate fields on purpose — they used to share one, distinguished
  only by a `"Error: "` string prefix at the call site.

### Alerts only render at the top level

**GitHub renders its coloured alert boxes only at the top level of a document.** Inside any HTML
element — `<details>` included — the extension is skipped and the block degrades to a plain
blockquote whose first line is a literal `[!NOTE]`. Nothing changes that: not extra blank lines,
not a blank line before `<summary>`, not a `<div>` wrapper, and not nesting inside another
blockquote. Verified against GitHub's own renderer (`POST /markdown`, and the `body_html` of a real
PR).

So `ArgoAppMarkdown.alerts()` is emitted by `String()` **outside** the app's `<details>`, between
the `---` rule and the block. Consequences worth knowing:

- A hoisted alert is detached from the block, so it **names its application**. `NoticeStr` from
  `internal/argocd` also names the app, so the rendered line repeats it once — accepted, because a
  notice that is self-contained at the `argocd` layer is worth more than the tidier sentence.
- An application with `ErrStr` and no resources renders as **just its alert** — no `<details>` at
  all. There are no diffs to fold, and the alert already carries the name, both statuses, the error
  text and the ArgoCD link.
- `appOpen()` never folds an application carrying a `NoticeStr` or `ErrStr`, whatever the auto
  thresholds say (an explicit `ARGO_DIFF_COMMENT_COLLAPSE=collapsed` still wins).
- `checkBodyWellFormed()` — a **test helper** in `markdown_test.go`, not a runtime check — walks
  `<details>` depth and fails on any `> [!` line nested inside one. That is the regression guard,
  and it only runs under `go test`. Nothing validates this at runtime, which is why the guard
  matters: the emitted string looks perfectly fine from Go.
- A fenced block inside an alert only renders when **every** one of its lines carries the `> `
  prefix, blank lines included — that is what `blockquote()` is for.
- `syncString()` / `healthString()` emit literal Unicode emoji, not `:shortcodes:`. These strings
  also land inside raw HTML `<summary>` elements, where shortcode substitution is not reliable.
- Blocks are folded per `ARGO_DIFF_COMMENT_COLLAPSE` (`auto` | `expanded` | `collapsed`). `auto`
  keeps a comment expanded until it covers more than `autoCollapseAppCount` (3) applications, or an
  application with more than `autoCollapseResourceCount` (5) resources. Because that decision needs
  totals `AddResourceDiff()` doesn't have, a resource stores its `Summary` and `Body` separately and
  `String()` renders it — nothing pre-renders a `<details>` tag.
- The index table is gated by `ARGO_DIFF_COMMENT_INDEX_COUNT`: `-1` always, `0` never, any other `n`
  once `n` applications have an entry (default 2). Capped at `maxIndexRows` (50).

## The comment size budget

`String()` splits its output across as many bodies as it takes, and **every** append is checked
against a budget:

```
budget := commentMaxLen() - commentWrapperLen()
limit  := budget - splitReserve
```

- `commentMaxLen()` is `ARGO_DIFF_COMMENT_MAX_CHARS`, defaulting to — and clamped at —
  `githubCommentHardMax` (262144).
- `commentBudget()` has a `minResourceLen` floor. Below roughly
  `len(truncatedMarker)+truncTailReserve` there is no room for `finalize()` to truncate *into*, so
  it would emit a marker that is itself over budget. The warning is the useful part, since nothing
  else tells an operator their preamble has eaten the comment.
  **The floor is itself clamped against `githubCommentHardMax`.** Raising the budget above the
  headroom that genuinely remains turns a useless-but-postable body into a 422, and posting nothing
  is worse than posting something tiny. `boundPreamble()` puts that out of reach for the preamble,
  but `commentIdentifier` carries `ARGO_DIFF_CONTEXT_STR`, which stays unbounded because comment
  matching depends on it — so the clamp is a live guard, not belt-and-braces.
- `boundPreamble()` in `comment.go` caps `ARGO_DIFF_COMMENT_PREAMBLE` at `maxPreambleLen` (4000).
  It takes the variable name as an argument because the preamble falls back to
  `ARGO_DIFF_CONTEXT_STR` — an operator with a long context string and no preamble set would
  otherwise be told to shorten a variable they never configured.
  Every input to the budget is now bounded; the preamble was the last one that was not, and
  `commentWrapperLen()` subtracts it from every body. README documents "150 chars or less", so the
  cap is generous by design — it exists to bound the input, not to police the guideline.
- `commentWrapperLen()` is what `Comment()` adds around each body: `ARGO_DIFF_COMMENT_PREAMBLE`
  (unbounded operator input) plus `commentIdentifier`. It is `len(wrapComment(""))`, derived from the
  same function `Comment()` posts with, so the two cannot drift. **This is the leak that used to
  produce a 422 and no comment at all**: a long preamble plus a full-size body.
- `splitReserve` covers everything appended *after* a fit check has already passed — the closing
  `</details>`, the continuation marker, and the `part i of n` header `finalize()` prepends.
- `ArgoAppMarkdown.maxResourceBodyLen()` bounds a single resource to what is left of one body once
  **everything `String()` emits ahead of it** is subtracted — the `---` rule, this app's alerts
  (up to `maxNoticeLen` each, so ~8KB), the `<details>` header and the resource's own markup — so a
  diff can never be too big for any body. Over it, the resource renders as
  `<<< DIFF TOO LARGE TO DISPLAY >>>`. Missing the alerts here is what used to overshoot the budget
  and drop a body into `finalize()` mid-markup.
- `ErrStr`, `NoticeStr` and each entry in `Notices` are bounded to `maxNoticeLen` (4000) by
  `truncateNotice()`; `HealthMsg` to `maxHealthMsgLen` (500). All three come from unbounded input
  (`err.Error()`, operator config, the cluster) and share the budget with the diffs.
- Individual lines longer than `COMMENT_LINE_MAX_CHARS` (default 175) get `...[TRUNCATED]`.
  `truncateLines()` always returns a string ending in exactly one newline, which is what lets a
  caller close a code fence on its own line — an unclosed fence swallows every diff below it.
- `finalize()` is the last guard: any body still over budget is hard-truncated with a visible
  marker. A 422 means no comment at all, which is worse than a truncated one. The cut lands at an
  arbitrary byte — almost always inside a resource, since resources are the bulk of a body — so it
  **closes what it cuts**: an odd ` ``` ` count gets a closing fence, then the marker, then one
  `</details>` per unclosed tag. Without that the fence swallows the marker and the reader sees the
  truncation notice as diff text. `truncTailReserve` holds the room for it.
- Every body is self-contained: balanced `<details>` tags and an even number of code fences. A
  mid-application split closes the app block before the continuation marker.
- `ARGOCD_UI_BASE_URL` adds a link to each app; the app path is hardcoded to `/applications/argocd/`.

## Commit statuses

`Status()` is a no-op when `GITHUB_ACTIONS=true` (`skipCommitStatus`) — under Actions the step's
exit code is the signal. The `dryRun` argument (dev mode) logs instead of calling the API.
The context string is `argo-diff` or `argo-diff/<ARGO_DIFF_CONTEXT_STR>`, and descriptions are
truncated to 140 characters.

## Tests

`markdown_test.go` sets the package vars `commentPreamble`, `commentIdentifier` and `argocdUiUrl`
directly (`init()` reads them, so `t.Setenv` can't reach them) and drives the rest through
`t.Setenv`. `checkBodyWellFormed()` asserts the per-body invariants above on every body of every
splitting test.

`comment_test.go` spins up an `httptest.Server` that serves `github_testdata/` fixtures, then
assigns a `go-github` client pointed at it to the package-level `commentClient`. Placeholders
`%%_COMMENT_ID_%%` / `%%_PR_NUM_%%` in the fixtures are substituted per request. Adding a new API
call means teaching that mock server the new path, or it will `t.Errorf` on the unknown route.
