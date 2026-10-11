#!/usr/bin/env bash
# Bumps all version-pinned files ahead of cutting a release tag, then commits
# the result:
#   charts/argo-diff/Chart.yaml (version + appVersion)
#   charts/argo-diff/README.md  (regenerated via helm-docs)
#   docs/k8s/kustomization.yaml (image tag)
#   action.yml                  (image tag)
#
# Usage: scripts/prep-release.sh <version>   (e.g. 2.11.0)
#
# After committing, prints the commits since the last stable release as a
# bulleted list, ready to paste into the PR description.
#
# Push the resulting commit directly to main or open a PR - whichever fits the
# change. Once merged, tag the commit and push the tag:
#   git tag <version> && git push origin <version>
# The release workflow does the rest (image build, GitHub release, and moving
# the floating v<major> tags).

set -euo pipefail

if [ $# -ne 1 ]; then
  echo "Usage: $0 <version>  (e.g. 2.11.0)" >&2
  exit 1
fi

if ! command -v helm-docs >/dev/null 2>&1; then
  echo "error: helm-docs not found on PATH; install it (https://github.com/norwoodj/helm-docs) to regenerate charts/argo-diff/README.md" >&2
  exit 1
fi

VERSION="$1"
REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

sed -i.bak -E "s/^version: .+/version: ${VERSION}/" charts/argo-diff/Chart.yaml
sed -i.bak -E "s/^appVersion: .+/appVersion: \"${VERSION}\"/" charts/argo-diff/Chart.yaml
rm -f charts/argo-diff/Chart.yaml.bak

sed -i.bak -E "s/^([[:space:]]*newTag: ).+/\1${VERSION}/" docs/k8s/kustomization.yaml
rm -f docs/k8s/kustomization.yaml.bak

sed -i.bak -E "s#(docker://ghcr\.io/vince-riv/argo-diff:).+'#\1${VERSION}'#" action.yml
rm -f action.yml.bak

helm-docs --chart-search-root=charts

git add charts/argo-diff/Chart.yaml charts/argo-diff/README.md docs/k8s/kustomization.yaml action.yml

if git diff --cached --quiet; then
  echo "No changes to commit - already at ${VERSION}." >&2
  exit 0
fi

# Capture before committing, so the prep commit is not in the list. Only stable
# X.Y.Z tags count as releases; prerelease tags (X.Y.Z-rc1) are skipped.
LAST_RELEASE="$(git describe --tags --abbrev=0 --match '[0-9]*.[0-9]*.[0-9]*' --exclude '*-*' HEAD 2>/dev/null || true)"
if [ -n "$LAST_RELEASE" ]; then
  COMMITS="$(git log --no-decorate --format='- %h %s' "${LAST_RELEASE}..HEAD")"
fi

git commit -m "chore: prep for release ${VERSION}"

echo
echo "Committed prep for ${VERSION}. Push directly to main or open a PR, then:"
echo "  git tag -a -s -m 'argo-diff v${VERSION}' ${VERSION} && git push origin ${VERSION}"

if [ -n "$LAST_RELEASE" ]; then
  echo
  echo "Commits since ${LAST_RELEASE} (paste into the PR description):"
  echo
  echo "Commits:"
  echo "$COMMITS"
else
  echo
  echo "No previous release tag found; skipping the commit list." >&2
fi
