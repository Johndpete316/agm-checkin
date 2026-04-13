# Disaster Recovery — AGM Check-In

This document covers everything needed to run AGM Check-In on a local laptop if the production infrastructure becomes unavailable during an event. The DR setup supports 5 concurrent registration staff over a local hotspot with no internet dependency after the initial backup pull.

**Scripts referenced here:**
- `scripts/backup-db.fish` — runs on the production server every 15 minutes, uploads compressed dumps to rclone remote
- `scripts/dr-deploy.fish` — runs on the DR laptop, restores the latest backup and deploys the full stack to microk8s

---

## What the DR Setup Looks Like

```
[Staff devices on WiFi] ──► [Laptop hotspot]
                                    │
                         ┌──────────┴──────────┐
                         │    microk8s          │
                         │  ┌───────────────┐   │
                         │  │  PostgreSQL   │   │
                         │  │  API :30080   │   │
                         │  │  Frontend :30000│ │
                         │  └───────────────┘   │
                         └─────────────────────-┘
```

Staff open `http://<laptop-ip>:30000` in their browser. No Cloudflare tunnel, no internet — just the laptop's hotspot.

---

## Part 1 — Installing Prerequisites on the DR Laptop

Install these once, well before the event. The laptop can be either Ubuntu/Debian or Arch.

### Docker

**Ubuntu / Debian**
```bash
sudo apt update
sudo apt install -y ca-certificates curl gnupg
sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
sudo chmod a+r /etc/apt/keyrings/docker.gpg

echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] \
  https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" \
  | sudo tee /etc/apt/sources.list.d/docker.list

sudo apt update
sudo apt install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin
sudo usermod -aG docker $USER
# Log out and back in, or: newgrp docker
```

**Arch**
```bash
sudo pacman -S docker docker-buildx
sudo systemctl enable --now docker
sudo usermod -aG docker $USER
# Log out and back in, or: newgrp docker
```

---

### microk8s

microk8s is distributed as a snap package. On Ubuntu it is available natively; on Arch, snapd must be installed first.

**Ubuntu / Debian**
```bash
sudo snap install microk8s --classic
sudo usermod -aG microk8s $USER
mkdir -p ~/.kube
sudo chown -R $USER ~/.kube
# Log out and back in for group membership to take effect

microk8s status --wait-ready
```

**Arch**

snapd is in the AUR. Install it with your AUR helper (example uses `yay`):
```bash
yay -S snapd
sudo systemctl enable --now snapd.socket
sudo ln -s /var/lib/snapd/snap /snap   # classic snap symlink

# Restart or re-login so the snap path is available, then:
sudo snap install microk8s --classic
sudo usermod -aG microk8s $USER
mkdir -p ~/.kube
sudo chown -R $USER ~/.kube
# Log out and back in

microk8s status --wait-ready
```

> **Arch note:** If you do not use an AUR helper, install snapd manually:
> ```bash
> git clone https://aur.archlinux.org/snapd.git
> cd snapd && makepkg -si
> ```

---

### rclone

```bash
# Ubuntu / Debian
sudo apt install -y rclone

# Arch
sudo pacman -S rclone
```

Configure rclone with the same remote used by `backup-db.fish`:
```bash
rclone config
```

Follow the interactive prompts to add the remote. Name it to match `RCLONE_REMOTE` in `dr-deploy.fish`.

---

### fish shell (if not present)

The DR script is written in fish.

```bash
# Ubuntu / Debian
sudo apt install -y fish

# Arch
sudo pacman -S fish
```

---

### Node.js (for frontend build)

The DR script builds the frontend locally. Node 20+ is required.

**Ubuntu / Debian — via nvm (recommended to match production):**
```bash
curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.7/install.sh | bash
# Restart shell, then:
nvm install 24
nvm use 24
```

**Arch:**
```bash
sudo pacman -S nodejs npm
```

---

## Part 2 — First-Time Configuration

### 1. Fill in the DR script config

Open `scripts/dr-deploy.fish` and set the values at the top:

```fish
set RCLONE_REMOTE  "REMOTE_NAME:PATH"   # match what backup-db.fish uses
set AUTH_PIN       "FILL_IN"            # the shared staff access code
set DB_PASSWORD    "FILL_IN"            # any password — just for the local DR postgres
```

### 2. Verify microk8s can run

```bash
microk8s status --wait-ready
microk8s kubectl get nodes
```

You should see a single node in `Ready` state.

### 3. Test a dry run (without live data)

Run the script once at home to make sure everything builds correctly before the event:

```bash
cd scripts
./dr-deploy.fish --backup-file /path/to/any/recent/dump.sql.gz
```

