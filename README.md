# AGM Check-In

A competition check-in tool for registration staff. Built to be lightweight, internal-only, and straightforward to operate.

## Stack

**Backend** — Go, chi router, GORM, PostgreSQL  
**Frontend** — React, Vite, Material UI, Recharts  
**Infrastructure** — k3s (Kubernetes), Helm, Cloudflare Tunnel

## What it does

- Staff authenticate with a shared access code and register their name — all activity is tied to the logged-in staff member
- Search competitors by name and check them in
- Flag competitors that require age/identity validation; staff review and correct date of birth before check-in
- Full audit trail — every check-in records which staff member performed it
- View all competitors in a sortable table (desktop) or card list (mobile)
- Stats dashboard showing check-in progress and check-ins by day
- Responsive — works on phones at the check-in desk

## Competitor data

Each competitor record includes: name, date of birth, shirt size, email, teacher, studio, and last registered event. The validation flag (`requiresValidation`) is used to enforce identity checks for minors before check-in is allowed.

## Auth

Access is gated by a shared access code distributed to staff out-of-band. Three failed attempts from any IP permanently blocks that IP. Tokens do not expire automatically and are revoked manually via the database after the event.

## Deployment

The application runs on a self-hosted k3s cluster. There is no cloud provider dependency — the cluster is three worker nodes and a control plane, all managed with standard `kubectl` and Helm.

Public access is handled by Cloudflare Tunnel. The tunnel connects outbound from the cluster to Cloudflare's edge, meaning no ports are exposed on the host machines and no ingress controller is needed. Two cloudflared replicas run inside the cluster for redundancy.

Docker images are built locally and imported directly into each node's containerd runtime via `k3s ctr images import`. No image registry is required.

The full deploy workflow is documented in [DEPLOYMENT.md](./DEPLOYMENT.md).

## Local development

**Backend**
```bash
cd agm-checkin-api
go mod tidy
./dev.fish
```

**Frontend**
```bash
cd agm-checkin-frontend
npm install
cp .env.example .env.local
npm run dev
```

Requires a local PostgreSQL instance. A Docker Compose file is provided in `docker/`.
