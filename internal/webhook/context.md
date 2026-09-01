# internal/webhook/

Parses GitHub webhook payloads into `EventInfo`, and verifies webhook signatures.

| File | Contents |
| ---- | -------- |
| `process.go` | `EventInfo`, `NewEventInfo()`, `ProcessPullRequest()`, `ProcessComment()` |
| `signature.go` | `VerifySignature()` — HMAC-SHA256 over the raw body |

## EventInfo

The struct every other package passes around. Its JSON tags are also the **file format** for
`go run cmd/main.go -f event.json` and the `/dev` endpoint, so renaming a tag is a breaking change
for local workflows and is documented in `README.md`:

```go
Ignore  `json:"ignore"`        RepoOwner `json:"owner"`     RepoName  `json:"repo"`
RepoDefaultRef `json:"default_ref"`  Sha `json:"commit_sha"`  PrNum `json:"pr"`
ChangeRef `json:"change_ref"`  BaseRef `json:"base_ref"`
Refresh `json:"refresh"`       ChangedFiles `json:"changed_files,omitempty"`
```

`NewEventInfo()` returns a **safe default**: `Ignore: true`, `PrNum: -1`. Every parse path starts
from it and only clears `Ignore` once the event is confirmed actionable, so an unrecognized payload
is dropped rather than processed.

`validateEventInfo()` requires owner, repo, and default ref; `Sha` and `ChangeRef` are only required
when `Refresh` is false, since a refresh re-reads them from the API.

## Event handling

- `ProcessPullRequest()` acts on the `opened` and `synchronize` actions, and on `edited` only when
  `changes.base.ref.from` is set — that is how GitHub reports a base-branch retarget (eg: when a
  stacked PR's parent branch merges and GitHub repoints it at `main`), as opposed to an ordinary
  title/body edit, which also sends `edited` but leaves `changes.base` empty. `baseRefChanged()`
  reads that field through nil-safe generated getters, so it needs no extra nil checks. Every other
  field is read through raw pointer dereferences — a malformed payload panics rather than erroring.
- `ProcessComment()` handles `issue_comment`: action must be `created`, the issue must be a PR
  (`PullRequestLinks != nil`), and the body must satisfy `github.IsRefreshComment()` (`argo diff` /
  `argo-diff`, optionally suffixed with the context string). It sets `Refresh: true`, leaving the
  sha and refs to be resolved from the API. This is the only import of `internal/github` from here.

## Signatures

`VerifySignature()` requires a non-empty secret, the exact `sha256=` + 64 hex chars length, and
compares with `hmac.Equal`. It is skipped entirely in dev mode by the server.

## Tests

`webhook_testdata/` holds real captured payloads: `payload-pr-open.json`, `payload-pr-sync.json`,
`payload-pr-close.json`, `payload-comment-created.json`, `payload-comment-argodiff-created.json`.
`payload-pr-edited-base.json` and `payload-pr-edited-title.json` are **derived**, not captured —
copies of `payload-pr-sync.json` with `action` changed to `edited` and a `changes` block added (base
retarget vs. title-only) to exercise `baseRefChanged()`. `process_test.go` asserts which of them are
ignored vs. actionable; `signature_test.go` covers the bad-length, bad-prefix, and valid cases.

The `github.com/google/go-github/v90` types are used to unmarshal payloads — a major-version bump of
that dependency changes this import path.
