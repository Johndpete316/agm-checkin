#!/usr/bin/env fish
#
# Disaster Recovery deployment for AGM Check-In on a local microk8s cluster.
# Designed to run on a laptop acting as a hotspot for registration staff.
#
# What this script does:
#   1. Builds fresh Docker images for the API and frontend
#   2. Loads them into microk8s (no registry needed)
#   3. Deploys PostgreSQL, the API, and the frontend into a dedicated namespace
#   4. Restores the latest database backup from rclone (or a local file you specify)
#   5. Prints the URL staff should open on their devices
#
# ── Prerequisites ──────────────────────────────────────────────────────────────
#   - microk8s installed and running  (snap install microk8s --classic)
#   - docker installed
#   - rclone configured with the same remote used by backup-db.fish
#     (only needed if pulling the backup from the remote; skip with --backup-file)
#
# ── Usage ─────────────────────────────────────────────────────────────────────
#   ./dr-deploy.fish                         # pull latest backup from rclone
#   ./dr-deploy.fish --backup-file dump.sql  # restore from a local file instead
#   ./dr-deploy.fish --teardown              # remove the agm-dr namespace entirely

# ── Configuration — edit these before an event ────────────────────────────────

set DR_NAMESPACE   "agm-dr"
set RCLONE_REMOTE  "REMOTE_NAME:PATH"    # same value as in backup-db.fish
set AUTH_PIN       "FILL_IN"             # shared staff access code
set DB_PASSWORD    "FILL_IN"             # postgres password for this DR instance
set DB_NAME        "agm_db"
set DB_USER        "agm"

set API_IMAGE  "agm-api:dr"
set FE_IMAGE   "agm-frontend:dr"

# NodePorts staff will connect to
set FRONTEND_NODEPORT 30000
set API_NODEPORT      30080

# ── Helpers ───────────────────────────────────────────────────────────────────

function log
    echo "==> $argv"
end

function die
    echo "ERROR: $argv"
    exit 1
end

function mk
    microk8s kubectl $argv
end

# ── Flags ─────────────────────────────────────────────────────────────────────

set BACKUP_FILE ""
set TEARDOWN false

for i in (seq (count $argv))
    if test "$argv[$i]" = "--backup-file"
        set BACKUP_FILE $argv[(math $i + 1)]
    else if test "$argv[$i]" = "--teardown"
        set TEARDOWN true
    end
end

# ── Teardown ──────────────────────────────────────────────────────────────────

if test "$TEARDOWN" = "true"
    echo ""
    echo "  This will delete the $DR_NAMESPACE namespace and all its resources."
    echo ""
    read --prompt "  Type 'yes' to continue: " --local confirm
    if test "$confirm" != "yes"
        echo "Aborted."
        exit 0
    end
    mk delete namespace $DR_NAMESPACE
    echo "Namespace $DR_NAMESPACE deleted."
    exit 0
end

# ── Detect laptop IP ──────────────────────────────────────────────────────────
#
# Pick the first non-loopback IPv4 address. If your hotspot uses a different
# interface, set LAPTOP_IP explicitly here instead.

set LAPTOP_IP (hostname -I | string split ' ' | grep -v '^127\.' | grep -v '^::' | head -1)

if test -z "$LAPTOP_IP"
    die "Could not detect laptop IP. Set LAPTOP_IP manually in this script."
end

set FRONTEND_URL "http://$LAPTOP_IP:$FRONTEND_NODEPORT"
set API_URL      "http://$LAPTOP_IP:$API_NODEPORT"

echo ""
echo "  Laptop IP     : $LAPTOP_IP"
echo "  Frontend URL  : $FRONTEND_URL  ← staff open this"
echo "  API URL       : $API_URL"
echo "  Namespace     : $DR_NAMESPACE"
echo ""
read --prompt "  Continue? [y/N] " --local confirm
if test "$confirm" != "y"
    echo "Aborted."
    exit 0
end

# ── microk8s preflight ────────────────────────────────────────────────────────

log "Checking microk8s status..."
microk8s status --wait-ready --timeout 30
or die "microk8s is not ready. Run: microk8s start"

log "Enabling required addons..."
microk8s enable dns storage
# ingress is not needed — NodePort is used for simplicity

# ── Build images ──────────────────────────────────────────────────────────────

set SCRIPT_DIR (dirname (realpath (status filename)))
set REPO_ROOT  (realpath $SCRIPT_DIR/..)

