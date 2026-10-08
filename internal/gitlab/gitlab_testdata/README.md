# GitLab fixtures (issue #160, Phase 0)

Captured on 2026-10-08 from gitlab.com `19.5.0-pre` against the test project `vrivellino/argo-diff`
(id 86684588), test MR !1 (`spike/phase0-gitlab` -> `main`). Phase 0 results and how they were
obtained: https://github.com/vince-riv/argo-diff/issues/160.

This branch is a staging area. PR 2.0 brings the directory into `main`
(`git checkout <this-branch> -- internal/gitlab/gitlab_testdata`); later PRs may move the webhook
fixtures if webhook parsing lives elsewhere.

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

| Fixture | `X-Gitlab-Event` | Case |
| --- | --- | --- |
| `merge-request-open` | Merge Request Hook | `action: open` |
| `merge-request-update-new-commits` | Merge Request Hook | `action: update`, `oldrev` set, `changes: {}` |
| `note-merge-request-create` | Note Hook | `noteable_type: MergeRequest`, `action: create`, no refresh keyword |

Not captured yet: `reopen`, `close`, `merge`, `update` with title change only, `update` with
target-branch change, Note Hook with the refresh keyword, Note Hook on an issue. Anything added
later that is hand-written rather than captured must be listed here as derived.
