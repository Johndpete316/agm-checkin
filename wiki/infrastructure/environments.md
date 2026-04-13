# Environments

---

## Production

| Property | Value |
|---|---|
| **Frontend URL** | `https://checkin.reduxit.net` |
| **API URL** | `https://api.checkin.reduxit.net` (also seen as `https://apicheckin.reduxit.net` in push-images.fish) |
| **pgAdmin URL** | `https://pgadmin.checkin.reduxit.net` |
| **Database** | PostgreSQL 16, database name `agm_db`, user `postgres` |
| **API port** | 8080 (internal), exposed via Cloudflare Tunnel |
| **Frontend port** | 80 (internal), exposed via Cloudflare Tunnel |
| **Cluster** | k3s, 4 nodes (1 control plane + 3 workers) |

Access to the production database is through pgAdmin at the pgadmin hostname (protected by a Cloudflare Access policy) or via `kubectl port-forward`:

```bash
kubectl port-forward svc/agm-checkin-api 8080:8080
kubectl port-forward svc/agm-checkin-postgres 5432:5432
```

### Bootstrap steps (first deploy only)

After the first `./scripts/push-images.fish`, two manual SQL operations are required:

```sql
-- Create the first event
INSERT INTO events (id, name, start_date, end_date, is_current)
VALUES ('glr-2026', 'GLR 2026', '2026-03-14', '2026-03-16', true);

-- Promote first admin (have them log in via UI first to create their token)
UPDATE staff_tokens SET role = 'admin' WHERE first_name = 'John' AND last_name = 'Peterson';
```

After the first admin is set, all subsequent role management is done through the Manage Users page in the UI.

---

## Local Development

### Backend

The backend requires a running PostgreSQL instance and two environment variables.

**Quick start:**

```fish
cd agm-checkin-api
# .env must contain: DATABASE_URL=...
./dev.fish
```

`dev.fish` reads `DATABASE_URL` from `.env`, sets `AUTH_PIN=1234`, and runs `go run ./bin/api`. The server listens on `:8080`.

**Using Docker Compose for the database:**

```bash
cd docker/agm-checkin-dev
docker compose up -d
```

This starts:
- PostgreSQL at `localhost:5432` (db: `agm_db`, user: `postgres`, password: `test`)
- pgAdmin4 at `localhost:80` (email: `johndpete5316@outlook.com`, password: `test`)

Then set `DATABASE_URL` in `agm-checkin-api/.env`:
```
DATABASE_URL=host=localhost user=postgres password=test dbname=agm_db port=5432 sslmode=disable
```

**Seeding test data (100 realistic competitors):**

```fish
cd agm-checkin-api
./seed.fish
```

This calls `go run ./bin/seed` which deletes all existing competitors and inserts 100 seeded records across multiple event slugs, with a mix of validated/unvalidated and adult/minor competitors.

### Frontend

```bash
cd agm-checkin-frontend
npm install
cp .env.example .env.local   # set VITE_API_URL=http://localhost:8080
fish -c "nvm use 24 && npm run dev"
```

The frontend dev server runs on port 5173 by default. `VITE_API_URL` in `.env.local` controls where API requests are sent. The project uses Node.js 24 via nvm with fish shell.

### Environment Variables Reference

| Variable | Required | Where Set | Description |
|---|---|---|---|
| `DATABASE_URL` | Yes (API) | `.env` file or shell | PostgreSQL DSN |
| `AUTH_PIN` | Yes (API) | `dev.fish` sets `1234` in dev | Shared staff access code |
| `ALLOWED_ORIGIN` | No (API) | env or Helm values | CORS allowed origin; defaults to `*` |
| `VITE_API_URL` | No (frontend) | `.env.local` or `--build-arg` | API base URL; defaults to `http://localhost:8080` |

---

## Disaster Recovery Environment

When production is unavailable, a laptop-hosted environment can be stood up using microk8s. See [Disaster Recovery](disaster-recovery.md) for the full runbook.

The DR environment uses NodePort services:
- Frontend: `http://<laptop-ip>:30000`
- API: `http://<laptop-ip>:30080`

---

## Related Pages

- [Infrastructure Overview](README.md)
- [Infrastructure as Code](iac.md)
- [Disaster Recovery](disaster-recovery.md)
