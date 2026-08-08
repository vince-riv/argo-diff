# internal/

All application code beyond the entry point. Each package has its own `context.md`.

## Packages and dependency direction

```
cmd/main.go
  ├── internal/server ──── internal/process_event ─┬── internal/argocd ── internal/webhook
  │       └── internal/webhook                     ├── internal/github
  │                                                └── internal/webhook
  └── internal/argocd, internal/github  (connectivity checks only)

internal/webhook ── internal/github   (only for IsRefreshComment)
internal/gendiff  (no importers — see its context.md)
```

| Package | Role |
| ------- | ---- |
| `argocd/` | Runs the `argocd` CLI; matches applications to a change and diffs them |
| `github/` | GitHub API client: PR comments, commit statuses, PR/file lookups |
| `process_event/` | Orchestrates one event end to end, including the timeout budget |
| `server/` | HTTP webhook handlers and the two run-once entry points |
| `webhook/` | `EventInfo` (the event data structure everything passes around) and HMAC checks |
| `gendiff/` | Unified-diff helper, currently unused |

`webhook.EventInfo` is the value that flows through the whole pipeline; if you add a field, check
every producer: `webhook.ProcessPullRequest`, `webhook.ProcessComment`, `server.eventInfoFromEnv`,
`server.eventInfoFromFile`, and the `/dev` handler.

## Conventions

- **Logging** is `github.com/rs/zerolog/log` throughout, with `Msgf` for formatting. `log.Trace()`
  carries argument dumps, `log.Debug()` filtering decisions, `log.Info()` API/CLI calls.
  Never log `ARGOCD_AUTH_TOKEN` or GitHub tokens — the existing code redacts them explicitly.
- **Configuration is read in `init()`** into package-level vars. Tests therefore cannot influence
  behavior by setting env vars at test time (`t.Setenv` after package load is too late for anything
  captured in `init()`); they assign to the package vars directly instead. Functions that read env
  vars on each call (`gitRepoMatch`, `checkSource`, `processTimeout`) *are* `t.Setenv`-testable.
- **Seams for mocking** are package-level `var`s: `argocd.execArgoCdCli` (the CLI), and the
  `github` package's `commentClient` / `statusClient` (swapped for `httptest`-backed clients).
- **Errors** are logged where they occur and returned upward; the top-level orchestrator decides
  whether one becomes a failed commit status, a PR comment warning, or a process exit code.
- **Fixtures** live in `<pkg>_testdata/` beside each package. `go.yml` triggers on `**/_testdata/**`
  as well as `**/*.go`.
