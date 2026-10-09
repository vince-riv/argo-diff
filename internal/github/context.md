# internal/github/

The GitHub provider: `Provider` implements `scm.Provider`, so everything GitHub-specific lives here
— the API client, PR comments, commit statuses, webhook parsing and signature checks, and GitHub
Actions detection. Comment rendering lives in `internal/comment`. Built on
`github.com/google/go-github/v92` (the major version is in the import path — a Renovate bump
requires a code change).

## Files

| File | Contents |
| ---- | -------- |
| `comment.go` | Client construction, `GetPullRequest()`, `ListPullRequestFiles()`, `getCommentUser()`, `ConnectivityCheck()` |
| `status.go` | `Status()` — commit status checks |
| `webhook.go` | `WebhookHandler` (the `scm.WebhookHandler` for GitHub), `ProcessPullRequest()`, `ProcessComment()` |
| `signature.go` | `VerifySignature()` — HMAC-SHA256 over the raw body |
| `ci.go` | `Provider.DetectCI()` (`GITHUB_ACTIONS=true`) and `Provider.EventFromCIEnv()` |
| `provider.go` | `Provider` — implements `scm.Provider`: the comment primitives, status, changed files, and the startup checks (`Enabled`, `ValidateConfig`, `ConnectivityCheck`, `RepoHosts`) |

go-github types stay inside this package. `GetPullRequest()` returns an `scm.ChangeRequest` and
`Status()` takes an `scm.Status` (see `internal/scm/context.md`), so callers never import go-github.

## Clients

`comment.go` and `status.go` each build their **own** client in `init()`, using the first available
credential: `GITHUB_PERSONAL_ACCESS_TOKEN`, then `GITHUB_TOKEN`, then a GitHub App
(`GITHUB_APP_ID` + `GITHUB_APP_INSTALLATION_ID` + `GITHUB_APP_PRIVATE_KEY` via `ghinstallation`).
The App path also builds `appsClient` (JWT auth) to resolve the bot's own login. Client
construction failures only log — the nil client surfaces later as an error. With no token and no
`GITHUB_APP_ID`, `init()` builds nothing and logs nothing: GitHub is simply disabled (eg: a
GitLab-only setup). A partial App setup is reported by `Provider.ValidateConfig()` at startup.

`getCommentUser()` caches the login (`commentLogin`) behind an `RWMutex`; the App path derives it
from `App.GetSlug()` (falling back to `App.GetName()` if the slug is empty) and appends `[bot]` —
GitHub builds the bot's real login from the App's slug, not its display name, so using `Name`
directly breaks comment reuse for any App whose name isn't already slug-shaped.

## GitHub Actions

`EventFromCIEnv()` builds the event from the Actions variables: requires
`GITHUB_EVENT_NAME=pull_request`, parses the PR number out of `GITHUB_REF` (`refs/pull/<n>/merge`),
splits `GITHUB_REPOSITORY`, and reads `REPO_DEFAULT_REF`, `GITHUB_HEAD_REF`, `GITHUB_BASE_REF`. A
malformed `GITHUB_REF` or `GITHUB_REPOSITORY` is an error rather than a panic. It sets
`Refresh: true` so the sha and refs are re-read from the API rather than trusted from the
environment.

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
- Whether an `issue_comment` re-runs the diff is decided by `comment.IsRefreshComment()` (see
  `internal/comment/context.md`).

## Webhooks

`WebhookHandler.Verify()` checks `X-Hub-Signature-256` against `GITHUB_WEBHOOK_SECRET` (read into
`webhookSecret` at package load). `Parse()` dispatches on `X-GitHub-Event`: `ping` →
`scm.WebhookPing`; `pull_request` → `ProcessPullRequest()`; `issue_comment` → `ProcessComment()`;
anything else → `scm.WebhookIgnored`. Parsed events get `EventInfo.Provider = "github"`.

### Event handling

- `ProcessPullRequest()` acts on the `opened` and `synchronize` actions, and on `edited` only when
  `changes.base.ref.from` is set — that is how GitHub reports a base-branch retarget (eg: when a
  stacked PR's parent branch merges and GitHub repoints it at `main`), as opposed to an ordinary
  title/body edit, which also sends `edited` but leaves `changes.base` empty. That check reads the
  field through nil-safe generated getters inline, so it needs no extra nil checks. Every other
  field is read through raw pointer dereferences — a malformed payload panics rather than erroring.
- `ProcessComment()` handles `issue_comment`: action must be `created`, the issue must be a PR
  (`PullRequestLinks != nil`), and the body must satisfy `comment.IsRefreshComment()` (`argo diff` /
  `argo-diff` by default, overridable via `ARGO_DIFF_REFRESH_COMMENT_KEYWORDS`, optionally suffixed
  with the context string). It sets `Refresh: true`, leaving the sha and refs to be resolved from the
  API.

### Signatures

`VerifySignature()` requires a non-empty secret, the exact `sha256=` + 64 hex chars length, and
compares with `hmac.Equal`. It is skipped entirely in dev mode by the server.

## Commit statuses

`Status()` is a no-op when `GITHUB_ACTIONS=true` (`skipCommitStatus`) — under Actions the step's
exit code is the signal. The `dryRun` argument (dev mode) logs instead of calling the API.
The context string is `argo-diff` or `argo-diff/<ARGO_DIFF_CONTEXT_STR>`, and descriptions are
truncated to 140 characters.

## Tests

Webhook payload fixtures live in `github_testdata/webhook/`. They are real captured payloads: `payload-pr-open.json`, `payload-pr-sync.json`,
`payload-pr-close.json`, `payload-comment-created.json`, `payload-comment-argodiff-created.json`.
`payload-pr-edited-base.json` and `payload-pr-edited-title.json` are **derived**, not captured —
copies of `payload-pr-sync.json` with `action` changed to `edited`, the sync-only `before`/`after`
keys removed, and a `changes` block added (base retarget vs. title-only) to exercise the base-retarget
check. `webhook_test.go` asserts which of them are ignored vs. actionable; `signature_test.go` covers
the bad-length, bad-prefix, and valid cases.
`TestWebhookHandler` covers `Verify()`/`Parse()` end to end.

`comment_test.go` still calls `Comment()` and `getExistingComments()`: those are now test-only
adapters in `adapters_test.go` that run `scm.PostComments()` / `scm.ExistingComments()` over
`Provider` and convert the result back to go-github types, so the original assertions pin the
behavior across the move. `comment_test.go` spins up an `httptest.Server` that serves `github_testdata/` fixtures, then
assigns a `go-github` client pointed at it to the package-level `commentClient`. Placeholders
`%%_COMMENT_ID_%%` / `%%_PR_NUM_%%` in the fixtures are substituted per request. Adding a new API
call means teaching that mock server the new path, or it will `t.Errorf` on the unknown route.
