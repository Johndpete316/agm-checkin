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
  authPin: <your 4-digit PIN>
```

To get a new tunnel token: Cloudflare Zero Trust → Networks → Tunnels → your tunnel → Configure → token.

---

## Cloudflare Tunnel routing

Routing is configured in the Cloudflare dashboard, not in code:

1. Go to [one.dash.cloudflare.com](https://one.dash.cloudflare.com) → Networks → Tunnels
2. Click your tunnel → **Public Hostnames**
3. Add two routes:

| Hostname | Service |
|---|---|
| `checkin.reduxit.net` | `http://agm-checkin-frontend:80` |
| `api.checkin.reduxit.net` | `http://agm-checkin-api:8080` |

The service names (`agm-checkin-frontend`, `agm-checkin-api`) match the Helm release name
prefix. If you install with `helm install agm-checkin ...` the services will be named exactly that.

Two cloudflared replicas run in the cluster, each maintaining their own connection to
Cloudflare's edge. If one pod dies, traffic automatically flows through the other.

---

## Full deploy workflow

### First deploy

```fish
# 1. Build images and push to all nodes
cd scripts
./push-images.fish

# 2. Install via Helm (from repo root)
helm upgrade --install agm-checkin ./helm/agm-checkin \
  -f ./helm/agm-checkin/values.secret.yaml

# 3. Confirm everything is running
kubectl get pods
```

Expected output — all pods `Running`:
```
agm-checkin-api-xxxxx          1/1     Running
agm-checkin-api-xxxxx          1/1     Running
agm-checkin-frontend-xxxxx     1/1     Running
agm-checkin-frontend-xxxxx     1/1     Running
agm-checkin-postgres-0         1/1     Running
agm-checkin-cloudflared-xxxxx  1/1     Running
agm-checkin-cloudflared-xxxxx  1/1     Running
```

### Seed the database

```fish
cd agm-checkin-api
./seed.fish
```

---

## Updating the application

### Code change to the API

```fish
# Rebuild and push API image only
docker build -t agm-api:latest ./agm-checkin-api
for node in ubuntu@k8s-worker-1 ubuntu@k8s-worker-2 ubuntu@k8s-worker-3
    docker save agm-api:latest | gzip | ssh $node "sudo k3s ctr images import -"
end

# Rolling restart (pulls the new image already on the nodes)
kubectl rollout restart deployment/agm-checkin-api
```

### Code change to the frontend

```fish
docker build \
  --build-arg VITE_API_URL=https://api.checkin.reduxit.net \
  -t agm-frontend:latest \
  ./agm-checkin-frontend

for node in ubuntu@k8s-worker-1 ubuntu@k8s-worker-2 ubuntu@k8s-worker-3
    docker save agm-frontend:latest | gzip | ssh $node "sudo k3s ctr images import -"
end

kubectl rollout restart deployment/agm-checkin-frontend
```

### Helm values change (e.g. scaling replicas)

```fish
# Edit values.yaml, then:
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
| Port Forwarding for triage | kubectl port-forward svc/agm-checkin-api 8080:808 | to test and validate out test cases |