log "Building API image ($API_IMAGE)..."
docker build -t $API_IMAGE $REPO_ROOT/agm-checkin-api
or die "API image build failed"

log "Building frontend image ($FE_IMAGE) with API URL: $API_URL ..."
docker build \
    --build-arg VITE_API_URL=$API_URL \
    -t $FE_IMAGE \
    $REPO_ROOT/agm-checkin-frontend
or die "Frontend image build failed"

# ── Load images into microk8s ─────────────────────────────────────────────────

log "Loading images into microk8s (this may take a minute)..."
docker save $API_IMAGE | microk8s ctr images import -
or die "Failed to load API image into microk8s"

docker save $FE_IMAGE | microk8s ctr images import -
or die "Failed to load frontend image into microk8s"

# ── Namespace and secrets ─────────────────────────────────────────────────────

log "Creating namespace $DR_NAMESPACE..."
mk create namespace $DR_NAMESPACE 2>/dev/null; or true  # ok if already exists

log "Creating secrets..."
mk create secret generic agm-postgres-secret \
    --namespace $DR_NAMESPACE \
    --from-literal=POSTGRES_USER=$DB_USER \
    --from-literal=POSTGRES_PASSWORD=$DB_PASSWORD \
    --from-literal=POSTGRES_DB=$DB_NAME \
    --dry-run=client -o yaml | mk apply -f -

mk create secret generic agm-api-secret \
    --namespace $DR_NAMESPACE \
    --from-literal=DATABASE_URL="postgresql://$DB_USER:$DB_PASSWORD@agm-postgres:5432/$DB_NAME" \
    --from-literal=AUTH_PIN=$AUTH_PIN \
    --from-literal=ALLOWED_ORIGIN=$FRONTEND_URL \
    --dry-run=client -o yaml | mk apply -f -

# ── PostgreSQL ────────────────────────────────────────────────────────────────

log "Deploying PostgreSQL..."
mk apply -f - <<EOF
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: agm-postgres-pvc
  namespace: $DR_NAMESPACE
spec:
  accessModes: [ReadWriteOnce]
  resources:
    requests:
      storage: 2Gi
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: agm-postgres
  namespace: $DR_NAMESPACE
spec:
  replicas: 1
  selector:
    matchLabels:
      app: agm-postgres
  template:
    metadata:
      labels:
        app: agm-postgres
    spec:
      containers:
      - name: postgres
        image: postgres:16
        envFrom:
        - secretRef:
            name: agm-postgres-secret
        ports:
        - containerPort: 5432
        volumeMounts:
        - name: data
          mountPath: /var/lib/postgresql/data
        readinessProbe:
          exec:
            command: [pg_isready, -U, $DB_USER]
          initialDelaySeconds: 5
          periodSeconds: 3
      volumes:
      - name: data
        persistentVolumeClaim:
          claimName: agm-postgres-pvc
---
apiVersion: v1
kind: Service
metadata:
  name: agm-postgres
  namespace: $DR_NAMESPACE
spec:
  selector:
    app: agm-postgres
  ports:
  - port: 5432
    targetPort: 5432
EOF

log "Waiting for PostgreSQL to be ready..."
mk rollout status deployment/agm-postgres -n $DR_NAMESPACE --timeout=120s
or die "PostgreSQL deployment timed out"

# Extra wait for the DB to finish initializing after pod reports ready
sleep 5

# ── Database restore ──────────────────────────────────────────────────────────

set POSTGRES_POD (mk get pod -l app=agm-postgres -n $DR_NAMESPACE -o jsonpath='{.items[0].metadata.name}')

if test -n "$BACKUP_FILE"
    # Use local file provided via --backup-file
    log "Restoring from local file: $BACKUP_FILE ..."
    if not test -f "$BACKUP_FILE"
        die "Backup file not found: $BACKUP_FILE"
    end
    set RESTORE_SOURCE $BACKUP_FILE
else
    # Pull latest backup from rclone remote
    log "Fetching latest backup from $RCLONE_REMOTE ..."
    set LATEST_REMOTE (rclone lsf $RCLONE_REMOTE --files-only | sort -r | head -1 | string trim)
    if test -z "$LATEST_REMOTE"
        die "No backups found at $RCLONE_REMOTE. Use --backup-file to specify a local file."
    end
    echo "Latest remote backup: $LATEST_REMOTE"
    set LOCAL_TEMP "/tmp/$LATEST_REMOTE"
    rclone copy "$RCLONE_REMOTE/$LATEST_REMOTE" /tmp/
    or die "rclone download failed"
    set RESTORE_SOURCE $LOCAL_TEMP
