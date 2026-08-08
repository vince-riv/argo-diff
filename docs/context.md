# docs/

Documentation assets. There is no generated site here — `README.md` at the repo root is the primary
documentation, and this directory holds what it references.

## img/

Screenshots embedded in `README.md`'s "Screenshots" section: a simple comment, a truncated long
line, an out-of-sync application, and a Helm application run via GitHub Actions. Replace them when
comment rendering changes visibly (`internal/github/markdown.go`).

## k8s/

A worked example of deploying argo-diff with raw manifests + Kustomize, for people who don't want
the Helm chart. Not applied by any test or workflow — purely illustrative, but kept current.

| File | Notes |
| ---- | ----- |
| `kustomization.yaml` | Targets namespace `argocd`; `images[].newTag` is bumped by `scripts/prep-release.sh` |
| `deployment.yaml` | Sets non-sensitive env inline (`image: argo-diff:CHANGEME`, replaced by the kustomize image transform) |
| `env-secret.yaml` | A **SealedSecret** — encrypted, safe to commit |
| `service.yaml` | ClusterIP on 8080 |
| `traefik_ingressroute.yaml` | Traefik `IngressRoute` exposing `/webhook` |
| `gen-env-secret.sh` | Prompts for the secrets and pipes `kubectl create secret --dry-run` into `kubeseal --merge-into env-secret.yaml` |

Known quirk: in `gen-env-secret.sh` the guard conditions for the PAT and the webhook secret are
swapped (the `$webhook_secret` branch adds `GITHUB_PERSONAL_ACCESS_TOKEN` and the `$api_token`
branch adds `GITHUB_WEBHOOK_SECRET`). Harmless when both are supplied; wrong when only one is.

When a configuration option changes, this directory is one of the surfaces that needs updating —
along with `README.md`, `action.yml`, and `charts/argo-diff/`.
