# internal/gitlab/

The GitLab provider: `Provider` implements `scm.Provider` on top of the GitLab REST API, the way
`internal/github` does for GitHub, and `cmd/main.go` registers it when `ARGO_DIFF_SCM_PROVIDERS`
lists `gitlab` (the default is `github` only). `GITLAB_TOKEN` must then be set.
Built on `gitlab.com/gitlab-org/api/client-go/v3`.

**Today it runs from `-f` event files (and `/dev`) only.** GitLab CI detection is Phase 3 of
issue #160 and webhooks are Phase 4; until then `DetectCI()` is false, `EventFromCIEnv()` errors,
and `WebhookHandler` is a placeholder whose `Verify()` rejects every request. Its `CheckConfig()`
only warns, so a server with `GITLAB_TOKEN` set still starts and serves GitHub.

## Files

| File | Contents |
| ---- | -------- |
| `client.go` | Client construction and config, `ConnectivityCheck()`, `getCurrentUser()`, API error logging |
| `merge_request.go` | `Provider.GetChangeRequest()`, `Provider.ListChangedFiles()` |
| `notes.go` | The comment primitives (`ListComments`, `CreateComment`, `UpdateComment`, `CurrentUser`) on MR notes, and `Dialect()` |
| `status.go` | `Provider.SetStatus()` — commit statuses |
| `provider.go` | `Provider` — the startup methods (`Enabled`, `ValidateConfig`, `CredentialsHint`, `ConnectivityCheck`, `RepoHosts`), and the CI and webhook placeholders |

client-go types stay inside this package, as go-github types do in `internal/github`.

## Event files

A `-f` event names the provider and the project's namespace path as the owner, which may hold
nested groups (see README's "Craft a local file with event data"):

```json
{"provider": "gitlab", "owner": "group/subgroup", "repo": "project", "default_ref": "main", "pr": 42, "refresh": true}
```

`pr` is the MR **IID**. `process_event` builds `scm.RepoRef{Owner, Name}`, and every call here
uses `RepoRef.FullPath()`. ArgoCD sources match on the GitLab host (`RepoHosts()`) or through
`internal/argocd`'s host-agnostic `/owner/repo` suffix fallback, both of which handle an owner
with `/` in it.

## Configuration

Read in `init()`, like every other package (see `internal/context.md`):

- `GITLAB_TOKEN` — a personal, project or group access token with `api` scope, or a fine-grained
  token with the MR, note and commit-status permissions. Required when `ARGO_DIFF_SCM_PROVIDERS`
  lists `gitlab`. `CI_JOB_TOKEN`
  **cannot** be used: it cannot create notes, post statuses or call `GET /user` (Phase 0).
- `GITLAB_BASE_URL` — the instance, eg: `https://gitlab.example.com`; falls back to
  `CI_SERVER_URL` (set in GitLab CI jobs), then `https://gitlab.com`. client-go appends `/api/v4`.
  A relative URL root (`https://example.com/gitlab`) works.
- `GITLAB_CA_FILE` — a PEM bundle of extra CAs to trust for a self-managed instance, on top of the
  system pool; falls back to `CI_SERVER_TLS_CA_FILE` (set in GitLab CI jobs).

**The client is built by `Provider.ValidateConfig()`, not in `init()`.** `cmd/main.go` calls it
only when `gitlab` is listed, so a stray `GITLAB_TOKEN` with a bad CA file or base URL in a
GitHub-only deployment builds nothing and logs nothing. A bad base URL or CA file makes
`ValidateConfig()` fail, which fails startup with the real cause. `init()` only resolves
`webBaseURL`.
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
  the same string GitHub uses as its context.
- **State machine.** GitLab updates a status of that name on the SHA in place only while it is
  still `pending` or `running`; a finished one gets a new record. So pending → success updates in
  place, and a re-run after success starts a new record. But **pending → pending is a 400**
  (`Cannot transition status via :enqueue from :pending`), which happens when a run died before its
  final status (the timeout path in `process_event`) or two runs overlap on one SHA (webhook plus a
  refresh note, from Phase 4). `alreadyPending()` treats that one 400 as success when the requested
  state is `pending`, so the logs don't report a failure for a status that is already right. The
  test body for it is derived from GitLab's source, not captured.
- **No `ref` or `pipeline_id` is sent**, so GitLab attaches the status to the first branch of the
  project that contains the SHA, and answers `404 References for commit Not Found` when none does.
  Known limits (review on #359), to address with Phase 4:
  - **Fork MRs:** the head SHA lives only in the fork, so every status call on a fork MR is
    expected to 404. `process_event` only logs status errors, so this fails quietly. Untested: the
    Phase 0 captures are all same-project; Phase 4 fixtures should capture a fork MR.
  - **Same-project MRs:** when the SHA is on several branches, the status lands on whichever comes
    first, and it never joins the MR's pipeline (`refs/merge-requests/<iid>/head`), so under
    "Pipelines must succeed" it may not gate the MR.
  - Likely fix: send `pipeline_id` = the MR's `head_pipeline_id`, or at least `ref` = its
    `source_branch`. Both need the MR to reach `SetStatus()`, so it is an `scm.Provider` change.
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
