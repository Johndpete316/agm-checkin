# Infrastructure as Code

All production infrastructure is defined with Helm. There are no raw Kubernetes manifests checked in — the Helm chart in `helm/agm-checkin/` is the single source of truth.

---

## Helm Chart

**Location:** `helm/agm-checkin/`  
**Chart version:** 0.1.0  
**App version:** 0.1.0  
**Type:** application

### Directory structure

```
helm/agm-checkin/
├── Chart.yaml
├── values.yaml          # committed defaults (no secrets)
├── values.secret.yaml   # gitignored — real secrets live here
└── templates/
    ├── _helpers.tpl                  # label helpers, spread affinity
    ├── api-deployment.yaml           # API Deployment + Secret (authPin)
    ├── api-service.yaml              # ClusterIP service for API
    ├── cloudflared-deployment.yaml   # cloudflared Deployment + Secret (tunnelToken)
    ├── frontend-deployment.yaml      # Frontend Deployment
    ├── frontend-service.yaml         # ClusterIP service for frontend
    ├── pgadmin-deployment.yaml       # pgAdmin Deployment
    ├── pgadmin-service.yaml          # ClusterIP service for pgAdmin
    ├── postgres-service.yaml         # ClusterIP service for postgres
    └── postgres-statefulset.yaml     # postgres StatefulSet + PVC template
```

### values.yaml (committed defaults)

```yaml
imagePullPolicy: IfNotPresent

api:
  replicaCount: 2
  image: agm-api
  tag: latest
  allowedOrigin: "https://checkin.reduxit.net"
  authPin: ""          # override in values.secret.yaml

frontend:
  replicaCount: 2
  image: agm-frontend
  tag: latest

postgres:
  image: postgres:16-alpine
  storageClass: local-path
  storageSize: 5Gi
  user: postgres
  password: test       # override in values.secret.yaml
  db: agm_db

pgadmin:
  image: dpage/pgadmin4:latest
  storageSize: 1Gi
  email: admin@example.com   # override in values.secret.yaml
  password: ""               # override in values.secret.yaml

cloudflared:
  replicaCount: 2
  tunnelToken: ""      # override in values.secret.yaml
```

### values.secret.yaml (gitignored — must be created manually)

This file is never committed. Create it alongside `values.yaml`:

```yaml
postgres:
  password: <db password>

cloudflared:
  tunnelToken: <tunnel token from Cloudflare Zero Trust dashboard>

api:
  authPin: <shared staff access code — distribute verbally>

pgadmin:
  email: <admin login email>
  password: <pgAdmin password>
```

The tunnel token can be retrieved from: Cloudflare Zero Trust → Networks → Tunnels → your tunnel → Configure → Token.

---

## How to Deploy

### Full deploy (code change to any service)

```fish
cd scripts
./push-images.fish
```

This script:
1. Builds `agm-api:latest` from `agm-checkin-api/` using Docker
2. Builds `agm-frontend:latest` from `agm-checkin-frontend/` with `--build-arg VITE_API_URL=https://apicheckin.reduxit.net`
3. For each of the 4 nodes (`k8s-cp`, `k8s-worker-1`, `k8s-worker-2`, `k8s-worker-3`): streams the image via `docker save | gzip | ssh node "sudo k3s ctr images import -"`
4. Runs `helm upgrade --install agm-checkin ../helm/agm-checkin -f ../helm/agm-checkin/values.secret.yaml`
5. Runs `kubectl rollout restart` on both the API and frontend deployments
6. Waits for rollout completion (120-second timeout)

### Helm values-only change (e.g., scaling replicas)

```fish
helm upgrade agm-checkin ./helm/agm-checkin \
  -f ./helm/agm-checkin/values.secret.yaml
```

No image rebuild or import needed.

---

## Why Images Are Imported Directly (No Registry)

k3s uses `containerd` internally, not Docker. There is no container registry in the cluster. Instead, `push-images.fish` saves each Docker image and pipes it over SSH into `k3s ctr images import` on every node.

The Helm chart uses `imagePullPolicy: IfNotPresent`, which tells Kubernetes to use the locally imported image and never attempt a pull. Images must be imported on **all nodes** because Kubernetes can schedule a pod on any available node, and a pod stays `Pending` if the image is missing on its assigned node.

---

## Secrets Management

| Secret | Stored in | Description |
|---|---|---|
| `AUTH_PIN` | `values.secret.yaml` → k8s Secret `agm-checkin-api-secret` | Shared staff access code |
| `TUNNEL_TOKEN` | `values.secret.yaml` → k8s Secret `agm-checkin-cloudflared-secret` | Cloudflare Tunnel token |
| `postgres.password` | `values.secret.yaml` → passed as env var to postgres StatefulSet | Database password |
| `pgadmin.email` / `pgadmin.password` | `values.secret.yaml` → pgAdmin env vars | pgAdmin credentials |

`AUTH_PIN` is injected into the API pod as an environment variable via a Kubernetes Secret referenced with `secretKeyRef`.

---

## Local Development with Docker Compose

For local development without k3s, a Docker Compose file is available at `docker/agm-checkin-dev/docker-compose.yml`. It starts:

- `postgres` (postgres:latest, tmpfs-backed, port 5432)
- `pgadmin4` (dpage/pgadmin4:latest, port 80)

The API and frontend are run directly on the host, not in containers.

---

## Related Pages

- [Infrastructure Overview](README.md)
- [CI/CD](cicd.md)
- [Environments](environments.md)
