# Argo Diff

Helm chart for Argo-Diff

![Version: 1.0.2](https://img.shields.io/badge/Version-1.0.2-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 1.0.2](https://img.shields.io/badge/AppVersion-1.0.2-informational?style=flat-square)

## Extended Description

The Argo Diff chart provides a method of comparing the generated manifests in a PR to the manifests that are currently deployed in the cluster. Then argo diff comments on the PR with any changes to ensure that the PR is not introducing any unexpected changes.

## Installing the Chart

To install the chart with the release name `my-release`:

```console
$ helm install my-release oci://ghcr.io/vince-riv/chart/argo-diff
```

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| command[0] | string | `"/app/argo-diff"` |  |
| config.argocd.authToken | string | `""` |  |
| config.argocd.grpcWeb | string | `""` |  |
| config.argocd.grpcWebRoot | string | `""` |  |
| config.argocd.serverAddr | string | `""` | REQUIRED: hostname and/or port of the ArgoCD server (eg: argocd.domain.tld or argocd.domain.tld:8080) |
| config.argocd.serverInsecure | string | `""` |  |
| config.argocd.serverPlainText | string | `""` |  |
| config.argocd.uiBaseURL | string | `""` | The base URL of the ArgoCD UI. Used for link generation in comments |
| config.commentLineMaxChars | string | `""` | Any individual line in Pull Request comments by argo-diff longer than this are truncated. Defaults to 175 |
| config.commentPreamble | string | `""` | String/markdown prefixed to comments. Try to keep to 150 chars or less in length |
| config.configMapAnnotations | object | `{}` |  |
| config.configMapCreate | bool | `true` | Have Helm create ConfigMap from values. If disabled, you need to manage environment variables |
| config.configMapName | string | `""` | Name of ConfigMap of environment variables. Defaults to release name |
| config.contextStr | string | `""` | Unique identifier of argo-diff instance. Use when deploying multiple instances (eg: one per cluster). Recommended to be a brief cluster nickname |
| config.github.application.id | string | `""` | GitHub Application Id. Ignored if github.auth.token (or GITHUB_TOKEN / GITHUB_PERSONAL_ACCESS_TOKEN) is set |
| config.github.application.installationId | string | `""` | GitHub App Installation Id. Ignored if github.auth.token (or GITHUB_TOKEN / GITHUB_PERSONAL_ACCESS_TOKEN) is set |
| config.github.application.privateKey | string | `""` | Value of Github application private key (contents of downloaded .pem file); Ignored if github.auth.token (or GITHUB_TOKEN / GITHUB_PERSONAL_ACCESS_TOKEN) is set |
| config.github.auth.token | string | `""` | Value of Github Personal Access Token. Populates GITHUB_TOKEN |
| config.github.webhook.sharedSecret | string | `""` | Shared secret key for Github webhook events |
| deployment.affinity | object | `{}` |  |
| deployment.annotations | object | `{}` |  |
| deployment.livenessProbe | object | `{"httpGet":{"path":"/healthz","port":"http"},"initialDelaySeconds":2,"periodSeconds":10}` | Configuration for liveness check. (See https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/) |
| deployment.nodeSelector | object | `{}` |  |
| deployment.podAnnotations | object | `{}` |  |
| deployment.podSecurityContext | object | `{}` |  |
| deployment.readinessProbe | object | `{"httpGet":{"path":"/healthz","port":"http"},"initialDelaySeconds":2,"periodSeconds":10}` | Configuration for readiness check. (See https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/) |
| deployment.replicas | int | `1` |  |
| deployment.resources | object | `{}` |  |
| deployment.revisionHistoryLimit | int | `5` |  |
| deployment.securityContext | object | `{}` |  |
| deployment.startupProbe | object | `{"failureThreshold":10,"httpGet":{"path":"/healthz","port":"http"},"periodSeconds":2}` | Configuration for startup check. (See https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/) |
| deployment.tolerations | list | `[]` |  |
| deployment.volumeMounts | list | `[]` | Additional volumeMounts on the output Deployment definition. |
| deployment.volumes | list | `[]` | Additional volumes on the output Deployment definition. |
| fullnameOverride | string | `""` |  |
| image.pullPolicy | string | `"IfNotPresent"` |  |
| image.repository | string | `"ghcr.io/vince-riv/argo-diff"` |  |
| image.tag | string | `""` |  |
| imagePullSecrets | list | `[]` |  |
| ingress.annotations | object | `{}` |  |
| ingress.className | string | `""` |  |
| ingress.enabled | bool | `false` |  |
| ingress.hosts[0].host | string | `"chart-example.local"` |  |
| ingress.hosts[0].paths[0].path | string | `"/"` |  |
| ingress.hosts[0].paths[0].pathType | string | `"ImplementationSpecific"` |  |
| ingress.tls | list | `[]` |  |
| labels | object | `{}` | Common labels to apply to resources |
| logLevel | string | `"info"` |  |
| nameOverride | string | `""` |  |
| namespaceOverride | string | `""` |  |
| secret.annotations | object | `{}` |  |
| secret.create | bool | `true` | Have Helm create Secret from values. If disabled, you need to manage sensitive environment variables |
| secret.name | string | `""` | Override the name of the secret passed to deployment's envFrom. Defaults to release name. Should contain the following keys ARGOCD_AUTH_TOKEN, GITHUB_WEBHOOK_SECRET, and GITHUB_PERSONAL_ACCESS_TOKEN/GITHUB_APP_PRIVATE_KEY |
| service.annotations | object | `{}` |  |
| service.port | int | `8080` |  |
| service.type | string | `"ClusterIP"` |  |
| serviceAccount.annotations | object | `{}` |  |
| serviceAccount.automount | bool | `true` |  |
| serviceAccount.create | bool | `true` |  |
| serviceAccount.name | string | `""` |  |

## Aids to navigation

- `templates/` contains all of the templates that are rendered by this chart.
  Note that files starting with `_` are not used to generate output directly,
  but the constructs therein are accessible by other template files (handy for
  reusable template fragments).
- `values.yaml` contains the default configuration parameters for the chart.
  This is also where we document configuration options for end users. Be sure to
  read https://helm.sh/docs/chart_best_practices/values/ before editing this
  file.

## Bootstrap yourself on Helm

- https://helm.sh/docs/intro/quickstart/
- http://masterminds.github.io/sprig/
- https://helm.sh/docs/chart_best_practices
- https://helm.sh/docs/chart_template_guide/data_types/
- https://json-schema.org/understanding-json-schema/
- https://go101.org/article/101.html (not required, but a handy reference for Go
  idioms that bleed through)

## Run Unittests

### Pre-Requisites

- helm
- https://github.com/helm-unittest/helm-unittest/tree/main

### Running Tests

1. `helm unittest .`

