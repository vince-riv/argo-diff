---
name: prep-release
description: Prepare an argo-diff release. Recommends a major/minor/patch bump from the commit log and changed files, runs scripts/prep-release.sh on a chore/prep-release-X.Y.Z branch, and opens a PR with the commit list and draft release notes. Never tags. Use when asked to prep, cut, or prepare a release.
---

# prep-release

Prepare a release PR. **Never run `git tag` or push a tag.** The user tags after the PR merges.

Follow `AGENTS.md` conventions throughout (commit trailers, no `Signed-off-by:`, no hard-wrapped PR bodies).

## 1. Preconditions

- `git fetch origin --tags`. Stop if the fetch fails.
- `helm-docs` must be on `PATH` (`command -v helm-docs`). Stop and tell the user if not.
- The working tree must be clean (`git status --porcelain`). Stop if not.
- Work from `origin/main`: the release covers what is on `origin/main`, not local state.

## 2. Find the last release

Only stable `X.Y.Z` tags count. Skip prerelease tags (`X.Y.Z-rc1`) and the floating `vN` tags.

```bash
LAST=$(git describe --tags --abbrev=0 --match '[0-9]*.[0-9]*.[0-9]*' --exclude '*-*' origin/main)
git log --oneline "$LAST..origin/main"
```

Stop if there are no commits since `$LAST`.

## 3. Recommend a bump

Do it in two passes. State both results, then give one recommendation.

**Pass 1 - commit log.** Use conventional-commit subjects:

- `!` after the type, or a `BREAKING CHANGE` footer → **major**.
- Any `feat:` → **minor**.
- Only `fix:`, `chore:`, `docs:`, `ci:`, `test:`, `refactor:`, dependency bumps → **patch**.

**Pass 2 - sanity check against files changed.** The log can lie (a `refactor:` can break users). Run:

```bash
git diff --stat "$LAST..origin/main"
git diff "$LAST..origin/main" -- action.yml charts/ cmd/ internal/config
```

Check what users can observe. Anything that changed here and is not purely additive means major:

- **Env vars:** compare the set of `Getenv(...)`/`LookupEnv(...)` names in non-test Go code at `$LAST` and at `origin/main` (`git grep -ho 'Getenv("[A-Z_0-9]*")\|LookupEnv("[A-Z_0-9]*")' <ref> -- '*.go' ':!*_test.go' | sort -u`). A removed or renamed var is breaking. Also check changed defaults, e.g. `internal/config`.
- **Action inputs:** `action.yml` inputs removed, renamed, or with a changed default.
- **Helm chart:** `charts/argo-diff/values.yaml` keys removed or renamed, changed defaults, changed templates.
- **Comment marker:** the `<!-- comment produced by argo-diff[...] -->` format (`internal/comment/wrap.go`) must still match comments from the last release. A change strands old comments.
- **Startup behavior:** credential checks and required-var failures in `cmd/main.go`. A new hard requirement for existing setups is breaking.
- **Go package API:** moves of `internal/` packages are not public API. Do not count them as breaking.

If pass 1 and pass 2 disagree, say so and recommend the higher bump only when pass 2 found a concrete break.

Versions follow [semver](https://semver.org) `MAJOR.MINOR.PATCH`. `X.Y.Z` in this skill is a placeholder for the new version, not a literal. A major bump resets minor and patch to 0. A minor bump resets patch to 0. Always bump from the last stable release (`$LAST`), even if a newer prerelease tag exists.

Then compute the proposed version and **ask the user to confirm it** before going further (`AskUserQuestion`: the recommended version first, the other bumps as options).

## 4. Branch and run the script

```bash
git checkout --no-track -b chore/prep-release-X.Y.Z origin/main
scripts/prep-release.sh X.Y.Z
```

The script bumps `action.yml`, `charts/argo-diff/Chart.yaml`, the chart `README.md`, and `docs/k8s/kustomization.yaml`, commits as `chore: prep for release X.Y.Z`, and prints the commit list. It does not add the `Assisted-by:` trailer, so amend the commit (not yet pushed) to add it, naming the model actually running:

```bash
git commit --amend -m "chore: prep for release X.Y.Z" -m "Assisted-by: Claude Code (<model-id>)"
```

If the script says "No changes to commit", stop and tell the user.

Check `git show --stat HEAD` touches only those four files. Flag anything else.

## 5. Draft release notes

Terse. No filler. Sections, omitting empty ones:

```markdown
## What's new
- **<feature>:** one line.

## Fixes
- one line each.

## Internal
- one line each, grouped. Dependency bumps and CI belong here.

## Upgrade notes
No breaking changes. <or list them, and say what to do>
```

Rules:

- Group commits by user-visible effect. Do not list every commit.
- State limits plainly (e.g. "event files only; CI and webhooks to follow").
- Name only env vars, inputs, and values you confirmed in the code, not ones you saw only in docs.

## 6. Push and open the PR

Push with an explicit refspec. Never push to `main`.

```bash
git push -u origin HEAD:refs/heads/chore/prep-release-X.Y.Z
```

Open the PR with `gh pr create` (title `chore: prep for release X.Y.Z`, base `main`). The body has, in order:

1. `Commits:` followed by the script's commit list (`- <hash> <subject>`), as in PR #376.
2. `## Draft release notes` followed by the notes from step 5.
3. The trailer lines the session's attribution reminder asks for, then `Assisted-by: Claude Code (<model-id>)`.

Do not hard-wrap paragraphs in the body. Never add `Signed-off-by:`.

## 7. Hand off - do not tag

Give the user the PR URL and say what to do next, in this order:

1. Review and merge the PR.
2. `git checkout main && git pull`.
3. Confirm `HEAD` is the merged prep commit (`git log -1`).
4. Tag and push:

```bash
git tag -a -s -m 'argo-diff vX.Y.Z' X.Y.Z && git push origin X.Y.Z
```

Then **end with a question**: ask the user to tell you when the PR is merged and they have pulled `main`. Only then, if they ask, show the tag command again. Never run it yourself.

The release workflow does the rest: image build, GitHub release, and the floating `vN` tag. The draft release notes go on the GitHub release page after it exists.
