# internal/gitlab/

The GitLab provider: `Provider` will implement `scm.Provider` on top of the GitLab REST API, the
way `internal/github` does for GitHub. It is being built in issue #160, Phase 2; until
`Provider` implements the whole interface it is not registered in `cmd/main.go`. Built on
`gitlab.com/gitlab-org/api/client-go`.

## Files

| File | Contents |
| ---- | -------- |
| `client.go` | Client construction and config, `ConnectivityCheck()`, `getCurrentUser()`, API error logging |
| `merge_request.go` | `Provider.GetChangeRequest()`, `Provider.ListChangedFiles()` |
| `notes.go` | The comment primitives (`ListComments`, `CreateComment`, `UpdateComment`, `CurrentUser`) on MR notes, and `Dialect()` |
| `status.go` | `Provider.SetStatus()` — commit statuses |
| `provider.go` | `Provider` — the startup methods so far: `Enabled`, `ValidateConfig`, `CredentialsHint`, `ConnectivityCheck`, `RepoHosts` |

client-go types stay inside this package, as go-github types do in `internal/github`.

## Configuration

Read in `init()`, like every other package (see `internal/context.md`):

- `GITLAB_TOKEN` — a personal, project or group access token with `api` scope, or a fine-grained
  token with the MR, note and commit-status permissions. It enables the provider. `CI_JOB_TOKEN`
  **cannot** be used: it cannot create notes, post statuses or call `GET /user` (Phase 0).
- `GITLAB_BASE_URL` — the instance, eg: `https://gitlab.example.com`; falls back to
  `CI_SERVER_URL` (set in GitLab CI jobs), then `https://gitlab.com`. client-go appends `/api/v4`.
  A relative URL root (`https://example.com/gitlab`) works.
- `GITLAB_CA_FILE` — a PEM bundle of extra CAs to trust for a self-managed instance, on top of the
  system pool; falls back to `CI_SERVER_TLS_CA_FILE` (set in GitLab CI jobs).

A bad base URL or CA file leaves `client` nil and stores the reason in `clientErr`;
`ValidateConfig()` returns it, so startup fails with the real cause rather than a nil-client error.
`RepoHosts()` is the base URL's hostname plus its relative URL root, if any.

## API calls

- **Projects are addressed by their full path**, `RepoRef.FullPath()` (eg: `group/sub/project`);
  client-go URL-encodes it (`group%2Fsub%2Fproject`). Never split it to find an owner.
- Every call passes its context with `gitlab.WithContext(ctx)`. client-go retries 429 and 5xx
  responses itself.
- **Never log `GITLAB_TOKEN`.** client-go keeps it in a request header, and its errors name only
  the method, URL and status. `internal/server`'s `logEnvironmentVariables()` redacts it.
- A fine-grained token that lacks a permission gets 403 `insufficient_granular_scope`, and the
  body's `error_description` names the missing permission. `logAPIError()` logs that text as-is
  in an `error_description` field.

## Merge requests

- A change request number is the MR's **IID** (`!1`), not its global `id`.
- `GetChangeRequest()` maps the MR's `sha`, `source_branch` and `target_branch` onto
  `scm.ChangeRequest`.
- `ListChangedFiles()` reads **every page** of `/merge_requests/:iid/diffs` (100 per page,
  following `X-Next-Page`). A renamed file contributes its old **and** new path, deduplicated,
  so the `manifest-generate-paths` filter sees either. (GitHub's `ListPullRequestFiles()` reads
  one page only; see issue #160.)

## Notes (comments)

argo-diff's comments on a merge request are **notes**. `scm.PostComments()` runs the shared
reuse algorithm over the primitives in `notes.go` (see `internal/scm/context.md`).

- `ListComments()` reads every page (100 per page, `order_by=created_at&sort=asc`, so oldest
  first) and **leaves out system notes** (`system: true`, eg: "added 1 commit").
- `UpdateComment()` addresses a note through its MR: `PUT .../merge_requests/:iid/notes/:id`.
- GitLab **trims a trailing newline** from a note body on save. Never compare bodies for
  equality; the marker match in `scm.ExistingComments()` uses `strings.Contains`.
- The API gives notes no URL; `toComment()` builds `<base>/<path>/-/merge_requests/<iid>#note_<id>`
  for log lines.
- `CurrentUser()` is the token's username (`getCurrentUser()`). With the `gitlab` connectivity
  check bypassed it returns `""`, so notes are matched by marker alone, as GitHub does with its
  bypass.
- `Dialect()` is `comment.GitLab`: a 1,048,576-**byte** hard max (GitLab answers 400 above it),
  alerts hoisted out of `<details>` like GitHub. See `internal/comment/context.md`.

## Commit statuses

`SetStatus()` posts `POST /projects/:id/statuses/:sha`.

- **States are mapped**: `pending` and `success` pass through; `failure` and `error` become
  `failed`. GitLab rejects `failure` with a 400 (its states are `pending`, `running`, `success`,
  `failed`, `canceled`, `skipped`).
- The status `name` is `argo-diff`, or `argo-diff/<ARGO_DIFF_CONTEXT_STR>` (read in `init()`),
  the same string GitHub uses as its context. GitLab keeps **one status per name and SHA** and
  updates it in place.
- The description is cut to **255 characters** (runes, not bytes), ending in `...`; GitLab
  answers 400 above that.
- `dryRun` (dev mode) logs instead of calling the API. Skipping statuses under GitLab CI is
  Phase 3 of issue #160.

## Connectivity check and user

`ConnectivityCheck()` resolves the token's user with `GET /user`; `ARGO_DIFF_BYPASS_CONNECTIVITY_CHECKS=gitlab`
skips it (see `internal/config/context.md`). `getCurrentUser()` caches the username behind a
mutex held across the call, so concurrent first callers make one request. Project and group access
tokens act as bot users, which have a username like any other.

## Tests

`fixtures_test.go` has `newFixtureServer()`: an `httptest` server that answers listed routes with
`gitlab_testdata/api/<name>.json` (status line and headers from `<name>.headers`, with gitlab.com
URLs rewritten to the server), records every request, and fails the test on an unlisted one. It
points the package `client` at the server and resets the cached user for the test. Routes match
on the **escaped** path, eg: `apiPath("merge_requests/1")` is
`/api/v4/projects/vrivellino%2Fargo-diff/merge_requests/1`. The captured notes carry the marker
`argo-diff[test]`; the server swaps it for `comment.Identifier()` so `TestPostComments` finds
the captured argo-diff note as its own and edits it.

## Fixtures

`gitlab_testdata/` holds real, sanitized captures from gitlab.com (Phase 0 of issue #160):

- `api/`: REST API responses (`<name>.json` body + `<name>.headers` status line and pagination
  headers).
- `webhook/`: webhook requests (`<name>.json` body + `<name>.headers`), re-signed with a documented
  **test** signing key.
- `ci/`: the `CI_*` / `GITLAB_*` environment of a detached merge request pipeline.

`gitlab_testdata/README.md` lists every file, how it was sanitized, the test signing key, and the
expected argo-diff behavior for each webhook. Webhook `.json` bodies have no trailing newline on
purpose: the signatures cover the exact bytes, so never reformat them.