end

log "Restoring database into pod $POSTGRES_POD ..."

# Drop and recreate to ensure a clean restore
mk exec $POSTGRES_POD -n $DR_NAMESPACE -- sh -c \
    "PGPASSWORD=$DB_PASSWORD psql -U $DB_USER -d postgres -c \"DROP DATABASE IF EXISTS $DB_NAME;\"" \
    2>/dev/null; or true

mk exec $POSTGRES_POD -n $DR_NAMESPACE -- sh -c \
    "PGPASSWORD=$DB_PASSWORD psql -U $DB_USER -d postgres -c \"CREATE DATABASE $DB_NAME;\""
or die "Failed to create database"

# Stream the backup into the pod (handles both .sql and .sql.gz)
if string match -q "*.gz" $RESTORE_SOURCE
    cat $RESTORE_SOURCE | gzip -d | mk exec -i $POSTGRES_POD -n $DR_NAMESPACE -- sh -c \
        "PGPASSWORD=$DB_PASSWORD psql -U $DB_USER -d $DB_NAME"
else
    cat $RESTORE_SOURCE | mk exec -i $POSTGRES_POD -n $DR_NAMESPACE -- sh -c \
        "PGPASSWORD=$DB_PASSWORD psql -U $DB_USER -d $DB_NAME"
end
or die "Database restore failed"

log "Database restored."

# ── API ───────────────────────────────────────────────────────────────────────

log "Deploying API (NodePort $API_NODEPORT)..."
mk apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: agm-api
  namespace: $DR_NAMESPACE
spec:
  replicas: 1
  selector:
    matchLabels:
      app: agm-api
  template:
    metadata:
      labels:
        app: agm-api
    spec:
      containers:
      - name: api
        image: $API_IMAGE
        imagePullPolicy: Never
        envFrom:
        - secretRef:
            name: agm-api-secret
        ports:
        - containerPort: 8080
        readinessProbe:
          httpGet:
            path: /health
            port: 8080
          initialDelaySeconds: 3
          periodSeconds: 5
---
apiVersion: v1
kind: Service
metadata:
  name: agm-api
  namespace: $DR_NAMESPACE
spec:
  type: NodePort
  selector:
    app: agm-api
  ports:
  - port: 8080
    targetPort: 8080
    nodePort: $API_NODEPORT
EOF

# ── Frontend ──────────────────────────────────────────────────────────────────

log "Deploying frontend (NodePort $FRONTEND_NODEPORT)..."
mk apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: agm-frontend
  namespace: $DR_NAMESPACE
spec:
  replicas: 1
  selector:
    matchLabels:
      app: agm-frontend
  template:
    metadata:
      labels:
        app: agm-frontend
    spec:
      containers:
      - name: frontend
        image: $FE_IMAGE
        imagePullPolicy: Never
        ports:
        - containerPort: 80
        readinessProbe:
          httpGet:
            path: /
            port: 80
          initialDelaySeconds: 3
          periodSeconds: 5
---
apiVersion: v1
kind: Service
metadata:
  name: agm-frontend
  namespace: $DR_NAMESPACE
spec:
  type: NodePort
  selector:
    app: agm-frontend
  ports:
  - port: 80
    targetPort: 80
    nodePort: $FRONTEND_NODEPORT
EOF

# ── Wait for everything ───────────────────────────────────────────────────────

log "Waiting for API..."
mk rollout status deployment/agm-api -n $DR_NAMESPACE --timeout=120s
or die "API deployment timed out"

log "Waiting for frontend..."
mk rollout status deployment/agm-frontend -n $DR_NAMESPACE --timeout=120s
or die "Frontend deployment timed out"

# ── Done ──────────────────────────────────────────────────────────────────────

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  DR deployment complete"
echo ""
echo "  Staff URL  →  $FRONTEND_URL"
echo "  API URL    →  $API_URL"
echo ""
echo "  Connect devices to this laptop's hotspot, then open:"
echo "  $FRONTEND_URL"
echo ""
echo "  To tear down when done:"
echo "  ./dr-deploy.fish --teardown"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
