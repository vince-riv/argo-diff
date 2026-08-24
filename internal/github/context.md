# internal/github/

Everything that talks to the GitHub API, plus the markdown rendering for PR comments. Built on
`github.com/google/go-github/v89` (the major version is in the import path — a Renovate bump
requires a code change).

## Files

| File | Contents |
| ---- | -------- |
| `comment.go` | Client construction, `Comment()`, `GetPullRequest()`, `ListPullRequestFiles()`, `IsRefreshComment()`, `ConnectivityCheck()` |
| `markdown.go` | `CommentMarkdown` / `ArgoAppMarkdown` — renders diffs into comment bodies and splits them across comments |
| `status.go` | `Status()` — commit status checks |

## Clients

`comment.go` and `status.go` each build their **own** client in `init()`, using the first available
credential: `GITHUB_PERSONAL_ACCESS_TOKEN`, then `GITHUB_TOKEN`, then a GitHub App
(`GITHUB_APP_ID` + `GITHUB_APP_INSTALLATION_ID` + `GITHUB_APP_PRIVATE_KEY` via `ghinstallation`).
The App path also builds `appsClient` (JWT auth) to resolve the bot's own login. Client
construction failures only log — the nil client surfaces later as an error.

`getCommentUser()` caches the login (`commentLogin`) behind an `RWMutex`; the App path appends
`[bot]`.

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

## Markdown limits

- `maxCommentLen` = 261500 (GitHub's cap is 262144); `CommentMarkdown.String()` returns a **slice**
  of bodies, splitting between applications or between resources, repeating the app header with
  `(cont.)` when a split lands mid-application.
- `maxResourceDiffLen` = 260000 — a single resource diff over that renders as
  `<<< DIFF TOO LARGE TO DISPLAY >>>`.
- Individual lines longer than `COMMENT_LINE_MAX_CHARS` (default 175) get `...[TRUNCATED]`.
- `ARGOCD_UI_BASE_URL` adds a link to each app; the app path is hardcoded to `/applications/argocd/`.
- Sync/health statuses render with emoji via `syncString()` / `healthString()`.

## Commit statuses

`Status()` is a no-op when `GITHUB_ACTIONS=true` (`skipCommitStatus`) — under Actions the step's
exit code is the signal. The `dryRun` argument (dev mode) logs instead of calling the API.
The context string is `argo-diff` or `argo-diff/<ARGO_DIFF_CONTEXT_STR>`, and descriptions are
truncated to 140 characters.

## Tests

`comment_test.go` spins up an `httptest.Server` that serves `github_testdata/` fixtures, then
assigns a `go-github` client pointed at it to the package-level `commentClient`. Placeholders
`%%_COMMENT_ID_%%` / `%%_PR_NUM_%%` in the fixtures are substituted per request. Adding a new API
call means teaching that mock server the new path, or it will `t.Errorf` on the unknown route.
