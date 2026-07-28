{{/*
Common labels
*/}}
{{- define "agm-checkin.labels" -}}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Spread replicas across nodes (soft anti-affinity)
Pass the component name as the first argument: include "agm-checkin.spreadAffinity" "api"
*/}}
{{- define "agm-checkin.spreadAffinity" -}}
affinity:
  podAntiAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
    - weight: 100
      podAffinityTerm:
        topologyKey: kubernetes.io/hostname
        labelSelector:
          matchLabels:
            app: agm-{{ . }}
{{- end }}

{{/*
The postgres DSN. Defined once here and rendered only into db-secret.yaml —
consumers pull it from that secret rather than re-templating the password.
*/}}
{{- define "agm-checkin.dsn" -}}
host={{ .Release.Name }}-postgres user={{ .Values.postgres.user }} password={{ .Values.postgres.password }} dbname={{ .Values.postgres.db }} port=5432 sslmode=disable
{{- end }}

{{/*
DATABASE_URL, sourced from the db secret. Include where a container's env list
needs it: {{ include "agm-checkin.databaseUrlEnv" . | nindent 12 }}
*/}}
{{- define "agm-checkin.databaseUrlEnv" -}}
- name: DATABASE_URL
  valueFrom:
    secretKeyRef:
      name: {{ .Release.Name }}-db-secret
      key: dsn
{{- end }}

{{/*
POSTGRES_PASSWORD, sourced from the db secret. For the postgres container itself
and for the psql/pg_dump jobs.
*/}}
{{- define "agm-checkin.postgresPasswordEnv" -}}
- name: POSTGRES_PASSWORD
  valueFrom:
    secretKeyRef:
      name: {{ .Release.Name }}-db-secret
      key: password
{{- end }}

{{/*
DB backup PVC name (existing claim wins if provided).
*/}}
{{- define "agm-checkin.dbBackupPvcName" -}}
{{- if .Values.dbBackup.existingClaim -}}
{{ .Values.dbBackup.existingClaim }}
{{- else -}}
{{ .Release.Name }}-db-backups
{{- end -}}
{{- end }}

{{/*
Prefix used for backup filenames.
*/}}
{{- define "agm-checkin.dbBackupPrefix" -}}
{{- default "predeploy" .Values.dbBackup.filePrefix -}}
{{- end }}
