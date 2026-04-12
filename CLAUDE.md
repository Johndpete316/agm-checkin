# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Intentions

- Provide a backend database and data services for the AGM Music Competition. Currently this covers competitor registration, check-in, and identity validation. The role may expand to further data services. Keep this in mind with design decisions.
- Provide an API to allow frontend applications to interact with these data services.
- Currently the only frontend is the check-in UI. This will expand in the future.
- An admin UI for post-event token expiry and management is a planned but not yet built feature.

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

### Environment variables

| Variable | Required | Description |
|----------|----------|-------------|
| `DATABASE_URL` | Yes | Standard PostgreSQL DSN |
| `AUTH_PIN` | Yes | Access code for staff login (stored plaintext in env, compared with constant-time compare) |
| `ALLOWED_ORIGIN` | No | CORS allowed origin (defaults to `*`) |

### Structure

| Path | Purpose |
|------|---------|
| `bin/api/main.go` | Router, all HTTP handlers, server entrypoint |
| `bin/seed/seed.go` | Seeds 100 realistic competitors for local development |
| `internal/db/db.go` | GORM models, `Connect()`, `AutoMigrate()` |
| `internal/db/auth.go` | Auth models: `IPBlocklist`, `PINAttempt`, `StaffToken` |
| `internal/service/competitors.go` | `CompetitorService` — all competitor business logic |
| `internal/service/auth.go` | `AuthService` — PIN verification, token creation, IP blocking |
| `internal/middleware/auth.go` | `IPBlocklist` and `RequireToken` middleware, `GetClientIP`, `StaffFromContext` |

### API Endpoints

All endpoints except `/health` and `POST /api/auth/token` require a `Authorization: Bearer <token>` header.
The `IPBlocklist` middleware applies globally — blocked IPs cannot reach any endpoint.

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/health` | None | Liveness/readiness probe |
| POST | `/api/auth/token` | None | Verify access code + register name → returns bearer token |
| GET | `/api/competitors` | Required | List all; optional `?search=` searches first name, last name, and full name |
| GET | `/api/competitors/{id}` | Required | Get single competitor |
| POST | `/api/competitors` | Required | Create competitor |
| PATCH | `/api/competitors/{id}/checkin` | Required | Mark checked in; sets `checkInDateTime` and `checkedInBy` from auth token |
| PATCH | `/api/competitors/{id}/dob` | Required | Update date of birth `{"dateOfBirth": "2005-03-15T00:00:00Z"}` |
| PATCH | `/api/competitors/{id}/validate` | Required | Mark competitor as validated (`validated = true`) |
| DELETE | `/api/competitors/{id}` | Required | Delete competitor |

### Data models

```go
type Competitor struct {
    ID                  string     // UUID, auto-generated
    NameFirst           string
    NameLast            string
    DateOfBirth         time.Time
    RequiresValidation  bool       // set true for competitors requiring identity check (typically minors)
    Validated           bool       // set true once staff has verified identity; false = not yet validated
    IsCheckedIn         bool
    CheckInDateTime     *time.Time // null until checked in
    CheckedInBy         string     // staff member's full name from auth token at time of check-in
    ShirtSize           string
    Email               string
    Teacher             string
    Studio              string
    LastRegisteredEvent string     // valid values: glr-2026, nat-2025, glr-2025, nat-2024
}

type StaffToken struct {
    ID        string    // UUID
    Token     string    // 32-byte random hex, used as Bearer token
    FirstName string
    LastName  string
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
- 3 failed attempts from the same IP → IP is added to `ip_blocklists` and permanently blocked
- IP is determined from `CF-Connecting-IP` header (set by Cloudflare Tunnel), falling back to `X-Forwarded-For`, then `RemoteAddr`
- Tokens do not expire automatically — expire manually via the database post-event
- `StaffFromContext(ctx)` helper retrieves the authenticated staff member inside any handler

---

## Frontend

### Running

```bash
cd agm-checkin-frontend
npm install
cp .env.example .env.local   # set VITE_API_URL if backend isn't on :8080
npm run dev
```

### Structure

| Path | Purpose |
|------|---------|
| `src/api/auth.js` | `requestToken(code, firstName, lastName)` — auth API call |
| `src/api/competitors.js` | All competitor fetch calls; injects Bearer token; redirects to `/login` on 401 |
| `src/context/AuthContext.jsx` | Auth state (token + staff name), persisted in localStorage under `agm_token` / `agm_staff` |
| `src/theme.js` | MUI theme (Montserrat font, primary `#1565C0`) |
| `src/App.jsx` | `ColorModeContext`, `AuthProvider`, `ProtectedRoute`, `AppLayout` |
| `src/components/NavBar.jsx` | Responsive nav: hamburger + Drawer on mobile, full nav on desktop (`md` breakpoint) |
| `src/components/CompetitorCard.jsx` | Card used on Check-In page; shows age, studio, teacher, shirt size; handles validation dialog |
| `src/pages/LoginPage.jsx` | Two-step login: access code → name → token stored in localStorage |
| `src/pages/CheckInPage.jsx` | `/home` — debounced server-side search + check-in |
| `src/pages/CompetitorsPage.jsx` | `/competitors` — card list on mobile, sortable table on desktop |
| `src/pages/StatsPage.jsx` | `/stats` — summary stats, donut chart (status), bar chart (check-ins by day) |

### Auth flow

1. Any unauthenticated route → redirect to `/login`
2. Step 1: "Access code" field (password type, no hints about format or length)
3. Step 2: First name + Last name (separate fields)
4. On success: token and staff name written to localStorage; user lands on `/home`
5. Token is sent as `Authorization: Bearer <token>` on every API request
6. Any 401 response clears localStorage and hard-redirects to `/login`

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
```

See `DEPLOYMENT.md` for full infrastructure details.
