# AGENTS.md

Guidance for coding agents working in this repository. `CLAUDE.md` is a symlink to this file.

**Keep this file succinct.** It is loaded into every session, so it carries only what applies
repo-wide. Directory-specific detail belongs in that directory's `context.md`.

## Directory context files

Every top-level directory (dot directories excepted) and every package under `internal/` has a
`context.md` describing what lives there, how it works, and its gotchas. Read a directory's
`context.md` before changing files in it, and update it when that directory's layout or behavior
changes.

| Path | What it is |
| ---- | ---------- |
| `cmd/` | Entry point: flags, env validation, mode dispatch |
| `internal/argocd/` | Wrapper around the `argocd` CLI; application matching and diffing |
| `internal/config/` | Cross-package operator config, eg: the connectivity-check bypass |
| `internal/github/` | GitHub API client, PR comments, commit statuses |
| `internal/process_event/` | Orchestrates one event: diff → commit status → PR comment |
| `internal/server/` | HTTP webhook server plus the run-once entry points |
| `internal/webhook/` | Webhook payload parsing (`EventInfo`) and signature verification |
| `internal/gendiff/` | Unified-diff helper; currently unused by the rest of the code |
| `charts/` | Helm chart for deploying argo-diff, plus a fixture chart for e2e tests |
| `docs/` | Screenshots and example raw Kubernetes manifests |
| `scripts/` | `prep-release.sh` — bumps every version-pinned file before a release |
| `test/` | Fixtures and runner for the k3s end-to-end test |
| `temp/` | Gitignored scratch dir used by the Docker build and `post-local.sh` |

## Overview

Argo-diff posts a Pull Request comment showing what a change does to the Kubernetes manifests
ArgoCD delivers, by diffing the PR's revision against live cluster state. It runs either as a
long-lived GitHub webhook receiver or as a one-shot process (GitHub Actions, or a local event file).

**Only pull request events are supported.** `ProcessCodeChange()` rejects any event without a PR
number; push events are not handled.

**Runtime dependency:** argo-diff shells out to the `argocd` CLI binary for all cluster interaction
— it is not an HTTP API client. A compatible `argocd` executable must be on `PATH` (override the
command name with `ARGOCD_CLI_CMD_NAME`) for anything to work locally.

## Development commands

```bash
go build -v ./...                      # build
go run cmd/main.go                     # run (needs env vars, see below)
go run cmd/main.go -f event.json       # run once against a single event file ("-" reads stdin)
go fmt ./...                           # format
golangci-lint run                      # lint (golangci-lint v2; config in .golangci.yaml)
go test ./...                          # all tests
go test -run TestName ./internal/argocd/...
```

Requires Go 1.25+ (see `go.mod`). Builds inject the version via ldflags
(`-X 'main.Version=...'`, from `git describe`); a plain `go build`/`go run` falls back to running
`git describe` itself, defaulting to `dev`.

### Local environment

`README.md` holds the full table of environment variables and their GitHub Actions inputs — keep
that table as the source of truth rather than duplicating it here. The minimum for a local run:

```bash
GITHUB_PERSONAL_ACCESS_TOKEN='github_pat_XXXX'   # or GITHUB_TOKEN, or the GITHUB_APP_* trio
ARGOCD_AUTH_TOKEN='YOUR_ARGOCD_TOKEN'
ARGOCD_SERVER_ADDR='argocd.your.domain:443'
ARGOCD_UI_BASE_URL='https://argocd.your.domain'
APP_ENV='dev'
```

```bash
set -o allexport ; . .env.sh ; set +o allexport
go run cmd/main.go
```

Most packages read their configuration in `init()`, so environment changes after package load have
no effect — tests set the package-level variables directly instead.

## Operational modes

Selected in `cmd/main.go`, in this order:

1. **GitHub Actions** — `GITHUB_ACTIONS=true`: builds the event from `GITHUB_*` env vars, runs once,
   exits non-zero on failure. Commit statuses are skipped; the failed step is the signal.
2. **File processing** — `-f <file>`: decodes an `EventInfo` JSON document and runs once.
3. **Webhook server** — the default: serves `/webhook`, `/webhook_log`, and `/healthz`.
4. **Dev mode** — `APP_ENV=dev`: adds a `/dev` endpoint, disables webhook signature validation, and
   turns commit statuses into dry-run logging (comments are still posted).

## Testing

`go test ./...`. Fixtures live in `*_testdata/` directories next to the packages that use them
(`internal/argocd/argocd_testdata/`, `internal/github/github_testdata/`,
`internal/webhook/webhook_testdata/`) and are captured from real `argocd` CLI output and GitHub API
responses. Package-level `var` seams (`execArgoCdCli`, the `github` package clients) are the
mocking points — see the relevant `context.md`.

## CI and releases

Workflows in `.github/workflows/`:

- **go.yml** — build, `go fmt`, tests, and lint (golangci-lint v2, `only-new-issues`; PRs only).
- **dockerbuild.yml** — GoReleaser build + multi-arch image publish to `ghcr.io`.
- **k3s.yml** — end-to-end test on a k3s cluster with ArgoCD, using `test/` (see `test/context.md`).
- **release.yml** — cuts a release on an `X.Y.Z[-suffix]` tag and moves the floating `vX` /
  `actions-vX` tags.
- **helm.yml** / **chart-releaser.yml** — chart lint/unittest, and publish on a `chart-X.Y.Z` tag.

Release prep is `scripts/prep-release.sh <version>` (needs `helm-docs` on `PATH`); `release.yml`
fails if `action.yml` is not pinned to the tagged version.

## Conventions

### Commit messages

- Conventional-commit subject (`feat:`, `fix:`, `chore:`, `docs:`), imperative mood; use the body to
  explain *why*, not just what.
- Coding agents **must** add an `Assisted-by:` trailer naming the agent and the model that did
  the work — not a fixed example model, the one actually running in the session (it can change
  mid-session, eg: via `/model`):

  ```
  Assisted-by: Claude Code (<model-id>)
  ```

- **Never add a `Signed-off-by:` trailer** to commits or pull requests in this repository.

### Deployment surfaces

Changes to configuration usually need to land in more than one place: `README.md`'s env var table,
`action.yml` inputs, `charts/argo-diff/values.yaml` (+ chart templates and unittests), and
`docs/k8s/`. Check all of them before calling a config change done.
