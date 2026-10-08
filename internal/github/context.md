# internal/github/

Everything that talks to the GitHub API. Comment rendering lives in `internal/comment`. Built on
`github.com/google/go-github/v92` (the major version is in the import path — a Renovate bump
requires a code change).

## Files

| File | Contents |
| ---- | -------- |
| `comment.go` | Client construction, `Comment()`, `GetPullRequest()`, `ListPullRequestFiles()`, `IsRefreshComment()`, `ConnectivityCheck()` |
| `status.go` | `Status()` — commit status checks |
| `provider.go` | `Provider` — the `scm.Commenter` primitives (comment list/create/update, current user, PR lookup) |

go-github types stay inside this package. `GetPullRequest()` returns an `scm.ChangeRequest` and
`Status()` takes an `scm.Status` (see `internal/scm/context.md`), so callers never import go-github.

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

- Every comment ends with an HTML marker, `comment.Identifier()`:
  `<!-- comment produced by argo-diff[<context>] -->`. Under GitHub Actions this package's `init()`
  calls `comment.SetIdentifierRef(GITHUB_REF)` so the marker also embeds the ref, and concurrent
  PRs don't collide. `ARGO_DIFF_CONTEXT_STR` distinguishes multiple argo-diff instances (eg: one per
  cluster). `comment.Wrap()` adds the preamble and marker to every body posted.
- The posting algorithm (HEAD check, reuse in order, outdating leftovers) is provider-neutral and
  lives in `scm.PostComments()` (see `internal/scm/context.md`). This package supplies only the
  primitives, as methods on `Provider` in `provider.go`: `GetChangeRequest`, `ListComments` (all
  pages, oldest first), `CreateComment`, `UpdateComment`, `CurrentUser`.
- `isGithubAction` (`GITHUB_ACTIONS=true` **and** `ARGO_DIFF_CI != "true"`) skips the
  connectivity check and the comment-author check — under Actions the token's identity isn't
  resolvable the same way. `go.yml` sets `ARGO_DIFF_CI=true` so tests behave like the deployed
  service.
- `bypassGithubCheck()` (see `internal/config/context.md`) does the same thing on purpose, for the same
  reason, but by operator opt-in: `ARGO_DIFF_BYPASS_CONNECTIVITY_CHECKS=github` is for a GitHub
  App installation token passed via `GITHUB_TOKEN` outside of Actions (eg: minted per-PR by a
  Jenkins plugin), which 403s on `GET /user` the same way an Actions token would. It's checked at
  every `isGithubAction` site: `ConnectivityCheck()` and `Provider.CurrentUser()`. Either one makes
  `CurrentUser()` return an empty login without calling `getCommentUser()`, and an empty login is
  how `scm.ExistingComments()` knows to match comments by the identifier marker alone. Outside those two
  cases an empty login is an **error**, not a marker-only match, so argo-diff can never start
  editing other users' marker-bearing comments.
- `IsRefreshComment()` matches any keyword in `refreshCommentKeywords` (parsed once in `init()` from
  `ARGO_DIFF_REFRESH_COMMENT_KEYWORDS`, a comma-separated list defaulting to `argo diff,argo-diff`),
  each optionally suffixed with the context string (case-insensitive, trimmed). This is what makes an
  `issue_comment` re-run the diff.

## Commit statuses

`Status()` is a no-op when `GITHUB_ACTIONS=true` (`skipCommitStatus`) — under Actions the step's
exit code is the signal. The `dryRun` argument (dev mode) logs instead of calling the API.
The context string is `argo-diff` or `argo-diff/<ARGO_DIFF_CONTEXT_STR>`, and descriptions are
truncated to 140 characters.

## Tests

`comment_test.go` still calls `Comment()` and `getExistingComments()`: those are now test-only
adapters in `adapters_test.go` that run `scm.PostComments()` / `scm.ExistingComments()` over
`Provider` and convert the result back to go-github types, so the original assertions pin the
behavior across the move. `comment_test.go` spins up an `httptest.Server` that serves `github_testdata/` fixtures, then
assigns a `go-github` client pointed at it to the package-level `commentClient`. Placeholders
`%%_COMMENT_ID_%%` / `%%_PR_NUM_%%` in the fixtures are substituted per request. Adding a new API
call means teaching that mock server the new path, or it will `t.Errorf` on the unknown route.
