# internal/server/

Entry points that turn an incoming event into a `process_event.ProcessCodeChange()` call.

| File | Contents |
| ---- | -------- |
| `http_server.go` | The webhook HTTP server and its handlers |
| `run_once.go` | GitHub Actions and event-file modes, plus env logging |

## http_server.go

`StartWebhookProcessor(addr, secret, devMode)` registers handlers on the **default** `http.ServeMux`
(`http.HandleFunc`), starts the listener in a goroutine, blocks on `SIGTERM`/`SIGINT`, then does a
30s graceful shutdown followed by `wg.Wait()` so in-flight event processing finishes.

Routes:

| Path | Handler | Notes |
| ---- | ------- | ----- |
| `/webhook` | `handleWebhook` | The real endpoint |
| `/webhook_log` | `printWebHook` | Logs the payload; verifies the signature but does nothing else |
| `/healthz` | `healthZ` | Returns `healthy` |
| `/dev` | `devHandler` | Registered only in dev mode; accepts a raw `EventInfo` JSON POST |

`handleWebhook` verifies `X-Hub-Signature-256` (skipped in dev mode), then dispatches on
`X-GitHub-Event`:

- `ping` → acknowledged.
- `pull_request` → `webhook.ProcessPullRequest()`.
- `issue_comment` → `webhook.ProcessComment()` (the `argo diff` refresh trigger).
- anything else → ignored with a 200.

An `EventInfo` with `Ignore` set is answered 200 and dropped. Otherwise processing is dispatched to
a goroutine and the response returns immediately — GitHub gets its ack long before the diff
finishes, and the returned error is intentionally discarded (`ignoredError`) since there is nobody
left to report it to.

## run_once.go

- `eventInfoFromEnv()` builds the event from GitHub Actions' variables: requires
  `GITHUB_EVENT_NAME=pull_request`, parses the PR number out of `GITHUB_REF`
  (`refs/pull/<n>/merge`), splits `GITHUB_REPOSITORY`, and reads `REPO_DEFAULT_REF`,
  `GITHUB_HEAD_REF`, `GITHUB_BASE_REF`. It sets `Refresh: true` so the sha and refs are re-read
  from the API rather than trusted from the environment.
- `eventInfoFromFile()` decodes an `EventInfo` JSON document; `-` reads stdin.
- **`ProcessGithubAction()` passes `devMode=true`.** That is not a bug: dev mode's only remaining
  effect at that point is dry-running commit statuses, which `github.Status()` already skips under
  Actions. Comments are still posted.
- `logEnvironmentVariables()` dumps configuration at debug level, redacting the sensitive vars to
  their first three characters. **Add new env vars to one of its two lists** when you introduce
  them.

## Tests

None in this package. The handlers are thin; the logic they call is tested in `webhook/`,
`argocd/`, `github/`, and `process_event/`.
