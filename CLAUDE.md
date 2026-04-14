# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Intentions

- Provide a backend database and data services for the AGM Music Competition. Currently this covers competitor registration, check-in, identity validation, event management, staff management, and audit logging. The role may expand to further data services. Keep this in mind with design decisions.
- Provide an API to allow frontend applications to interact with these data services.
- Currently the only frontend is the check-in UI. This will expand in the future.

## Project Overview

AGM Check-In is a two-part application for competition registration staff to check in competitors:

- `agm-checkin-api/` — Go REST API (chi router, GORM, PostgreSQL)
- `agm-checkin-frontend/` — Vite + React SPA (MUI, React Router, Recharts)

---

## Backend

### Running

```bash
cd agm-checkin-api
# .env must contain DATABASE_URL and AUTH_PIN
./dev.fish
```

### First-time setup

```bash
cd agm-checkin-api
go mod tidy
```

After first deploy, bootstrap the first admin user and create the initial event via the database directly (no API exists for these bootstrap operations):

```sql
-- Promote a staff token to admin after first login
UPDATE staff_tokens SET role = 'admin' WHERE first_name = 'John' AND last_name = 'Peterson';

-- Create the first event
INSERT INTO events (id, name, start_date, end_date, is_current)
VALUES ('glr-2026', 'GLR 2026', '2026-03-14', '2026-03-16', true);
```

### Environment variables

| Variable | Required | Description |
|----------|----------|-------------|
| `DATABASE_URL` | Yes | Standard PostgreSQL DSN |
| `AUTH_PIN` | Yes | Access code for staff login (stored plaintext in env, compared with constant-time compare) |
| `ALLOWED_ORIGIN` | Yes | CORS allowed origin; startup fatal if unset |
| `TRUSTED_PROXY` | No | IP header trust mode: `cloudflare` (default, for CF Tunnel) or `direct` (local dev, no proxy) |

### Structure

| Path | Purpose |
|------|---------|
| `bin/api/main.go` | Router, all HTTP handlers, server entrypoint |
| `bin/seed/seed.go` | Seeds 100 realistic competitors for local development |
| `bin/import/main.go` | CSV normalization script: reads 4 raw historical CSVs, outputs one normalized CSV to stdout |
| `internal/db/db.go` | GORM models (`Competitor`, `Event`, `CompetitorEvent`, `AuditLog`), `Connect()`, `AutoMigrate()` |
| `internal/db/auth.go` | Auth models: `IPBlocklist`, `PINAttempt`, `StaffToken` |
| `internal/service/competitors.go` | `CompetitorService` — all competitor business logic |
| `internal/service/events.go` | `EventService` — event CRUD and current-event management |
| `internal/service/staff.go` | `StaffService` — staff token listing, role updates, revocation |
| `internal/service/auth.go` | `AuthService` — PIN verification, token creation, IP blocking |
| `internal/service/audit.go` | `AuditService` — writes audit log entries; `AuditLogView` for API responses |
| `internal/middleware/auth.go` | `IPBlocklist`, `RequireToken`, and `RequireAdmin` middleware; `GetClientIP`, `StaffFromContext` |

### API Endpoints

All endpoints except `/health` and `POST /api/auth/token` require an `Authorization: Bearer <token>` header.
The `IPBlocklist` middleware applies globally — blocked IPs cannot reach any endpoint.
Endpoints marked **Admin** additionally require `RequireAdmin` middleware (role must be `"admin"`).

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/health` | None | Liveness/readiness probe |
| POST | `/api/auth/token` | None | Verify access code + register name → returns bearer token + role |
| GET | `/api/auth/me` | Required | Returns current staff token info (used for role sync on page focus) |
| GET | `/api/competitors` | Required | List competitors; `?search=` for name search; registration users only see competitors registered for the current event |
| GET | `/api/competitors/{id}` | Required | Get single competitor with current-event check-in record |
| POST | `/api/competitors` | Required | Create competitor |
| PATCH | `/api/competitors/{id}` | Admin | Update all competitor fields (including `note`) |
| PATCH | `/api/competitors/{id}/checkin` | Required | Mark checked in for current event; auto-updates `lastRegisteredEvent` |
| PATCH | `/api/competitors/{id}/dob` | Required | Update date of birth `{"dateOfBirth": "2005-03-15T00:00:00Z"}` |
| PATCH | `/api/competitors/{id}/validate` | Required | Mark competitor as validated (`validated = true`) |
| DELETE | `/api/competitors/{id}` | Required | Delete competitor |
| GET | `/api/competitors/{id}/events` | Required | Full event history for a competitor |
| POST | `/api/competitors/import` | Admin | Bulk import from normalized CSV upload (multipart `file` field); creates DB snapshot before writing |
| GET | `/api/events` | Required | List all events (sorted by start_date desc) |
| GET | `/api/events/current` | Required | Get the current event |
| POST | `/api/events` | Admin | Create a new event |
| PATCH | `/api/events/{id}/current` | Admin | Set the current event (clears all others) |
| GET | `/api/staff` | Admin | List all staff tokens |
| PATCH | `/api/staff/{id}/role` | Admin | Update a staff member's role (`{"role": "admin"}` or `{"role": "registration"}`) |
| DELETE | `/api/staff/{id}` | Admin | Revoke a staff token |
| GET | `/api/audit` | Admin | List audit log entries; optional `?action=`, `?actor=`, `?limit=` filters |

### Data models

```go
type Competitor struct {
    ID                  string    // UUID, auto-generated
    NameFirst           string
    NameLast            string
    DateOfBirth         time.Time
    RequiresValidation  bool      // set true for competitors requiring identity check (typically minors)
    Validated           bool      // set true once staff has verified identity
    ShirtSize           string
    Email               string
    Teacher             string
    Studio              string
    LastRegisteredEvent string    // event ID slug of the most recent event this competitor registered for
    Note                string    // free-form internal staff note; visible to all roles, editable by admins only via PATCH /api/competitors/{id}
}

