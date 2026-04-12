# DEPLOYMENT.md

How to build, ship, and update AGM Check-In on k3s with Cloudflare Tunnel.

---

## Architecture overview

```
Your browser
     │
     ▼
Cloudflare Edge  ←──────────────────────────────────────┐
     │                                                   │
     │  Cloudflare Tunnel (outbound only, no open ports) │
     │                                                   │
     ▼                                              cloudflared pods (x2)
 k3s cluster                                             │
 ┌───────────────────────────────────────┐               │
 │  frontend pods (x2) ←── nginx        │───────────────┘
 │  api pods (x2)      ←── Go binary    │
 │  postgres pod (x1)  ←── StatefulSet  │
 └───────────────────────────────────────┘
```

Everything runs inside k3s. Cloudflared pods connect **outbound** to Cloudflare's network —
no NodePort, no LoadBalancer, no firewall rules needed. Traffic flows in through Cloudflare's
edge and gets forwarded to the right service inside the cluster.

---

## How the frontend Docker build works

The React app uses `VITE_API_URL` to know where to send fetch requests. Vite bakes this
into the JavaScript at **build time** — it is not an environment variable at runtime.
This means the value must be injected when building the Docker image:

```
docker build --build-arg VITE_API_URL=https://api.checkin.reduxit.net -t agm-frontend:latest .
```

The Dockerfile passes it through:
```dockerfile
ARG VITE_API_URL
ENV VITE_API_URL=$VITE_API_URL   # makes it visible to Vite during npm run build
RUN npm run build                # Vite replaces import.meta.env.VITE_API_URL inline
```

The resulting `dist/` folder has the URL hardcoded in the JS bundle. The nginx container
just serves these static files — it has no knowledge of the API URL itself.

**If you change the API domain**, you must rebuild and re-push the frontend image.
The `push-images.fish` script handles this — just update `VITE_API_URL` at the top of the file.

---

## How images get to k3s without a registry

k3s uses `containerd` internally (not Docker). You cannot `docker push` directly to it.
Instead, `push-images.fish`:

1. Builds both images locally with Docker
2. For each node: `docker save image | gzip | ssh node "sudo k3s ctr images import -"`

This pipes the image over SSH directly into containerd on each node. No registry required.
The Helm chart uses `imagePullPolicy: IfNotPresent` so k8s uses what's already there
and never tries to pull from the internet.

**Why all 3 nodes?** k8s can schedule a pod on any node. If a node doesn't have the image,
the pod stays `Pending`. Importing to all nodes avoids this.

---

## Secrets

Real secrets live in `helm/agm-checkin/values.secret.yaml` (gitignored).
Never put tokens or passwords in `values.yaml` — that gets committed.

Current secrets file: `helm/agm-checkin/values.secret.yaml`
```yaml
postgres:
  password: <your db password>

cloudflared:
  tunnelToken: <your tunnel token>

api:
  authPin: <staff access code>

pgadmin:
  email: <pgadmin login email>
  password: <pgadmin password>
```

`authPin` is the access code staff enter at login. Distribute it out-of-band (verbally).
To get a new tunnel token: Cloudflare Zero Trust → Networks → Tunnels → your tunnel → Configure → token.

---

## Cloudflare Tunnel routing

Routing is configured in the Cloudflare dashboard, not in code:

