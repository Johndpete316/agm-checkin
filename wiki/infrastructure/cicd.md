# CI/CD

There is no automated CI/CD pipeline (no GitHub Actions, no Jenkins, no ArgoCD). All deploys are triggered manually by running `scripts/push-images.fish` from a developer machine with SSH access to the cluster nodes and `kubectl`/`helm` configured.

---

## Deploy Script: push-images.fish

**Location:** `scripts/push-images.fish`

This is the single script that handles a complete production deployment from a code change to a running rollout.

### What it does (in order)

1. **Build the API image**
   ```
   docker build -t agm-api:latest ../agm-checkin-api
   ```
   Uses the two-stage Dockerfile: Go builder stage compiles the binary, alpine final stage produces a minimal image (~20MB).

2. **Build the frontend image**
   ```
   docker build --build-arg VITE_API_URL=https://apicheckin.reduxit.net -t agm-frontend:latest ../agm-checkin-frontend
   ```
   Uses the two-stage Dockerfile: Node 20 builder stage runs `npm run build`, nginx alpine final stage serves the `dist/` folder.

   **Critical:** `VITE_API_URL` is baked into the JavaScript bundle at build time by Vite. It is not a runtime variable. If the API domain changes, this build arg must be updated in `push-images.fish` and the frontend must be rebuilt and redeployed.

3. **Import images to all 4 cluster nodes**
   For each node in `[ubuntu@k8s-cp, ubuntu@k8s-worker-1, ubuntu@k8s-worker-2, ubuntu@k8s-worker-3]`:
   ```
   docker save agm-api:latest | gzip | ssh ubuntu@<node> "sudo k3s ctr images import -"
   docker save agm-frontend:latest | gzip | ssh ubuntu@<node> "sudo k3s ctr images import -"
   ```
   This pipes compressed image tarballs directly into containerd on each node via SSH.

4. **Helm upgrade**
   ```
   helm upgrade --install agm-checkin ../helm/agm-checkin \
     -f ../helm/agm-checkin/values.secret.yaml
   ```
   Applies any changes to Helm values (replicas, env vars, secrets).

5. **Rolling restart**
   ```
   kubectl rollout restart deployment/agm-checkin-api
   kubectl rollout restart deployment/agm-checkin-frontend
   ```
   Forces pods to restart and pick up the newly imported images.

6. **Wait for rollout**
   ```
   kubectl rollout status deployment/agm-checkin-api --timeout=120s
   kubectl rollout status deployment/agm-checkin-frontend --timeout=120s
   ```
   Blocks until both deployments are fully rolled out or errors after 120 seconds.

### Prerequisites for running push-images.fish

- Docker installed and running on the developer machine
- SSH access to all 4 nodes (key-based, no password prompt)
- `kubectl` configured pointing at the cluster
- `helm` installed
- `helm/agm-checkin/values.secret.yaml` present and populated

---

## Image Distribution: No Registry Approach

The cluster has no container registry (no Docker Hub, no GHCR, no private registry). The rationale:

- The cluster is air-gapped from the public internet for image pulls by design
- `imagePullPolicy: IfNotPresent` is set on all application images in the Helm chart
- Images are imported directly to each node's containerd store via `k3s ctr images import`
- All 4 nodes must have the image because Kubernetes can schedule a pod on any node

The cloudflared image (`cloudflare/cloudflared:latest`) uses `imagePullPolicy: Always` and is pulled from Docker Hub by each node — this is the only image pulled from the internet.

---

## No GitHub Actions

There are no `.github/workflows/` files in this repository. There is no automated testing, linting, or deployment triggered on push or pull request.

> ⚠️ Not determined from static analysis — verify manually whether any external CI is configured (e.g., Cloudflare Pages, external webhook).

---

## Rollback

There is no automated rollback. To roll back:

1. Check out the previous commit in the repository
2. Re-run `./scripts/push-images.fish`

Alternatively, for a data rollback, use `scripts/restore-db.fish` with a previous backup dump.

---

## Related Pages

- [Infrastructure as Code](iac.md)
- [Environments](environments.md)
- [Disaster Recovery](disaster-recovery.md)
