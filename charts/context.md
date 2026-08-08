# charts/

Two Helm charts with very different purposes:

| Chart | Purpose |
| ----- | ------- |
| `argo-diff/` | The published chart for running argo-diff as a webhook receiver |
| `test-basic-deployment/` | A fixture chart deployed by the k3s e2e test — never released |

## charts/argo-diff

Templates render a Deployment (with `/healthz` liveness, readiness, and startup probes), Service,
optional Ingress, ServiceAccount, and — when enabled — the ConfigMap and Secret holding argo-diff's
environment variables. The Deployment consumes both via `envFrom` and annotates the pod with
`checksum/config` / `checksum/secret` so a config change triggers a rollout.

Configuration split:

- **ConfigMap** (`config.configMapCreate`): non-sensitive vars — `ARGOCD_SERVER_ADDR`,
  `ARGOCD_GRPC_WEB*`, `ARGOCD_SERVER_INSECURE`, `ARGOCD_SERVER_PLAINTEXT`, `ARGOCD_UI_BASE_URL`,
  `ARGO_DIFF_COMMENT_PREAMBLE`, `ARGO_DIFF_CONTEXT_STR`, `COMMENT_LINE_MAX_CHARS`, `GITHUB_APP_ID`,
  `GITHUB_APP_INSTALLATION_ID`.
- **Secret** (`secret.create`): `ARGOCD_AUTH_TOKEN`, `GITHUB_TOKEN`, `GITHUB_APP_PRIVATE_KEY`,
  `GITHUB_WEBHOOK_SECRET`.

Set `configMapCreate: false` / `secret.create: false` to manage those objects yourself and point at
them with `config.configMapName` / `secret.name`.

**Not every argo-diff env var has a values key** — `ARGO_DIFF_TIMEOUT` and
`ARGO_DIFF_DISABLE_NON_GITHUB_REPO_MATCH`, for example, currently require an externally managed
ConfigMap. Adding one means touching `values.yaml` (with its `# key -- description` comment for
helm-docs), `templates/configmap.yaml`, `tests/configmap_test.yaml`, and the README table.

### Versioning

`version` and `appVersion` in `Chart.yaml` are bumped together by `scripts/prep-release.sh`, which
also regenerates `README.md`. Publishing is driven by a `chart-X.Y.Z` tag
(`.github/workflows/chart-releaser.yml`), which verifies the tag matches `Chart.yaml` and pushes the
chart to `oci://ghcr.io/vince-riv/chart/argo-diff`.

### README

`README.md` is **generated** — never edit it by hand. It comes from `README.md.gotmpl` plus the
`# key -- description` comments in `values.yaml`, via `helm-docs --chart-search-root=charts`.

### Tests

`tests/*.yaml` are [helm-unittest](https://github.com/helm-unittest/helm-unittest) suites, run in CI
by `.github/workflows/helm.yml` (which also runs `helm lint`). Locally: `helm unittest .` from the
chart directory. `tests/__snapshot__/` exists for snapshot assertions but is currently empty.

`.debug/` is gitignored scratch output (`.gitignore` ignores `.debug`), typically from
`helm template --output-dir`.

## charts/test-basic-deployment

Pinned at version `0.0.0` and deliberately not published. It renders a namespace, deployment,
configmap, secret, serviceaccount, clusterrole, and clusterrolebinding, and is referenced by
`test/argocd-applications/helm-deployment-app.yaml` as a multi-source ArgoCD application (chart from
this repo, values from `test/helm-deployment.values.yaml`). Changing it changes what the k3s
end-to-end test diffs — see `test/context.md`.
