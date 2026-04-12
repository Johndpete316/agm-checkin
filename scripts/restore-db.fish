#!/usr/bin/env fish

# Restores the production database from a local pg_dump export.
# Scales the API down before restore and back up after so AutoMigrate
# runs cleanly against the freshly restored schema.
#
# Usage:
#   ./restore-db.fish <path-to-dump.sql>
#
# Recommended export command from local postgres:
#   pg_dump -U postgres -d agm_db --no-owner --no-privileges -f dump.sql
#
# WARNING: drops and recreates agm_db entirely.
# All existing data is wiped — competitors, staff tokens, IP blocklist.
# Staff members will need to sign in again after this runs.

set DUMP_FILE $argv[1]
set POSTGRES_POD "agm-checkin-postgres-0"
set API_DEPLOYMENT "agm-checkin-api"
set API_REPLICAS 2

# ── Preflight ────────────────────────────────────────────────────────────────

if test (count $argv) -lt 1
    echo "Usage: ./restore-db.fish <path-to-dump.sql>"
    exit 1
end

if not test -f $DUMP_FILE
    echo "Error: file not found: $DUMP_FILE"
    exit 1
end

echo ""
echo "  Dump file : $DUMP_FILE ("(wc -c < $DUMP_FILE | string trim)" bytes)"
echo "  Target pod: $POSTGRES_POD"
echo ""
echo "  This will DROP the production database and restore from the dump."
echo "  All data including staff tokens will be wiped."
echo ""
read --prompt "  Type 'yes' to continue: " --local confirm
if test "$confirm" != "yes"
    echo "Aborted."
    exit 0
end

# ── Scale down API ────────────────────────────────────────────────────────────

echo ""
echo "==> Scaling down API..."
kubectl scale deployment/$API_DEPLOYMENT --replicas=0
or begin; echo "Failed to scale down API"; exit 1; end

# Wait for all API pods to terminate so there are no open DB connections
kubectl wait --for=delete pod -l app=agm-api --timeout=60s 2>/dev/null
# wait returns non-zero if no pods exist — that's fine, continue either way

# ── Drop and recreate the database ───────────────────────────────────────────

echo ""
echo "==> Dropping existing database..."
kubectl exec $POSTGRES_POD -- sh -c \
    'PGPASSWORD=$POSTGRES_PASSWORD psql -U $POSTGRES_USER -d postgres -c \
    "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = \047agm_db\047 AND pid <> pg_backend_pid();"'

kubectl exec $POSTGRES_POD -- sh -c \
    'PGPASSWORD=$POSTGRES_PASSWORD psql -U $POSTGRES_USER -d postgres -c "DROP DATABASE IF EXISTS agm_db;"'
or begin
    echo "Failed to drop database — scaling API back up"
    kubectl scale deployment/$API_DEPLOYMENT --replicas=$API_REPLICAS
    exit 1
end

echo "==> Creating fresh database..."
kubectl exec $POSTGRES_POD -- sh -c \
    'PGPASSWORD=$POSTGRES_PASSWORD psql -U $POSTGRES_USER -d postgres -c "CREATE DATABASE agm_db;"'
or begin
    echo "Failed to create database — scaling API back up"
    kubectl scale deployment/$API_DEPLOYMENT --replicas=$API_REPLICAS
    exit 1
end

# ── Restore ───────────────────────────────────────────────────────────────────

echo "==> Restoring from dump..."
cat $DUMP_FILE | kubectl exec -i $POSTGRES_POD -- sh -c \
    'PGPASSWORD=$POSTGRES_PASSWORD psql -U $POSTGRES_USER -d agm_db'
or begin
    echo "Restore failed — scaling API back up before exit"
    kubectl scale deployment/$API_DEPLOYMENT --replicas=$API_REPLICAS
    exit 1
end

# ── Scale API back up ─────────────────────────────────────────────────────────

echo ""
echo "==> Scaling API back up ($API_REPLICAS replicas)..."
kubectl scale deployment/$API_DEPLOYMENT --replicas=$API_REPLICAS
or begin; echo "Failed to scale API back up — run manually: kubectl scale deployment/$API_DEPLOYMENT --replicas=$API_REPLICAS"; exit 1; end

kubectl rollout status deployment/$API_DEPLOYMENT --timeout=120s

echo ""
echo "Done."
echo "AutoMigrate ran on startup — any new columns have been added to the restored schema."
echo "All staff members will need to sign in again."
