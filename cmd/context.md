# cmd/

Application entry point. A single file, `main.go` — there is no other command in this module, and
`.goreleaser.yaml` builds `./cmd` into the `argo-diff` binary.

## What `main.go` does

`init()`:

- Sets the global zerolog level from `LOG_LEVEL` (`panic`…`trace`, default `info`).
- Resolves `Version`. It is set via `-ldflags "-X 'main.Version=...'"` at build time; when it is
  still `dev`, `init()` shells out to
  `git describe --always --dirty --exclude 'chart-*' --exclude 'actions-*'` and falls back to `dev`
  if that fails. The `--exclude` flags keep chart and action tags out of the version string.
- Registers flags via `github.com/spf13/pflag`: `-H/--host`, `-p/--port` (default 8080),
  `-f/--event-file`.

`main()` validates the environment and then dispatches, in this order:

1. Fatals unless `ARGOCD_AUTH_TOKEN` and `ARGOCD_SERVER_ADDR` are set.
2. `registerProviders()` registers what `selectProviders(knownProviders)` returns
   (`github.Provider{}`, `gitlab.Provider{}`), and fatals on its error. **`ARGO_DIFF_SCM_PROVIDERS`
   decides which providers run** (`config.ScmProviders()`; unset or empty means `github`):
   - a provider **not in the list is skipped**, even with its credentials set, and its
     `ValidateConfig()` never runs;
   - a listed provider whose `Enabled()` is false (no credentials) is an error naming its
     `CredentialsHint()`. When the list is the default, the error also says to set
     `ARGO_DIFF_SCM_PROVIDERS` (eg: `gitlab`), since a GitLab-only operator hits this first;
   - an **unknown name in the list is an error**, unlike the warn-only bypass list: this list
     decides what runs, so a typo must not turn a provider off without a word;
   - a listed provider must pass `ValidateConfig()`.
   For GitHub, any of `GITHUB_PERSONAL_ACCESS_TOKEN`, `GITHUB_TOKEN` or the `GITHUB_APP_*` trio
   counts as credentials, and with no token all three App variables are required. For GitLab,
   `GITLAB_TOKEN` does, and `ValidateConfig()` builds the client, failing on a malformed
   `GITLAB_BASE_URL` or an unreadable `GITLAB_CA_FILE`. `main_test.go` covers `selectProviders()`
   with stub providers.
3. `APP_ENV=dev` turns on dev mode.
4. `argocd.ConnectivityCheck()` — always runs, in every mode. It executes `argocd version`, so the
   `argocd` CLI must be on `PATH` (or named by `ARGOCD_CLI_CMD_NAME`) even for a run that would
   otherwise do nothing, and both client and server must be >= 2.12.0.
5. `argocd.SetRepoHosts(p.Name(), p.RepoHosts())` for every enabled provider, so each event
   matches application sources on its own provider's hosts (see `internal/argocd/context.md`).
6. **CI detection.** The first enabled provider whose `DetectCI()` is true (GitHub:
   `GITHUB_ACTIONS=true`) gets `server.ProcessCI(p)`, then return. Provider connectivity checks are
   deliberately skipped here. This branch also warns when `process_event.RequireAppMatch()` is true:
   `ARGO_DIFF_REQUIRE_APP_MATCH` is inert in CI, because `EventFromCIEnv()` always sets
   `EventInfo.Refresh` and the match check is gated behind `!Refresh`. That is the only reason `cmd`
   imports `internal/process_event` — the helper is exported so the parsing rules live in one
   place. See `internal/process_event/context.md` step 4.
7. Otherwise `ConnectivityCheck()` for every enabled provider, then:
   - `-f <file>` → `server.ProcessFileEvent()` and return.
   - else fatal unless every enabled provider's `WebhookHandler().CheckConfig()` passes (GitHub:
     `GITHUB_WEBHOOK_SECRET` is set), and start the webhook server.

Run-once modes (`ProcessCI`, `ProcessFileEvent`) print the error to stderr and `os.Exit(1)`; that
non-zero exit is how a CI step fails, since commit statuses are skipped in CI.

Adding a provider means adding it to `knownProviders`.

## Gotchas

- The env validation above happens in `main()`, but the `argocd` and `github` packages read their
  own configuration in *their* `init()` functions, which run first. Setting env vars from inside
  `main()` would be too late to affect them.
- `startServer()` maps an empty host + zero port to `:8080`.
- There is no test file here. Logic worth testing belongs in `internal/`.
