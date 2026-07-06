#!/usr/bin/env bash
# Bumps all version-pinned files ahead of cutting a release tag:
#   charts/argo-diff/Chart.yaml (version + appVersion)
#   charts/argo-diff/README.md  (regenerated via helm-docs)
#   docs/k8s/kustomization.yaml (image tag)
#   action.yml                  (image tag)
#
# Usage: scripts/prep-release.sh <version>   (e.g. 2.11.0)
#
# Review the diff, then commit/push directly to main or open a PR - whichever
# fits the change. Once merged, tag the resulting commit and push the tag:
#   git tag <version> && git push origin <version>
# The release workflow does the rest (image build, GitHub release, and moving
# the floating v<major>/actions-v<major> tags).

set -euo pipefail

if [ $# -ne 1 ]; then
  echo "Usage: $0 <version>  (e.g. 2.11.0)" >&2
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

if command -v helm-docs >/dev/null 2>&1; then
  helm-docs --chart-search-root=charts
else
  echo "warning: helm-docs not found on PATH; charts/argo-diff/README.md badges not regenerated" >&2
fi

echo
echo "Bumped to ${VERSION}:"
git diff --stat -- charts/argo-diff/Chart.yaml charts/argo-diff/README.md docs/k8s/kustomization.yaml action.yml
