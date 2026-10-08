# internal/webhook/

Holds `EventInfo`, the provider-neutral event every other package passes around. Despite the
package name, parsing webhook payloads is each provider's job: GitHub's is in `internal/github`
(`webhook.go`), behind the `scm.WebhookHandler` interface. This package imports nothing internal.

| File | Contents |
| ---- | -------- |
| `process.go` | `EventInfo`, `NewEventInfo()`, `EventInfo.Validate()` |

## EventInfo

Its JSON tags are also the **file format** for `go run cmd/main.go -f event.json` and the `/dev`
endpoint, so renaming a tag is a breaking change for local workflows and is documented in
`README.md`:

```go
Provider `json:"provider"`
Ignore  `json:"ignore"`        RepoOwner `json:"owner"`     RepoName  `json:"repo"`
RepoDefaultRef `json:"default_ref"`  Sha `json:"commit_sha"`  PrNum `json:"pr"`
ChangeRef `json:"change_ref"`  BaseRef `json:"base_ref"`
Refresh `json:"refresh"`       ChangedFiles `json:"changed_files,omitempty"`
```

`Provider` names the `scm` registry entry that handles the event. **Empty means
`scm.DefaultProvider` (`github`)**, so event files written before multi-provider support keep
working. Webhook and CI producers set it; `-f` files and `/dev` posts may omit it.

`NewEventInfo()` returns a **safe default**: `Ignore: true`, `PrNum: -1`. Every parse path starts
from it and only clears `Ignore` once the event is confirmed actionable, so an unrecognized payload
is dropped rather than processed.

`Validate()` requires owner, repo, and default ref; `Sha` and `ChangeRef` are only required when
`Refresh` is false, since a refresh re-reads them from the API. Provider parsers call it.

## Tests

None here: the struct has no logic beyond `Validate()`, which the provider parsers' tests cover.
