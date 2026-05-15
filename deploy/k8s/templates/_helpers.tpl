{{/*
Expand the name of the chart.
*/}}
{{- define "cloud-agent-platform.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "cloud-agent-platform.fullname" -}}
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
Create chart name and version as used by the chart label.
*/}}
{{- define "cloud-agent-platform.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "cloud-agent-platform.labels" -}}
helm.sh/chart: {{ include "cloud-agent-platform.chart" . }}
{{ include "cloud-agent-platform.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "cloud-agent-platform.selectorLabels" -}}
app.kubernetes.io/name: {{ include "cloud-agent-platform.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "cloud-agent-platform.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "cloud-agent-platform.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Redis host
*/}}
{{- define "cloud-agent-platform.redis.host" -}}
{{- if .Values.redis.enabled -}}
{{- printf "%s-redis.%s.svc.cluster.local:%d" .Release.Name .Release.Namespace .Values.redis.service.port | trunc 63 }}
{{- else -}}
{{- printf "%s:%d" .Values.redis.external.host .Values.redis.external.port | trunc 63 }}
{{- end }}
{{- end }}

{{/*
MinIO endpoint
*/}}
{{- define "cloud-agent-platform.minio.endpoint" -}}
{{- if .Values.minio.enabled -}}
{{- printf "%s-minio.%s.svc.cluster.local:9000" .Release.Name .Release.Namespace | trunc 63 }}
{{- else -}}
{{- .Values.minio.external.endpoint | trunc 63 }}
{{- end }}
{{- end }}

{{/*
PostgreSQL DSN
*/}}
{{- define "cloud-agent-platform.postgresql.dsn" -}}
{{- if .Values.postgresql.enabled -}}
{{- printf "postgres://%s:%s@%s.%s.svc.cluster.local:%d/%s?sslmode=disable" .Values.postgresql.username .Values.postgresql.password (printf "%s-postgresql" .Release.Name) .Release.Namespace .Values.postgresql.service.port .Values.postgresql.database | trunc 255 }}
{{- else -}}
{{- .Values.secrets.databaseDsn | trunc 255 }}
{{- end }}
{{- end }}
