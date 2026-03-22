{{- define "claude-oauth-proxy.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "claude-oauth-proxy.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := include "claude-oauth-proxy.name" . -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "claude-oauth-proxy.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "claude-oauth-proxy.labels" -}}
helm.sh/chart: {{ include "claude-oauth-proxy.chart" . }}
app.kubernetes.io/name: {{ include "claude-oauth-proxy.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "claude-oauth-proxy.selectorLabels" -}}
app.kubernetes.io/name: {{ include "claude-oauth-proxy.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "claude-oauth-proxy.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "claude-oauth-proxy.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "claude-oauth-proxy.apiKeySecretName" -}}
{{- if .Values.config.apiKey.existingSecret.name -}}
{{- .Values.config.apiKey.existingSecret.name -}}
{{- else -}}
{{- include "claude-oauth-proxy.fullname" . -}}
{{- end -}}
{{- end -}}

{{- define "claude-oauth-proxy.apiKeySecretKey" -}}
{{- if .Values.config.apiKey.existingSecret.key -}}
{{- .Values.config.apiKey.existingSecret.key -}}
{{- else -}}
api-key
{{- end -}}
{{- end -}}

{{- define "claude-oauth-proxy.tokenFilePath" -}}
{{- printf "%s/%s" .Values.persistence.mountPath .Values.persistence.tokenFileName -}}
{{- end -}}

{{- define "claude-oauth-proxy.validate" -}}
{{- if gt (int .Values.replicaCount) 1 -}}
{{- fail "replicaCount greater than 1 is not supported because Claude OAuth token refresh is single-writer." -}}
{{- end -}}
{{- if and (not .Values.config.apiKey.existingSecret.name) (not .Values.config.apiKey.value) (not (hasKey .Values.config.extraEnv "CLAUDE_OAUTH_PROXY_API_KEY")) -}}
{{- fail "set config.apiKey.value, config.apiKey.existingSecret.name, or config.extraEnv.CLAUDE_OAUTH_PROXY_API_KEY before installing the chart." -}}
{{- end -}}
{{- end -}}
