{{/*
Expand the name of the chart.
*/}}
{{- define "metal-onboarding-operator.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "metal-onboarding-operator.fullname" -}}
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
Common labels
*/}}
{{- define "metal-onboarding-operator.labels" -}}
helm.sh/chart: {{ include "metal-onboarding-operator.name" . }}-{{ .Chart.Version | replace "+" "_" }}
{{ include "metal-onboarding-operator.selectorLabels" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "metal-onboarding-operator.selectorLabels" -}}
app.kubernetes.io/name: {{ include "metal-onboarding-operator.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
ServiceAccount name
*/}}
{{- define "metal-onboarding-operator.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "metal-onboarding-operator.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}
