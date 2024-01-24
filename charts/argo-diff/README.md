# Argo Diff

A Helm chart for Kubernetes

![Version: 0.5.1](https://img.shields.io/badge/Version-0.5.1-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.9.1](https://img.shields.io/badge/AppVersion-0.9.1-informational?style=flat-square)

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
| affinity | object | `{}` |  |
| autoscaling.enabled | bool | `false` |  |
| autoscaling.maxReplicas | int | `100` |  |
| autoscaling.minReplicas | int | `1` |  |
| autoscaling.targetCPUUtilizationPercentage | int | `80` |  |
| config.argocdBaseURL | string | `""` | The base URL of the ArgoCD server. Through which the argo-diff app can communicate with argocd server. |
| config.argocdUIBaseURL | string | `""` | The base URL of the ArgoCD UI. Used for link generation in comments |
| config.secretName | string | `""` | The name of the secret that contains the argocd credentials. Should contain the following keys ARGOCD_AUTH_TOKEN, GITHUB_PERSONAL_ACCESS_TOKEN, GITHUB_WEBHOOK_SECRET |
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
| livenessProbe | object | `{"httpGet":{"path":"/healthz","port":"http"},"initialDelaySeconds":2,"periodSeconds":10}` | Configuration for liveness check. (See https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/) |
| nameOverride | string | `""` |  |
| nodeSelector | object | `{}` |  |
| podAnnotations | object | `{}` |  |
| podLabels | object | `{}` |  |
| podSecurityContext | object | `{}` |  |
| readinessProbe | object | `{"httpGet":{"path":"/healthz","port":"http"},"initialDelaySeconds":2,"periodSeconds":10}` | Configuration for readiness check. (See https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/) |
| replicaCount | int | `1` |  |
| resources | object | `{}` |  |
| securityContext | object | `{}` |  |
| service.port | int | `8080` |  |
| service.type | string | `"ClusterIP"` |  |
| serviceAccount.annotations | object | `{}` |  |
| serviceAccount.automount | bool | `true` |  |
| serviceAccount.create | bool | `true` |  |
| serviceAccount.name | string | `""` |  |
| startupProbe | object | `{"failureThreshold":10,"httpGet":{"path":"/healthz","port":"http"},"periodSeconds":2}` | Configuration for startup check. (See https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/) |
| tolerations | list | `[]` |  |
| volumeMounts | list | `[]` |  |
| volumes | list | `[]` |  |

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