1. Go to [one.dash.cloudflare.com](https://one.dash.cloudflare.com) → Networks → Tunnels
2. Click your tunnel → **Public Hostnames**
3. Add routes:

| Hostname | Service |
|---|---|
| `checkin.reduxit.net` | `http://agm-checkin-frontend:80` |
| `api.checkin.reduxit.net` | `http://agm-checkin-api:8080` |
| `pgadmin.checkin.reduxit.net` | `http://agm-checkin-pgadmin:80` |

The service names (`agm-checkin-frontend`, `agm-checkin-api`, `agm-checkin-pgadmin`) match the Helm release name
prefix. If you install with `helm install agm-checkin ...` the services will be named exactly that.

Two cloudflared replicas run in the cluster, each maintaining their own connection to
Cloudflare's edge. If one pod dies, traffic automatically flows through the other.

**pgAdmin access**: pgAdmin runs as a ClusterIP service exposed only through the Cloudflare Tunnel. It has a 1Gi PVC for session/preferences storage. Add an Access policy in Cloudflare Zero Trust to restrict who can reach the pgadmin hostname.

---

## Full deploy workflow

### First deploy

```fish
# 1. Build images, push to all nodes, run Helm upgrade, and rolling restart
cd scripts
./push-images.fish

# 2. Confirm everything is running
kubectl get pods
```

`push-images.fish` handles the full cycle: build → import to containerd on each node → `helm upgrade --install` → `kubectl rollout restart` → waits for rollout to complete.

Expected output — all pods `Running`:
```
agm-checkin-api-xxxxx          1/1     Running
agm-checkin-api-xxxxx          1/1     Running
agm-checkin-frontend-xxxxx     1/1     Running
agm-checkin-frontend-xxxxx     1/1     Running
agm-checkin-postgres-0         1/1     Running
agm-checkin-pgadmin-xxxxx      1/1     Running
agm-checkin-cloudflared-xxxxx  1/1     Running
agm-checkin-cloudflared-xxxxx  1/1     Running
```

### Bootstrap after first deploy

After the first deploy, two manual steps are required:

**1. Create the first event** (via pgAdmin or port-forward):
```sql
INSERT INTO events (id, name, start_date, end_date, is_current)
VALUES ('glr-2026', 'GLR 2026', '2026-03-14', '2026-03-16', true);
```

**2. Promote the first admin user** — have the target person log in through the UI first to create their token, then:
```sql
UPDATE staff_tokens SET role = 'admin' WHERE first_name = 'John' AND last_name = 'Peterson';
```
After this, the admin can manage all other users' roles through the Manage Users page.

### Seed the database (dev only)

```fish
cd agm-checkin-api
./seed.fish
```

### Restore production data from a dump

```fish
# Export from local postgres:
pg_dump -U postgres -d agm_db --no-owner --no-privileges -f dump.sql

# Restore to production (drops and recreates agm_db, scales API down/up):
cd scripts
./restore-db.fish ../dump.sql
```

This wipes all existing data including staff tokens. Staff will need to sign in again after a restore.

---

## Updating the application

For any code change — API or frontend — just run:

```fish
cd scripts
./push-images.fish
```

The script builds both images, imports them to all nodes, runs `helm upgrade`, performs a rolling restart of both deployments, and waits for rollout completion.

### Helm values change only (e.g. scaling replicas)

```fish
helm upgrade agm-checkin ./helm/agm-checkin \
  -f ./helm/agm-checkin/values.secret.yaml
```

---

## Useful kubectl commands

```fish
# Watch all pods
kubectl get pods -w

# Logs for the API
kubectl logs -l app=agm-api -f

# Logs for cloudflared (useful if tunnel isn't connecting)
kubectl logs -l app=agm-cloudflared -f

# Describe a pod (good for diagnosing startup failures)
kubectl describe pod <pod-name>

# Scale API to 3 replicas manually
kubectl scale deployment agm-checkin-api --replicas=3

# Check services (confirm names match Cloudflare routing config)
kubectl get services

# Get a shell inside the API pod
kubectl exec -it deployment/agm-checkin-api -- sh
```

---

## Troubleshooting

| Symptom | Likely cause | Fix |
|---|---|---|
| Pod stuck `Pending` | Image not on that node | Re-run push-images.fish |
| Pod `CrashLoopBackOff` | App crashing on start | `kubectl logs <pod>` |
| Cloudflare shows tunnel offline | cloudflared pods not up | `kubectl logs -l app=agm-cloudflared` |
| Site loads but API calls fail | Wrong VITE_API_URL baked in | Rebuild frontend with correct --build-arg |
| postgres pod `Pending` | No storage available | `kubectl describe pod agm-checkin-postgres-0` |
| pgAdmin pod `Pending` | PVC not bound | `kubectl describe pvc agm-checkin-pgadmin-data` |
| pgAdmin shows wrong email/password | Credentials set at first start; PVC cached old values | Delete PVC and pod to force re-init, or recreate with correct values before first start |
| Port Forwarding for triage | `kubectl port-forward svc/agm-checkin-api 8080:8080` | Test API directly without going through Cloudflare |
