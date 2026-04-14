# Disaster Recovery and Backup

AGM Check-In includes a full DR plan designed to run on a laptop acting as a hotspot for registration staff during an event, with no dependency on internet connectivity after initial backup retrieval.

---

## Backup Strategy (Production)

**Script:** `scripts/backup-db.fish`

The script is designed to run on the production server on a 15-minute cadence. It is not triggered automatically — it must be scheduled via cron or a systemd timer (see below).

### What the backup script does

1. Runs `pg_dump` inside the `agm-checkin-postgres-0` pod via `kubectl exec`
2. Compresses the output with `gzip`
3. Saves the file locally to `/var/backups/agm-checkin/agm_backup_<YYYYMMDD_HHMMSS>.sql.gz`
4. **Deduplication check:** hashes the decompressed SQL (excluding timestamp-only lines) and compares against the previous upload hash. If the content hasn't changed, skips the upload — avoids needless remote writes when no data has changed between runs.
5. Uploads the file to the configured rclone remote (`r2:agm-db-backup/postgres_backups` by default)
6. Rotates local backups: keeps the 96 most recent (24 hours at 15-minute intervals), prunes older ones

### Scheduling the backup

**Recommended: systemd timer**

Create `/etc/systemd/system/agm-backup.service`:
```ini
[Unit]
Description=AGM Check-In database backup

[Service]
Type=oneshot
User=ubuntu
ExecStart=/home/ubuntu/agm-checkin/scripts/backup-db.fish
StandardOutput=append:/var/log/agm-backup.log
StandardError=append:/var/log/agm-backup.log
```

Create `/etc/systemd/system/agm-backup.timer`:
```ini
[Unit]
Description=AGM Check-In database backup every 15 minutes
After=network-online.target

[Timer]
OnBootSec=2min
OnUnitActiveSec=15min
Persistent=true

[Install]
WantedBy=timers.target
```

Enable:
```bash
sudo systemctl daemon-reload
sudo systemctl enable --now agm-backup.timer
systemctl list-timers agm-backup.timer
```

**Alternative: cron**
```
*/15 * * * * /home/ubuntu/agm-checkin/scripts/backup-db.fish >> /var/log/agm-backup.log 2>&1
```

### rclone configuration

The remote is named in `backup-db.fish` as `RCLONE_REMOTE`. The current value is `r2:agm-db-backup/postgres_backups`, which points to a Cloudflare R2 bucket. Configure rclone before using the backup script:
```bash
rclone config
```

---

## Restoring Production from a Dump

**Script:** `scripts/restore-db.fish`

This script wipes the production database and restores from a local `.sql` file (plain text, not gzipped). It is a destructive operation that requires confirmation.

### What restore-db.fish does

1. Prompts for confirmation (must type `yes`)
2. Scales the `agm-checkin-api` deployment to 0 replicas (closes all DB connections)
3. Waits for all API pods to terminate
4. Terminates any remaining connections to `agm_db`
5. Drops `agm_db`
6. Creates a fresh `agm_db`
7. Pipes the dump file into `psql` inside the postgres pod via `kubectl exec -i`
8. Scales the API deployment back to 2 replicas
9. Waits for the rollout (GORM AutoMigrate runs on startup against the restored schema)

```fish
cd scripts
./restore-db.fish ../path/to/dump.sql
```

> **Warning:** All data is wiped including staff tokens. Staff must sign in again after a restore.

---

## Disaster Recovery Deployment

**Script:** `scripts/dr-deploy.fish`

This script stands up the full AGM Check-In stack on a laptop running microk8s, then restores the latest production backup. Designed to run at an event venue with no internet after the initial backup pull.

### DR Architecture

```
[Staff devices on WiFi] ──► [Laptop hotspot]
                                    │
                         ┌──────────┴──────────┐
                         │    microk8s          │
                         │  namespace: agm-dr   │
                         │  ┌───────────────┐   │
                         │  │  PostgreSQL   │   │
                         │  │  API :30080   │   │
                         │  │  Frontend     │   │
                         │  │  :30000       │   │
                         │  └───────────────┘   │
                         └─────────────────────-┘
```

Staff connect their devices to the laptop's WiFi hotspot and navigate to `http://<laptop-ip>:30000`.

### DR Prerequisites (install once, before the event)

- Docker
- microk8s (`snap install microk8s --classic`)
- rclone (configured with the same remote as `backup-db.fish`)
- fish shell
- Node.js 20+ (for frontend build)

### DR Script Configuration

Edit the top of `scripts/dr-deploy.fish` before an event:

```fish
set DR_NAMESPACE   "agm-dr"
set RCLONE_REMOTE  "REMOTE_NAME:PATH"   # same as backup-db.fish
set AUTH_PIN       "FILL_IN"            # shared staff access code
set DB_PASSWORD    "FILL_IN"            # any password for local DR postgres
```

### DR Runbook

**Step 1 — Start the laptop hotspot** (via OS network settings). Note the IP address.

**Step 2 — Start microk8s**
```bash
microk8s start
microk8s status --wait-ready
```

**Step 3 — Run dr-deploy.fish**
```bash
cd scripts
./dr-deploy.fish
```

The script:
1. Detects the laptop IP automatically via `hostname -I`
2. Confirms the detected IP and URLs before proceeding
3. Builds Docker images for API and frontend (frontend baked with `VITE_API_URL=http://<laptop-ip>:30080`)
4. Loads images into microk8s via `microk8s ctr images import`
5. Creates namespace `agm-dr` and secrets
6. Deploys PostgreSQL with a 2Gi PVC
7. Fetches the latest backup from the rclone remote (or uses `--backup-file` if specified)
8. Drops and recreates the database, streams the backup in
9. Deploys the API (NodePort 30080) and frontend (NodePort 30000)
10. Waits for all deployments to be ready
11. Prints the staff URL

**Alternative: use a pre-downloaded backup**
```bash
./dr-deploy.fish --backup-file /path/to/agm_backup_<timestamp>.sql.gz
```
The script handles both `.sql` and `.sql.gz` backup files.

**Step 4 — Connect staff devices** to the hotspot, navigate to `http://<laptop-ip>:30000`

**Step 5 — Teardown when done**
```bash
./dr-deploy.fish --teardown
```
Deletes the entire `agm-dr` namespace.

---

## Restoring to Production After DR

Once production infrastructure is restored:

1. Take a final dump from the DR postgres pod:
   ```bash
   POSTGRES_POD=$(microk8s kubectl get pod -l app=agm-postgres -n agm-dr -o jsonpath='{.items[0].metadata.name}')
   microk8s kubectl exec $POSTGRES_POD -n agm-dr -- \
     sh -c 'PGPASSWORD=$POSTGRES_PASSWORD pg_dump -U $POSTGRES_USER --no-owner --no-privileges agm_db' \
     | gzip > dr_final_$(date +%Y%m%d_%H%M%S).sql.gz
   ```

2. Decompress and restore to production:
   ```bash
   gunzip dr_final_<timestamp>.sql.gz
   cd scripts
   ./restore-db.fish dr_final_<timestamp>.sql
   ```

3. Tear down DR namespace:
   ```bash
   ./dr-deploy.fish --teardown
   ```

---

## Related Pages

- [Infrastructure Overview](README.md)
- [Environments](environments.md)
- [CI/CD](cicd.md)
