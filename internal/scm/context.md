# internal/scm/

The source-control-provider abstraction: neutral types that the rest of argo-diff uses instead of
any one provider's API types. GitHub (`internal/github`) is the only provider today; the plan in
issue #160 adds GitLab.

| File | Contents |
| ---- | -------- |
| `types.go` | `RepoRef`, `ChangeRequest`, `Status` and its four states |
| `provider.go` | `Provider` interface, `DefaultProvider`, the registry (`Register`, `Lookup`, `Providers`) |
| `webhook.go` | `WebhookHandler` interface, `WebhookEvent`, `WebhookKind` |
| `comment.go` | `Comment`, the `Commenter` primitive interface, `ExistingComments()`, `PostComments()` |

## Types

- `RepoRef` is `Host` + `Owner` + `Name`. **`Owner` may contain `/`** (GitLab nested groups), so
  never split a full path on the first `/` to get it back. `Host` may be empty: the provider then
  uses its configured API host.
- `ChangeRequest` is a PR/MR reduced to number, head SHA, head ref and base ref. A field the
  provider did not return stays **empty rather than erroring**; each caller decides what a missing
  field means (`process_event` fails a refresh; the comment HEAD check treats it as "not HEAD").
- `Status` is `pending` / `success` / `failure` / `error`, the GitHub spelling. Providers map these
  onto their own states (GitLab rejects `failure` and wants `failed`). `Valid()` guards the set.

## Provider and registry

`Provider` is everything `process_event.ProcessCodeChange()` needs: the `Commenter` primitives plus
`Name()`, `Dialect()`, `ListChangedFiles()` and `SetStatus()`. GitHub's is `github.Provider`.

`cmd/main.go` calls `Register()` at startup. `Lookup(name)` returns a provider by name; **an empty
name means `DefaultProvider` (`github`)**, which keeps event files and webhooks from before
multi-provider support working. `Providers()` lists them sorted by name.

## Webhooks

Each provider's `WebhookHandler()` verifies and parses its webhook requests; the server reads the
body once and calls `Verify(headers, body)` (skipped in dev mode) then `Parse(headers, body)`.
`Parse` classifies the request as `WebhookPing` (acknowledged), `WebhookIgnored` (an event type
argo-diff doesn't handle) or `WebhookChange`, whose `Info` is the `EventInfo` with `Provider` set.
A `WebhookChange` may still carry `Info.Ignore` (eg: a closed PR). This package imports
`internal/webhook` for `EventInfo`.

## Posting comments

`PostComments()` is the one comment-reuse algorithm; providers implement only `Commenter`'s
primitives. In order:

1. **HEAD check.** `GetChangeRequest()`; if `sha` is no longer the head, return without posting, so
   a slow run can't overwrite a newer comment. A lookup **error assumes HEAD** (a flaky API must not
   suppress the comment); an **empty head SHA assumes not HEAD**.
2. **Find earlier comments** (`ExistingComments()`): every comment containing
   `comment.Identifier()` and, when `CurrentUser()` returns a login, written by that login. **An
   empty login means "identity unknown": match by marker alone** — GitHub uses this under Actions and
   with the `github` connectivity bypass.
3. **Reuse in order.** Body *i* (wrapped with `comment.Wrap()`) edits existing comment *i*; extra
   bodies are created; an edit or create error aborts with that error.
4. **Outdate leftovers.** Remaining earlier comments are overwritten with
   `[Outdated argo-diff content]` + the marker rather than deleted, so a later run can reuse them.
   An error here is logged and skipped. An empty body list therefore clears every earlier comment.

`ListComments()` must return every page, oldest first, and leave out system comments (GitLab system
notes). Never compare comment bodies for equality — GitLab trims a trailing newline on save; match
with `strings.Contains` on the marker, as `ExistingComments()` does.

`comment_test.go` drives the algorithm with an in-memory fake `Commenter`.

## Rules

- This package must not import a provider package. Providers import it. It imports
  `internal/comment` for `Wrap()` / `Identifier()`.
