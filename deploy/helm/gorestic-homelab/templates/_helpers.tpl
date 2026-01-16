{{/*
Expand the name of the chart.
*/}}
{{- define "gorestic-homelab.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "gorestic-homelab.fullname" -}}
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
{{- define "gorestic-homelab.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "gorestic-homelab.labels" -}}
helm.sh/chart: {{ include "gorestic-homelab.chart" . }}
{{ include "gorestic-homelab.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "gorestic-homelab.selectorLabels" -}}
app.kubernetes.io/name: {{ include "gorestic-homelab.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Secret name
*/}}
{{- define "gorestic-homelab.secretName" -}}
{{- if .Values.secrets.existingSecret }}
{{- .Values.secrets.existingSecret }}
{{- else }}
{{- include "gorestic-homelab.fullname" . }}-secrets
{{- end }}
{{- end }}

{{/*
SSH key secret name
*/}}
{{- define "gorestic-homelab.sshKeySecretName" -}}
{{- if .Values.sshKey.existingSecret }}
{{- .Values.sshKey.existingSecret }}
{{- else }}
{{- include "gorestic-homelab.fullname" . }}-ssh-key
{{- end }}
{{- end }}

{{/*
PVC name
*/}}
{{- define "gorestic-homelab.pvcName" -}}
{{- if .Values.persistence.existingClaim }}
{{- .Values.persistence.existingClaim }}
{{- else }}
{{- include "gorestic-homelab.fullname" . }}-data
{{- end }}
{{- end }}

{{/*
Image name
*/}}
{{- define "gorestic-homelab.image" -}}
{{- $tag := default .Chart.AppVersion .Values.image.tag }}
{{- printf "%s:%s" .Values.image.repository $tag }}
{{- end }}
