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
2. Fatals unless GitHub credentials exist: `GITHUB_PERSONAL_ACCESS_TOKEN` or `GITHUB_TOKEN`, else
   all three of `GITHUB_APP_ID`, `GITHUB_APP_INSTALLATION_ID`, `GITHUB_APP_PRIVATE_KEY`.
3. `APP_ENV=dev` turns on dev mode.
4. `argocd.ConnectivityCheck()` — always runs, in every mode. It executes `argocd version`, so the
   `argocd` CLI must be on `PATH` (or named by `ARGOCD_CLI_CMD_NAME`) even for a run that would
   otherwise do nothing, and both client and server must be >= 2.12.0.
5. `GITHUB_ACTIONS=true` → `server.ProcessGithubAction()`, then return. The GitHub connectivity
   check is deliberately skipped here.
6. Otherwise `github.ConnectivityCheck()`, then:
   - `-f <file>` → `server.ProcessFileEvent()` and return.
   - else fatal unless `GITHUB_WEBHOOK_SECRET` is set, and start the webhook server.

Run-once modes (`ProcessGithubAction`, `ProcessFileEvent`) print the error to stderr and
`os.Exit(1)`; that non-zero exit is how a GitHub Actions step fails, since commit statuses are
skipped under Actions.

## Gotchas

- The env validation above happens in `main()`, but the `argocd` and `github` packages read their
  own configuration in *their* `init()` functions, which run first. Setting env vars from inside
  `main()` would be too late to affect them.
- `startServer()` maps an empty host + zero port to `:8080`.
- There is no test file here. Logic worth testing belongs in `internal/`.
