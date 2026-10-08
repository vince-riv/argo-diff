# internal/scm/

The source-control-provider abstraction: neutral types that the rest of argo-diff uses instead of
any one provider's API types. GitHub (`internal/github`) is the only provider today; the plan in
issue #160 adds GitLab.

| File | Contents |
| ---- | -------- |
| `types.go` | `RepoRef`, `ChangeRequest`, `Status` and its four states |

## Types

- `RepoRef` is `Host` + `Owner` + `Name`. **`Owner` may contain `/`** (GitLab nested groups), so
  never split a full path on the first `/` to get it back. `Host` may be empty: the provider then
  uses its configured API host.
- `ChangeRequest` is a PR/MR reduced to number, head SHA, head ref and base ref. A field the
  provider did not return stays **empty rather than erroring**; each caller decides what a missing
  field means (`process_event` fails a refresh; the comment HEAD check treats it as "not HEAD").
- `Status` is `pending` / `success` / `failure` / `error`, the GitHub spelling. Providers map these
  onto their own states (GitLab rejects `failure` and wants `failed`). `Valid()` guards the set.

## Rules

- This package must not import a provider package. Providers import it.
