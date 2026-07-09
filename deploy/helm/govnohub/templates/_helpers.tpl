{{- define "govnohub.fullname" -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "govnohub.labels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "govnohub.databaseURL" -}}
postgres://{{ .Values.postgresql.username }}:{{ .Values.postgresql.password }}@{{ include "govnohub.fullname" . }}-postgresql:5432/{{ .Values.postgresql.database }}?sslmode=disable
{{- end }}
