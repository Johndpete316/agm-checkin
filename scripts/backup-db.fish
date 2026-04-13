#!/usr/bin/env fish
#
# Scheduled PostgreSQL backup for AGM Check-In.
# Streams pg_dump from the postgres pod, compresses with gzip,
# saves locally, then syncs to a remote via rclone.
#
# ── Scheduling ────────────────────────────────────────────────────────────────
#
# Option A — cron (run as the user that has kubectl access):
#   */15 * * * * /home/ubuntu/agm-checkin/scripts/backup-db.fish >> /var/log/agm-backup.log 2>&1
#
# Option B — systemd timer (recommended, handles missed runs on reboot):
#   See the unit file templates at the bottom of this script.
#   Install: sudo systemctl daemon-reload && sudo systemctl enable --now agm-backup.timer

# ── Configuration ─────────────────────────────────────────────────────────────

set POSTGRES_POD  "agm-checkin-postgres-0"
set BACKUP_DIR    "/var/backups/agm-checkin"
set KEEP_LOCAL    96                            # 96 × 15 min = 24 h of local history
set RCLONE_REMOTE "r2:agm-db-backup/postgres_backups"            # e.g. "b2:mybucket/agm-backups"

# ── Setup ─────────────────────────────────────────────────────────────────────

set TIMESTAMP (date +%Y%m%d_%H%M%S)
set FILENAME  "agm_backup_$TIMESTAMP.sql.gz"
set FILEPATH  "$BACKUP_DIR/$FILENAME"

mkdir -p $BACKUP_DIR

echo "[$TIMESTAMP] Starting backup → $FILEPATH"

# ── Dump ──────────────────────────────────────────────────────────────────────

kubectl exec $POSTGRES_POD -- sh -c \
    'PGPASSWORD=$POSTGRES_PASSWORD pg_dump -U $POSTGRES_USER --no-owner --no-privileges agm_db' \
    | gzip > $FILEPATH

if test $status -ne 0
    echo "[$TIMESTAMP] ERROR: pg_dump failed — aborting"
    rm -f $FILEPATH
    exit 1
end

set FILESIZE (wc -c < $FILEPATH | string trim)
echo "[$TIMESTAMP] Dump complete — $FILESIZE bytes compressed"

# ── Sync to remote ────────────────────────────────────────────────────────────

echo "[$TIMESTAMP] Uploading to $RCLONE_REMOTE ..."
rclone copy $FILEPATH $RCLONE_REMOTE --log-level INFO --s3-no-check-bucket

if test $status -ne 0
    # Not fatal — local copy still exists. Alert if you have monitoring.
    echo "[$TIMESTAMP] WARNING: rclone upload failed — backup saved locally only at $FILEPATH"
else
    echo "[$TIMESTAMP] Remote upload OK"
end

# ── Rotate local backups ──────────────────────────────────────────────────────

set old_files (ls -t $BACKUP_DIR/agm_backup_*.sql.gz 2>/dev/null | tail -n +$KEEP_LOCAL)
if test (count $old_files) -gt 0
    echo "[$TIMESTAMP] Pruning "(count $old_files)" old local backup(s)..."
    for f in $old_files
        rm $f
    end
end

echo "[$TIMESTAMP] Done."

# ── systemd unit file templates ───────────────────────────────────────────────
#
# /etc/systemd/system/agm-backup.service
# ----------------------------------------
# [Unit]
# Description=AGM Check-In database backup
#
# [Service]
# Type=oneshot
# User=ubuntu
# ExecStart=/home/ubuntu/agm-checkin/scripts/backup-db.fish
# StandardOutput=append:/var/log/agm-backup.log
# StandardError=append:/var/log/agm-backup.log
#
#
# /etc/systemd/system/agm-backup.timer
# ----------------------------------------
# [Unit]
# Description=AGM Check-In database backup every 15 minutes
# After=network-online.target
#
# [Timer]
# OnBootSec=2min
# OnUnitActiveSec=15min
# Persistent=true
#
# [Install]
# WantedBy=timers.target
#
#
# Install commands:
#   sudo systemctl daemon-reload
#   sudo systemctl enable --now agm-backup.timer
#   systemctl list-timers agm-backup.timer   # verify
