#!/usr/bin/env fish

# Restores production from a predeploy backup created by the Helm backup hook.
# Backups are stored on the in-cluster backup PVC and restored through a
# temporary helper pod that mounts that PVC.
#
# Usage:
#   ./restore-predeploy-backup.fish
#   ./restore-predeploy-backup.fish --file predeploy_r42_20260727T190000Z.sql.gz
#   ./restore-predeploy-backup.fish --release agm-checkin --namespace default

set RELEASE "agm-checkin"
set NAMESPACE "default"
set BACKUP_PVC ""
set BACKUP_FILE ""
set API_REPLICAS ""
set BACKUP_PREFIX "predeploy"

for i in (seq (count $argv))
    switch "$argv[$i]"
        case --release
            set RELEASE "$argv[(math $i + 1)]"
        case --namespace
            set NAMESPACE "$argv[(math $i + 1)]"
        case --pvc
            set BACKUP_PVC "$argv[(math $i + 1)]"
        case --file
            set BACKUP_FILE "$argv[(math $i + 1)]"
        case --api-replicas
            set API_REPLICAS "$argv[(math $i + 1)]"
        case --prefix
            set BACKUP_PREFIX "$argv[(math $i + 1)]"
    end
end

if test -z "$BACKUP_PVC"
    set BACKUP_PVC "$RELEASE-db-backups"
end

set API_DEPLOYMENT "$RELEASE-api"
set POSTGRES_STS "$RELEASE-postgres"
set POSTGRES_SERVICE "$RELEASE-postgres"
set HELPER_POD "$RELEASE-db-restore-helper"

function die
    echo "ERROR: $argv"
    kubectl delete pod/$HELPER_POD -n $NAMESPACE --ignore-not-found=true >/dev/null 2>&1
    exit 1
end

set POSTGRES_USER (kubectl get statefulset/$POSTGRES_STS -n $NAMESPACE -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="POSTGRES_USER")].value}')
set POSTGRES_PASSWORD (kubectl get statefulset/$POSTGRES_STS -n $NAMESPACE -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="POSTGRES_PASSWORD")].value}')
set POSTGRES_DB (kubectl get statefulset/$POSTGRES_STS -n $NAMESPACE -o jsonpath='{.spec.template.spec.containers[0].env[?(@.name=="POSTGRES_DB")].value}')

if test -z "$POSTGRES_USER" -o -z "$POSTGRES_PASSWORD" -o -z "$POSTGRES_DB"
    die "Could not read Postgres credentials from statefulset/$POSTGRES_STS"
end

if test -z "$API_REPLICAS"
    set API_REPLICAS (kubectl get deployment/$API_DEPLOYMENT -n $NAMESPACE -o jsonpath='{.spec.replicas}')
end

if test -z "$API_REPLICAS"
    set API_REPLICAS 2
end

kubectl delete pod/$HELPER_POD -n $NAMESPACE --ignore-not-found=true >/dev/null 2>&1

begin
    echo "apiVersion: v1"
    echo "kind: Pod"
    echo "metadata:"
    echo "  name: $HELPER_POD"
    echo "  namespace: $NAMESPACE"
    echo "spec:"
    echo "  restartPolicy: Never"
    echo "  containers:"
    echo "    - name: helper"
    echo "      image: postgres:16-alpine"
    echo "      command: [\"sh\", \"-c\", \"sleep 3600\"]"
    echo "      env:"
    echo "        - name: POSTGRES_USER"
    echo "          value: \"$POSTGRES_USER\""
    echo "        - name: POSTGRES_PASSWORD"
    echo "          value: \"$POSTGRES_PASSWORD\""
    echo "        - name: POSTGRES_DB"
    echo "          value: \"$POSTGRES_DB\""
    echo "      volumeMounts:"
    echo "        - name: db-backups"
    echo "          mountPath: /backups"
    echo "  volumes:"
    echo "    - name: db-backups"
    echo "      persistentVolumeClaim:"
    echo "        claimName: $BACKUP_PVC"
end | kubectl apply -f - >/dev/null
or die "Failed to create helper pod (check PVC name: $BACKUP_PVC)"

kubectl wait --for=condition=Ready pod/$HELPER_POD -n $NAMESPACE --timeout=60s >/dev/null
or die "Helper pod did not become ready"

