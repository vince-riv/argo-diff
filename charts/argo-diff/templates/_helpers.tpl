{{/*
Expand the name of the chart.
*/}}
{{- define "argo-diff.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "argo-diff.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Expand the namespace of the release.
*/}}
{{- define "argo-diff.namespace" -}}
{{- default .Release.Namespace .Values.namespaceOverride | trunc 63 | trimSuffix "-" -}}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "argo-diff.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "argo-diff.labels" -}}
helm.sh/chart: {{ include "argo-diff.chart" . }}
{{ include "argo-diff.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- if .Values.labels }}
{{- toYaml .Values.labels }}
{{- end }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "argo-diff.selectorLabels" -}}
app.kubernetes.io/name: {{ include "argo-diff.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "argo-diff.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "argo-diff.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
ConfigMap name
*/}}
{{- define "argo-diff.configMapName" -}}
{{- if .Values.config.configMapName }}
{{- print .Values.config.configMapName }}
{{- else }}
{{- printf "%s-env" (include "argo-diff.fullname" .) }}
{{- end }}
{{- end }}

{{/*
Secret name
*/}}
{{- define "argo-diff.secretName" -}}
{{- if .Values.secret.name }}
{{- print .Values.secret.name }}
{{- else }}
{{- printf "%s-env" (include "argo-diff.fullname" .) }}
{{- end }}
{{- end }}

{{/*
Container env for the argo-diff container. deployment.env entries (name -> entry-minus-name, always
a map) take priority over the chart's LOG_LEVEL / ARGOCD_CLI_CMD_NAME defaults (name -> plain
scalar) via a single merge, so a same-named override replaces the default instead of duplicating
it. Defaults stay scalars, not maps, so merge's map-recursion can never blend a default's value into
an override that switches to valueFrom. The merged result mixes scalars and maps, so converting
back to a list has to check which is which.
*/}}
{{- define "argo-diff.env" -}}
{{- $deploymentEnv := dict }}
{{- range .Values.deployment.env }}
  {{- $_ := set $deploymentEnv .name (omit . "name") }}
{{- end }}
{{- $envDefaults := dict "LOG_LEVEL" .Values.logLevel }}
{{- if .Values.argocdCli.image.tag }}
  {{- $_ := set $envDefaults "ARGOCD_CLI_CMD_NAME" "/argocd-cli/argocd" }}
{{- end }}
{{- $env := merge $deploymentEnv $envDefaults }}
{{- $envList := list }}
{{- range $name, $val := $env }}
  {{- if kindIs "map" $val }}
    {{- $envList = append $envList (merge (dict "name" $name) $val) }}
  {{- else }}
    {{- $envList = append $envList (dict "name" $name "value" ($val | toString)) }}
  {{- end }}
{{- end }}
{{- toYaml $envList }}
{{- end -}}
