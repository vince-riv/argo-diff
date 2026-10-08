# GitLab fixtures (issue #160, Phase 0)

Captured on 2026-10-08 from gitlab.com `19.5.0-pre` against the test project `vrivellino/argo-diff`
(id 86684588), test MR !1 (`spike/phase0-gitlab` -> `main`). Phase 0 results and how they were
obtained: https://github.com/vince-riv/argo-diff/issues/160.

These fixtures were staged on the branch `test/gitlab-phase0-fixtures` and committed here in
PR 2.0 of the plan. The webhook fixtures stay here, next to the GitLab webhook parser (Phase 4).

## Sanitization

- Email addresses replaced with `user@example.com` / `user@users.noreply.example.com`.
- Gravatar hashes (derived from the email) replaced with 64 zeros.
- API `.headers` files keep only the status line, `content-type`, `link`, and the `x-page` /
  `x-next-page` / `x-prev-page` / `x-per-page` / `x-total` / `x-total-pages` pagination headers.
- Webhook `.headers` files drop the receiver's `Host`, Cloudflare, and proxy headers. The request
  line is rewritten to `POST /webhook/gitlab`.
- Usernames, numeric IDs, SHAs, and gitlab.com URLs are real and public; they are kept so author
  matching and URLs stay realistic.

## Webhook signatures are re-signed with a test key

Sanitizing a webhook body invalidates GitLab's original `webhook-signature`. Every webhook fixture
is re-signed over its sanitized `.json` body (bytes exactly as stored, no trailing newline) with
this **test** signing token:

```
whsec_YXJnby1kaWZmLXRlc3Qtc2lnbmluZy1rZXktMDAwMQ==
```

Scheme (Standard Webhooks, GitLab 19.0+): `key = base64decode(token without "whsec_")`,
`signature = "v1," + base64(HMAC-SHA256(key, webhook-id + "." + webhook-timestamp + "." + body))`.
`content-length` matches the sanitized body. No `x-gitlab-token` header is present (the capture
hook had no secret token configured); tests for the shared-secret path add one themselves.

## `api/` - REST API responses (`<name>.json` + `<name>.headers`)

| Fixture | Request | Notes |
| --- | --- | --- |
| `version` | `GET /version` | |
| `user` | `GET /user` | Human PAT user (`bot: false`) |
| `mr` | `GET /projects/:id/merge_requests/1` | Head `b27a067`, `diff_refs` |
| `mr-diffs-p1` | `GET .../merge_requests/1/diffs?per_page=3&page=1` | Rename `post-local.sh` -> `scripts/post-local.sh` (`renamed_file: true`); `link` rel="next", `x-next-page: 2` |
| `mr-diffs-p2` | same, `page=2` | Last page: `x-next-page` empty, no rel="next" |
| `mr-notes` | `GET .../merge_requests/1/notes` | argo-diff marker note, 3 plain notes, 1 system note (`added 1 commit`) |
| `note-create` | `POST .../notes` | 201 |
| `note-update` | `PUT .../notes/:id` | 200, `[Outdated argo-diff content]` body (GitLab trims the trailing newline) |
| `note-create-too-long` | `POST .../notes`, 1,048,577 bytes | 400 body only. Limit is 1 MiB in bytes |
| `note-create-too-long-multibyte` | `POST .../notes`, 1,000,000 chars of `€` | 400 body only |
| `status-pending`, `status-success` | `POST /projects/:id/statuses/:sha`, `name=argo-diff` | Same `id`: one status per name + SHA, updated in place |
| `status-failed` | same, `name=argo-diff/ctx`, `state=failed` | |
| `status-badstate` | same, `state=failure` | 400 `state does not have a valid value` |
| `status-desc256` | same, 256-char description | 400; max is 255 |
| `status-list` | `GET /projects/:id/repository/commits/:sha/statuses` | |

## `ci/` - GitLab CI environment

| Fixture | Notes |
| --- | --- |
| `ci-env-merge-request-detached.env` | All `CI_*` / `GITLAB_*` vars from a detached MR pipeline (`CI_PIPELINE_SOURCE=merge_request_event`, `CI_MERGE_REQUEST_EVENT_TYPE=detached`). Names containing TOKEN, PASSWORD, SECRET, KEY, JWT, or AUTH are `<redacted>` (this includes `CI_COMMIT_AUTHOR`) |

Not captured: a merged-results pipeline (needs Premium).

## `webhook/` - webhook requests (`<name>.json` body + `<name>.headers`)

All captured from MR !1 and issue #1 on the test project (2026-10-08). None are derived.

| Fixture | `X-Gitlab-Event` | Case | argo-diff should |
| --- | --- | --- | --- |
| `merge-request-open` | Merge Request Hook | `action: open` | run |
| `merge-request-update-new-commits` | Merge Request Hook | `action: update`, `oldrev` set, `changes: {}` | run |
| `merge-request-update-target-branch` | Merge Request Hook | `action: update`, `changes`: `target_branch`, `merge_status`, `updated_at` | run |
| `merge-request-reopen` | Merge Request Hook | `action: reopen`, `changes.state_id` 2 -> 1 | run |
| `merge-request-update-title` | Merge Request Hook | `action: update`, `changes`: `title`, `updated_at`, `updated_by_id`; no `oldrev` | ignore |
| `merge-request-update-draft-ready` | Merge Request Hook | `action: update`, `changes`: `draft`, `title`, `updated_at` (Draft -> ready) | ignore |
| `merge-request-close` | Merge Request Hook | `action: close`, `state: closed`, `changes.state_id` | ignore |
| `merge-request-update-after-close` | Merge Request Hook | `action: update`, `state: closed`, `changes.state_id`: GitLab sends this right after `close` | ignore |
| `merge-request-update-after-reopen` | Merge Request Hook | `action: update`, `changes`: `merge_status`, `state_id`: GitLab sends this right after `reopen` | ignore |
| `merge-request-merge` | Merge Request Hook | `action: merge`, `state: merged`, `changes`: `merge_commit_sha`, `state_id` (merged into scratch branch `phase0-target`) | ignore |
| `note-merge-request-create` | Note Hook | `noteable_type: MergeRequest`, `action: create`, argo-diff-style body (no refresh keyword) | ignore |
| `note-merge-request-refresh` | Note Hook | `noteable_type: MergeRequest`, `action: create`, body `argo-diff` (a default refresh keyword) | refresh |
| `note-merge-request-update` | Note Hook | `action: update`: edit to `[Outdated argo-diff content]` + marker | ignore |
| `note-issue` | Note Hook | `noteable_type: Issue`, `issue` object instead of `merge_request`, body `argo-diff` | ignore |

The "argo-diff should" column is the intended PR 4.1 behavior, not a capture fact.

Observed but not kept:
- Note Hook `create` for the three ~1 MiB length-test notes. Each payload was ~2 MB, because the
  note text appears twice (`object_attributes.note` and `object_attributes.description`). The
  capture endpoint truncated them.
- Four more `action: update` Note Hooks, same shape as `note-merge-request-update`.
- No Note Hook was sent for the `added 1 commit` system note.
- No Issue Hook: issue events were not enabled on the capture webhook.