if test -z "$BACKUP_FILE"
    set BACKUP_FILE (kubectl exec -n $NAMESPACE $HELPER_POD -- sh -c "ls -1t /backups/$BACKUP_PREFIX"'_r'"*.sql.gz 2>/dev/null | head -1 | xargs -r basename")
    if test -z "$BACKUP_FILE"
        die "No backup files found under /backups with prefix '$BACKUP_PREFIX'"
    end
end

set FILE_EXISTS (kubectl exec -n $NAMESPACE $HELPER_POD -- sh -c "test -f /backups/$BACKUP_FILE && echo yes || echo no")
if test "$FILE_EXISTS" != "yes"
    die "Backup file not found in PVC: $BACKUP_FILE"
end

echo ""
echo "  Release        : $RELEASE"
echo "  Namespace      : $NAMESPACE"
echo "  Backup PVC     : $BACKUP_PVC"
echo "  Backup file    : $BACKUP_FILE"
echo "  API deployment : $API_DEPLOYMENT"
echo "  API replicas   : $API_REPLICAS"
echo ""
echo "  This will DROP and recreate database '$POSTGRES_DB' from the selected backup."
echo ""
read --prompt "  Type 'yes' to continue: " --local confirm
if test "$confirm" != "yes"
    echo "Aborted."
    kubectl delete pod/$HELPER_POD -n $NAMESPACE --ignore-not-found=true >/dev/null 2>&1
    exit 0
end

echo ""
echo "==> Scaling down API..."
kubectl scale deployment/$API_DEPLOYMENT -n $NAMESPACE --replicas=0
or die "Failed to scale down API"

kubectl wait --for=delete pod -l app=agm-api -n $NAMESPACE --timeout=60s 2>/dev/null

echo "==> Dropping and recreating database..."
kubectl exec -n $NAMESPACE $HELPER_POD -- sh -ec '
  PGPASSWORD="$POSTGRES_PASSWORD" psql -h "'$POSTGRES_SERVICE'" -U "$POSTGRES_USER" -d postgres -c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '\''$POSTGRES_DB'\'' AND pid <> pg_backend_pid();"
  PGPASSWORD="$POSTGRES_PASSWORD" psql -h "'$POSTGRES_SERVICE'" -U "$POSTGRES_USER" -d postgres -c "DROP DATABASE IF EXISTS \"$POSTGRES_DB\";"
  PGPASSWORD="$POSTGRES_PASSWORD" psql -h "'$POSTGRES_SERVICE'" -U "$POSTGRES_USER" -d postgres -c "CREATE DATABASE \"$POSTGRES_DB\";"
'
or begin
    kubectl scale deployment/$API_DEPLOYMENT -n $NAMESPACE --replicas=$API_REPLICAS >/dev/null 2>&1
    die "Failed to reset database"
end

echo "==> Restoring from backup..."
kubectl exec -n $NAMESPACE $HELPER_POD -- sh -ec '
  file="/backups/'$BACKUP_FILE'"
  if [ "${file##*.}" = "gz" ]; then
    gzip -dc "$file" | PGPASSWORD="$POSTGRES_PASSWORD" psql -h "'$POSTGRES_SERVICE'" -U "$POSTGRES_USER" -d "$POSTGRES_DB"
  else
    cat "$file" | PGPASSWORD="$POSTGRES_PASSWORD" psql -h "'$POSTGRES_SERVICE'" -U "$POSTGRES_USER" -d "$POSTGRES_DB"
  fi
'
or begin
    kubectl scale deployment/$API_DEPLOYMENT -n $NAMESPACE --replicas=$API_REPLICAS >/dev/null 2>&1
    die "Restore failed"
end

echo "==> Scaling API back up ($API_REPLICAS replicas)..."
kubectl scale deployment/$API_DEPLOYMENT -n $NAMESPACE --replicas=$API_REPLICAS
or die "Failed to scale API back up"

kubectl rollout status deployment/$API_DEPLOYMENT -n $NAMESPACE --timeout=120s
or die "API rollout did not complete"

kubectl delete pod/$HELPER_POD -n $NAMESPACE --ignore-not-found=true >/dev/null 2>&1

echo ""
echo "Done. Database restored from $BACKUP_FILE."
