# internal/

All application code beyond the entry point. Each package has its own `context.md`.

## Packages and dependency direction

```
cmd/main.go
  ├── internal/server ──── internal/process_event ─┬── internal/argocd ── internal/webhook
  │       ├── internal/scm                         ├── internal/comment ── internal/config
  │       └── internal/webhook                     ├── internal/ignorable ── internal/config
  │                                                ├── internal/scm ─┬─ internal/comment
  │                                                │                 └─ internal/webhook
  │                                                └── internal/webhook
  ├── internal/github ─┬─ internal/comment, internal/config, internal/webhook
  │                    └─ internal/scm   (github.Provider implements scm.Provider)
  ├── internal/scm      (registers the providers)
  └── internal/argocd, internal/github  (connectivity checks)

internal/webhook  (imports nothing internal)
internal/gendiff  (no importers — see its context.md)
```

| Package | Role |
| ------- | ---- |
| `argocd/` | Runs the `argocd` CLI; matches applications to a change and diffs them |
| `comment/` | Renders diffs into comment bodies (per-provider `Dialect`), plus the preamble/marker wrapper |
| `github/` | The GitHub provider (`github.Provider`): API client, PR comments, commit statuses, PR/file lookups, webhook parsing and signatures, GitHub Actions detection |
| `ignorable/` | Decides which resource diffs are "ignorable" (every changed line matches a regex), so `comment/` can fold them |
| `process_event/` | Orchestrates one event end to end, including the timeout budget |
| `scm/` | The `Provider` interface and registry, neutral types (`RepoRef`, `ChangeRequest`, `Status`), and the comment-reuse algorithm over provider primitives; imports no provider |
| `server/` | HTTP webhook handlers and the two run-once entry points |
| `webhook/` | `EventInfo`, the provider-neutral event everything passes around (parsing lives in each provider) |
| `gendiff/` | Unified-diff helper, currently unused |

`webhook.EventInfo` is the value that flows through the whole pipeline; if you add a field, check
every producer: `github.ProcessPullRequest`, `github.ProcessComment`, `github.Provider.EventFromCIEnv`,
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
  `github` package's `commentClient` / `statusClient` (swapped for `httptest`-backed clients). The
  `scm.Commenter` interface lets provider-neutral code run against an in-memory fake.
- **Errors** are logged where they occur and returned upward; the top-level orchestrator decides
  whether one becomes a failed commit status, a PR comment warning, or a process exit code.
- **Fixtures** live in `<pkg>_testdata/` beside each package. `go.yml` triggers on `**/_testdata/**`
  as well as `**/*.go`.