Browse to the printed URL to verify the UI loads and you can log in.

Tear it down when done:
```bash
./dr-deploy.fish --teardown
```

---

## Part 3 — Backups (Production Side)

The production server runs `backup-db.fish` every 15 minutes via cron or systemd timer. Each backup is:

1. Streamed out of the postgres pod with `pg_dump`
2. Compressed with gzip
3. Uploaded to the rclone remote
4. Kept locally for 24 hours, then pruned

Fill in `RCLONE_REMOTE` in `backup-db.fish` and schedule it on the production server:

**cron:**
```
*/15 * * * * /home/ubuntu/agm-checkin/scripts/backup-db.fish >> /var/log/agm-backup.log 2>&1
```

**systemd timer (recommended — handles missed runs after a reboot):**

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
systemctl list-timers agm-backup.timer   # verify next run time
```

---

## Part 4 — Day-of DR Runbook

### Step 1 — Start the laptop hotspot

On Ubuntu: Settings → Wi-Fi → Turn On Hotspot  
On Arch / GNOME: Settings → Wi-Fi → Turn On Hotspot  
On Arch / KDE: via NetworkManager in system tray

Note the hotspot's IP (usually `10.42.0.1` or `192.168.x.1`). The DR script detects this automatically via `hostname -I`.

### Step 2 — Start microk8s

```bash
microk8s start
microk8s status --wait-ready
```

### Step 3 — Run the DR deployment

From the repo root:
```bash
cd scripts
./dr-deploy.fish
```

The script will:
1. Show you the detected laptop IP and the URLs it will use — confirm before continuing
2. Build API and frontend Docker images (5–10 minutes on first run, faster after layer cache)
3. Load them into microk8s
4. Deploy PostgreSQL with a persistent volume
5. Pull the latest backup from the rclone remote and restore it
6. Deploy the API and frontend
7. Print the staff URL

### Step 4 — Connect staff devices

Have staff connect their devices to the laptop's hotspot and navigate to:

```
http://<laptop-ip>:30000
```

The URL is printed at the end of `dr-deploy.fish`. Staff log in with the normal access code (`AUTH_PIN` in the script).

### Step 5 — Monitor

Check that all pods are running:
```bash
microk8s kubectl get pods -n agm-dr
```

All three pods (`agm-postgres-*`, `agm-api-*`, `agm-frontend-*`) should show `Running` with `1/1` ready.

View API logs if something looks wrong:
```bash
microk8s kubectl logs -n agm-dr deployment/agm-api --follow
```

---

## Part 5 — Restoring to Production After DR

Once production is back up:

1. Take a final dump from the DR postgres pod:
   ```bash
   POSTGRES_POD=$(microk8s kubectl get pod -l app=agm-postgres -n agm-dr -o jsonpath='{.items[0].metadata.name}')
   microk8s kubectl exec $POSTGRES_POD -n agm-dr -- \
     sh -c 'PGPASSWORD=$POSTGRES_PASSWORD pg_dump -U $POSTGRES_USER --no-owner --no-privileges agm_db' \
     | gzip > dr_final_$(date +%Y%m%d_%H%M%S).sql.gz
   ```

2. Restore to production using the existing `restore-db.fish`:
   ```bash
   cd scripts
   ./restore-db.fish dr_final_<timestamp>.sql.gz
   ```

3. Tear down the DR namespace:
   ```bash
   ./dr-deploy.fish --teardown
   ```

---

## Troubleshooting

**`microk8s status` shows `microk8s is not running`**
```bash
microk8s start
```
If it stays stuck, check: `sudo journalctl -u snap.microk8s.daemon-kubelite -n 50`

**Image load is slow**
First run builds from scratch — expect 5–10 minutes. Subsequent runs reuse Docker's layer cache and are much faster (1–2 minutes for a code change).

**`rclone: no backups found`**
The remote path in `dr-deploy.fish` must match what `backup-db.fish` uploads to. Verify with:
```bash
rclone lsf REMOTE_NAME:PATH
```

If internet is unavailable, pass a pre-downloaded backup:
```bash
./dr-deploy.fish --backup-file /path/to/agm_backup_<timestamp>.sql.gz
```

**Port 30000 or 30080 already in use**
Change `FRONTEND_NODEPORT` and `API_NODEPORT` in `dr-deploy.fish` to other values in the 30000–32767 range.

**Staff can't reach the laptop**
- Confirm devices are connected to the laptop's hotspot, not another network
- Check the firewall isn't blocking the NodePorts: `sudo ufw allow 30000/tcp && sudo ufw allow 30080/tcp`
- On Arch with nftables: `sudo nft add rule inet filter input tcp dport {30000, 30080} accept`
