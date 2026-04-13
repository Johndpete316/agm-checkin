# AGM Check-In

A competition check-in and registration management tool for AGM Music Competition staff. Built to be lightweight, internal-only, and straightforward to operate at the check-in desk.

## What it does

AGM Check-In gives registration staff everything they need to run a competition check-in desk:

- **Staff authentication** — staff authenticate with a shared access code and register their name; all activity is tied to the logged-in staff member with two roles: `registration` (standard) and `admin`
- **Competitor search and check-in** — debounced server-side name search; check in a competitor in one tap
- **Identity validation** — competitors flagged for validation (typically minors) must have their date of birth reviewed and confirmed before check-in is allowed; staff can correct the DOB on the spot
- **Competitor management** — add new competitors, edit existing records (admin), view full event history per competitor
- **Event management** — manage multiple events, set the active event; check-in state is per-event
- **Staff management** — admins can view all staff tokens, change roles, and revoke access
- **Stats dashboard** — live summary of check-in progress, donut chart by status, bar chart of check-ins by day with T-shirt inventory breakdown
- **Audit log** — every mutating action records who did it, when, and from which IP — filterable by action type and actor
- **Bulk import** — admins can upload a normalized CSV to seed competitor records from historical data; the API snapshots the database before writing
- **Responsive UI** — card layout on mobile (phones at the desk), sortable table on desktop

## Stack

### Backend

| Technology | Role |
|---|---|
| **Go** | Application language |
| **chi** | HTTP router |
| **GORM** | ORM / database access |
| **PostgreSQL** | Primary data store |

### Frontend

| Technology | Role |
|---|---|
| **React** | UI framework |
| **Vite** | Build tool and dev server |
| **Material UI (MUI)** | Component library |
| **React Router** | Client-side routing |
| **Recharts** | Charts on the stats dashboard |

### Infrastructure

| Technology | Role |
|---|---|
| **k3s** | Lightweight Kubernetes distribution; self-hosted on bare-metal nodes |
| **Helm** | Kubernetes package management; all app config lives in `helm/agm-checkin/` |
| **Cloudflare Tunnel** | Public access without exposed ports; two `cloudflared` replicas run inside the cluster |
| **PostgreSQL** | Deployed as a Helm release inside the cluster |
| **pgAdmin** | Database admin UI, also deployed in-cluster |

Docker images are built locally and imported directly into each node's containerd runtime via `k3s ctr images import` — no external image registry is required.

## Data model

```
Competitor ────────────┐
  id (UUID)            │
  nameFirst            │ 1
  nameLast             │
  dateOfBirth          │
  requiresValidation   │         CompetitorEvent
  validated            ├────────► id (UUID)
  shirtSize            │         competitorID  ──► Competitor.id
  email                │         eventID       ──► Event.id
  teacher              │         checkedIn
  studio               │         checkInDatetime
  lastRegisteredEvent  │         checkedInBy (staff name snapshot)
  note                 │
                       │
Event ─────────────────┘
  id (slug, e.g. "glr-2026")
  name
  startDate / endDate
  isCurrent

StaffToken
  id (UUID)
  token (32-byte hex Bearer token)
  firstName / lastName
  role ("registration" | "admin")
  createdAt

AuditLog
  id (UUID)
  actorID / actorName
  action (e.g. "competitor.checkin", "staff.role_updated")
  entityType / entityID / entityName
  detail (JSON)
  ipAddress
  createdAt
```

**Key relationships:**

- A `Competitor` can participate in many `Event`s; participation and check-in state live in `CompetitorEvent` (unique per competitor + event)
- `CompetitorEvent` captures a snapshot of *who* checked the competitor in (`checkedInBy`) at the time of check-in — the staff token is not linked by foreign key so revoked staff names are preserved in history
- The "current event" is a flag on `Event` (`isCurrent = true`); only one event is current at a time; registration-role staff only see competitors registered for the current event
- `AuditLog` is append-only and written fire-and-forget from HTTP handlers (not services) because only handlers have staff context and client IP

## Auth

- Shared access code (`AUTH_PIN` env var) distributed to staff out-of-band; staff also register their first and last name on login
- Three failed login attempts from any IP permanently blocks that IP (stored in `ip_blocklists`)
- IP is resolved from `CF-Connecting-IP` (Cloudflare Tunnel header), falling back to `X-Forwarded-For`, then `RemoteAddr`
- Tokens do not expire automatically; revoke them via the Manage Users page (admin) or directly in the database post-event
- On every page focus, the frontend re-validates the token against `GET /api/auth/me` to pick up role changes in real time

## API summary

All endpoints except `/health` and `POST /api/auth/token` require `Authorization: Bearer <token>`.

| Area | Endpoints |
|---|---|
| Auth | `POST /api/auth/token`, `GET /api/auth/me` |
| Competitors | `GET`, `POST /api/competitors`; `GET`, `PATCH`, `DELETE /api/competitors/{id}`; `/checkin`, `/dob`, `/validate`, `/events` sub-routes |
| Events | `GET /api/events`, `GET /api/events/current`, `POST /api/events`, `PATCH /api/events/{id}/current` |
| Staff | `GET /api/staff`, `PATCH /api/staff/{id}/role`, `DELETE /api/staff/{id}` |
| Audit | `GET /api/audit` |
| Import | `POST /api/competitors/import` |

Admin-only endpoints: `PATCH /api/competitors/{id}`, `/import`, all `/api/events` mutations, all `/api/staff`, `GET /api/audit`.

## Deployment

The application runs on a self-hosted k3s cluster with no cloud provider dependency. Two `cloudflared` replicas run inside the cluster and connect outbound to Cloudflare's edge, so no ports need to be exposed on the host machines and no ingress controller is required.

**Full deploy** (build + push images to nodes + helm upgrade):
```fish
cd scripts && ./push-images.fish
```

**Database restore** (wipe production DB and restore from local dump):
```fish
cd scripts && ./restore-db.fish ../path/to/dump.sql
```

See [DEPLOYMENT.md](./DEPLOYMENT.md) for full infrastructure details.

## Local development

**Backend**
```bash
cd agm-checkin-api
go mod tidy
# .env must contain DATABASE_URL and AUTH_PIN
./dev.fish
```

**Frontend**
```bash
cd agm-checkin-frontend
npm install
cp .env.example .env.local   # set VITE_API_URL if backend isn't on :8080
npm run dev
```

Requires a local PostgreSQL instance. A Docker Compose file is provided in `docker/`.
