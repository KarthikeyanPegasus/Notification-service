{{/*
Expand the name of the chart.
*/}}
{{- define "notification-system.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
Truncated to 63 chars because some Kubernetes name fields are limited.
*/}}
{{- define "notification-system.fullname" -}}
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
Create chart label.
*/}}
{{- define "notification-system.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels applied to every resource.
*/}}
{{- define "notification-system.labels" -}}
helm.sh/chart: {{ include "notification-system.chart" . }}
{{ include "notification-system.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels (stable — do not change without a rolling strategy).
*/}}
{{- define "notification-system.selectorLabels" -}}
app.kubernetes.io/name: {{ include "notification-system.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Component-scoped selector labels used in Deployments and Services.
Usage: include "notification-system.componentLabels" (list . "api")
*/}}
{{- define "notification-system.componentLabels" -}}
{{- $ctx := index . 0 -}}
{{- $component := index . 1 -}}
{{ include "notification-system.selectorLabels" $ctx }}
app.kubernetes.io/component: {{ $component }}
{{- end }}

{{/*
ServiceAccount name.
*/}}
{{- define "notification-system.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "notification-system.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Secret name — where all sensitive env vars live.
*/}}
{{- define "notification-system.secretName" -}}
{{- default (printf "%s-secrets" (include "notification-system.fullname" .)) .Values.secrets.name }}
{{- end }}

{{/*
ConfigMap name.
*/}}
{{- define "notification-system.configMapName" -}}
{{- printf "%s-config" (include "notification-system.fullname" .) }}
{{- end }}

{{/*
Resolved PostgreSQL DSN.
Priority: config.database.dsn → postgresql subchart → externalDatabase
*/}}
{{- define "notification-system.databaseDSN" -}}
{{- if .Values.config.database.dsn -}}
{{- .Values.config.database.dsn -}}
{{- else if .Values.postgresql.enabled -}}
{{- printf "postgres://%s:%s@%s-postgresql:5432/%s?sslmode=disable"
    .Values.postgresql.auth.username
    .Values.postgresql.auth.password
    (include "notification-system.fullname" .)
    .Values.postgresql.auth.database -}}
{{- else if .Values.externalDatabase.host -}}
{{- printf "postgres://%s:%s@%s:%d/%s?sslmode=disable"
    .Values.externalDatabase.user
    .Values.externalDatabase.password
    .Values.externalDatabase.host
    (int .Values.externalDatabase.port)
    .Values.externalDatabase.database -}}
{{- else -}}
{{- fail "No database configured. Set config.database.dsn, postgresql.enabled=true, or externalDatabase.host." -}}
{{- end -}}
{{- end }}

{{/*
Resolved Redis address.
Priority: config.redis.addr → redis subchart → externalRedis
*/}}
{{- define "notification-system.redisAddr" -}}
{{- if .Values.config.redis.addr -}}
{{- .Values.config.redis.addr -}}
{{- else if .Values.redis.enabled -}}
{{- printf "%s-redis-master:6379" (include "notification-system.fullname" .) -}}
{{- else if .Values.externalRedis.host -}}
{{- printf "%s:%d" .Values.externalRedis.host (int .Values.externalRedis.port) -}}
{{- else -}}
{{- fail "No Redis configured. Set config.redis.addr, redis.enabled=true, or externalRedis.host." -}}
{{- end -}}
{{- end }}

{{/*
Resolved Kafka broker list.
Priority: config.pubsub.kafkaBrokers → externalKafka.brokers
*/}}
{{- define "notification-system.kafkaBrokers" -}}
{{- if .Values.config.pubsub.kafkaBrokers -}}
{{- .Values.config.pubsub.kafkaBrokers -}}
{{- else if .Values.externalKafka.brokers -}}
{{- .Values.externalKafka.brokers -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end }}

{{/*
Resolved Temporal host:port.
Priority: config.cadence.hostPort → externalTemporal
*/}}
{{- define "notification-system.temporalHostPort" -}}
{{- if .Values.config.cadence.hostPort -}}
{{- .Values.config.cadence.hostPort -}}
{{- else if .Values.externalTemporal.host -}}
{{- printf "%s:%d" .Values.externalTemporal.host (int .Values.externalTemporal.port) -}}
{{- else -}}
{{- "" -}}
{{- end -}}
{{- end }}

{{/*
Image tag — falls back to appVersion.
Usage: include "notification-system.imageTag" (list . .Values.image.api)
*/}}
{{- define "notification-system.imageTag" -}}
{{- $ctx := index . 0 -}}
{{- $img := index . 1 -}}
{{- default $ctx.Chart.AppVersion $img.tag -}}
{{- end }}

{{/*
Full image reference with optional global registry prefix.
Usage: include "notification-system.image" (list . .Values.image.api)
*/}}
{{- define "notification-system.image" -}}
{{- $ctx := index . 0 -}}
{{- $img := index . 1 -}}
{{- $tag := include "notification-system.imageTag" (list $ctx $img) -}}
{{- $registry := default "" $ctx.Values.global.imageRegistry -}}
{{- if $registry -}}
{{- printf "%s/%s:%s" $registry $img.repository $tag -}}
{{- else -}}
{{- printf "%s:%s" $img.repository $tag -}}
{{- end -}}
{{- end }}

{{/*
Internal URL the Worker uses to reach the API health endpoint.
*/}}
{{- define "notification-system.workerInternalUrl" -}}
{{- printf "http://%s-api:%d" (include "notification-system.fullname" .) (int .Values.api.service.port) -}}
{{- end }}