// Check-in state lives in CompetitorEvent, not on Competitor directly.
// GetAll and GetByID return CompetitorWithCheckIn which embeds the current-event CE record.
type CompetitorWithCheckIn struct {
    Competitor
    CurrentCheckIn *CompetitorEvent // nil if not registered for current event
}

type Event struct {
    ID        string    // human-readable slug, e.g. "glr-2026"
    Name      string
    StartDate time.Time
    EndDate   time.Time
    IsCurrent bool
}

// CompetitorEvent records participation in a specific event.
// Unique index on (competitor_id, event_id) — one row per competitor per event.
type CompetitorEvent struct {
    ID              string
    CompetitorID    string
    EventID         string
    CheckedIn       bool
    CheckInDatetime *time.Time // null for historical imports not yet checked in
    CheckedInBy     string     // staff full name at time of check-in; empty for historical imports
}

type AuditLog struct {
    ID         string    // UUID
    ActorID    string    // StaffToken.ID
    ActorName  string    // "First Last"
    Action     string    // e.g. "competitor.checkin", "staff.role_updated"
    EntityType string    // e.g. "competitor", "staff", "event"
    EntityID   string
    EntityName string    // human-readable name of entity at time of action
    DetailRaw  string    // JSON stored as text (gorm column "detail"); use AuditLogView for API output
    IPAddress  string
    CreatedAt  time.Time
}

type StaffToken struct {
    ID        string    // UUID
    Token     string    // 32-byte random hex, used as Bearer token
    FirstName string
    LastName  string
    Role      string    // "registration" (default) or "admin"
    CreatedAt time.Time
}

type IPBlocklist struct {
    ID        uint
    IPAddress string
    BlockedAt time.Time
}

