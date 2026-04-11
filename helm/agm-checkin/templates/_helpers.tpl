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
