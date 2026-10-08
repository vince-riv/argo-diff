# internal/gitlab/

The GitLab provider (issue #160, Phase 2). Today this directory holds only the captured fixtures;
the provider code lands in the following Phase 2 PRs.

## Fixtures

`gitlab_testdata/` holds real, sanitized captures from gitlab.com (Phase 0 of issue #160):

- `api/`: REST API responses (`<name>.json` body + `<name>.headers` status line and pagination
  headers), served by `httptest` in this package's tests.
- `webhook/`: webhook requests (`<name>.json` body + `<name>.headers`), re-signed with a documented
  **test** signing key.
- `ci/`: the `CI_*` / `GITLAB_*` environment of a detached merge request pipeline.

`gitlab_testdata/README.md` lists every file, how it was sanitized, the test signing key, and the
expected argo-diff behavior for each webhook. Webhook `.json` bodies have no trailing newline on
purpose: the signatures cover the exact bytes, so never reformat them.