type PINAttempt struct {
    ID          uint
    IPAddress   string
    AttemptedAt time.Time
}
```

### Auth system

- Staff log in with a shared access code (`AUTH_PIN` env var) + their first and last name
- `POST /api/auth/token` body: `{"code": "...", "firstName": "...", "lastName": "..."}`
- Response includes `token` and `role` (`"registration"` or `"admin"`)
- 3 failed attempts from the same IP → IP is added to `ip_blocklists` and permanently blocked
- IP is determined from `CF-Connecting-IP` header (set by Cloudflare Tunnel), falling back to `X-Forwarded-For`, then `RemoteAddr`
- Tokens do not expire automatically — revoke via the Manage Users page (admin) or directly in the database post-event
- `StaffFromContext(ctx)` helper retrieves the authenticated staff member inside any handler
- `RequireAdmin` middleware: reads staff from context, returns 403 if role is not `"admin"`
- `GET /api/auth/me` returns the current token's info — used by the frontend to sync role on page refresh and tab focus

### Audit logging

Every mutating handler logs an `AuditLog` entry after a successful operation:
- Action strings follow the pattern `entity.action` (e.g. `competitor.checkin`, `staff.role_updated`, `event.set_current`)
- `Detail` is a free-form JSON object with action-specific fields (e.g. `{"from": "registration", "to": "admin"}`)
- Audit writes are fire-and-forget — a failed audit write is logged to stderr but never returns an error to the caller
- `AuditLogView` struct exposes `Detail` as `json.RawMessage` for proper JSON output in the API response
- Audit logging happens in HTTP handlers (not services) because only handlers have staff context and client IP

---

## Frontend

### Running

```bash
cd agm-checkin-frontend
npm install
cp .env.example .env.local   # set VITE_API_URL if backend isn't on :8080
npm run dev
```

Note: This project uses Node.js via nvm with fish shell: `fish -c "nvm use 24 && npm run dev"`

### Structure

| Path | Purpose |
|------|---------|
| `src/api/auth.js` | `requestToken(code, firstName, lastName)` — auth API call |
| `src/api/competitors.js` | All competitor fetch calls; injects Bearer token; redirects to `/login` on 401 |
| `src/api/events.js` | `listEvents`, `getCurrentEvent`, `createEvent`, `setCurrentEvent` |
| `src/api/staff.js` | `listStaff`, `updateStaffRole`, `revokeStaff` |
| `src/api/audit.js` | `listAuditLogs({ action, actor, limit })` |
| `src/context/AuthContext.jsx` | Auth state (token + staff info + role), persisted in localStorage under `agm_token` / `agm_staff`; exposes `isAdmin`, `syncRole` |
| `src/theme.js` | MUI theme (Montserrat font, primary `#1565C0`) |
| `src/App.jsx` | `ColorModeContext`, `AuthProvider`, `ProtectedRoute`, `AdminRoute`, `AppLayout` |
| `src/components/NavBar.jsx` | Responsive nav: hamburger + Drawer on mobile, full nav on desktop (`md` breakpoint); shows admin links when `isAdmin` |
| `src/components/CompetitorCard.jsx` | Card used on Check-In page; shows age, studio, teacher, shirt size, email, lastRegisteredEvent; handles validation dialog; check-in state from `currentCheckIn` |
| `src/components/EditCompetitorDialog.jsx` | Admin-only dialog to edit all competitor fields |
| `src/components/AddCompetitorDialog.jsx` | Admin-only dialog to add a new competitor; pre-populates `lastRegisteredEvent` from current event |
| `src/pages/LoginPage.jsx` | Two-step login: access code → name → token + role stored in localStorage |
| `src/pages/CheckInPage.jsx` | `/home` — debounced server-side search + check-in |
| `src/pages/CompetitorsPage.jsx` | `/competitors` — card list on mobile, sortable table on desktop; edit + add for admins |
| `src/pages/StatsPage.jsx` | `/stats` — summary stats, donut chart (status), bar chart (check-ins by day) |
| `src/pages/EventsPage.jsx` | `/events` (admin) — event list, set current event, create new event |
| `src/pages/ManageUsersPage.jsx` | `/manage-users` (admin) — table of staff tokens; inline role changes; revoke with confirmation |
| `src/pages/AuditPage.jsx` | `/audit` (admin) — audit log table with action filter and actor search |
| `src/pages/ImportPage.jsx` | `/import` (admin) — CSV file upload with preview, triggers bulk competitor import |

### Auth flow

1. Any unauthenticated route → redirect to `/login`
2. Step 1: "Access code" field (password type, no hints about format or length)
3. Step 2: First name + Last name (separate fields)
4. On success: token, staff name, and role written to localStorage; user lands on `/home`
5. Token is sent as `Authorization: Bearer <token>` on every API request
6. Any 401 response clears localStorage and hard-redirects to `/login`
7. On mount and on `visibilitychange` (tab focus), `syncRole()` hits `GET /api/auth/me` to pick up any role changes; on 401, forces logout

### Role-based access

- `isAdmin` is derived from `auth.staff.role === 'admin'` in `AuthContext`
- `AdminRoute` in `App.jsx` redirects non-admins to `/home`
- Admin-only routes: `/events`, `/manage-users`, `/audit`
- Admin-only UI elements: edit/add on Competitors page, role controls on Manage Users

### Validation flow

Competitors with `requiresValidation = true` and `validated = false` show a "Validate" chip.
Clicking "Check In" on these opens a dialog with an editable date-of-birth field (pre-populated from existing DOB).
Staff corrects the DOB if needed, then confirms. This fires:
1. `PATCH /dob` if the date changed
2. `PATCH /validate` unconditionally
3. `PATCH /checkin`

### Updating the data model

If `Competitor` fields change in the backend:
- Update the Go struct in `internal/db/db.go`
- `AutoMigrate` handles schema changes on next startup (adds columns, does not drop them)
- On the frontend, `src/api/competitors.js` is the single file to update for field references
- Component field references (`nameFirst`, `nameLast`, etc.) are intentional — update those where used

---

## Deployment

**Full deploy** (build images + push to nodes + helm upgrade + rolling restart):
```fish
cd scripts && ./push-images.fish
```

**Database restore** (wipes production DB and restores from a local dump):
```fish
cd scripts && ./restore-db.fish ../path/to/dump.sql
```

**Secrets file** (`helm/agm-checkin/values.secret.yaml`, gitignored):
```yaml
postgres:
  password: <db password>
cloudflared:
  tunnelToken: <tunnel token>
api:
  authPin: <access code>
pgadmin:
  email: <admin email>
  password: <pgadmin password>
```

See `DEPLOYMENT.md` for full infrastructure details.

## Test Generation Agent

When asked to generate tests:
1. Enumerate all public funcs in the target package
2. Generate _test.go files per the project test conventions
3. Run `go test -race ./...` and fix all failures
4. Run `go vet ./...` and fix all warnings
5. Do not stop until exit code 0 on both commands
