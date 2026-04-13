# Backend Overview

The backend is a monolithic Go REST API. There are no microservices. All business logic — competitor management, event management, staff management, authentication, and audit logging — lives in a single binary served from `bin/api/main.go`.

---

## Purpose and Responsibilities

- Expose a REST API for the React frontend
- Authenticate staff with a shared access code and issue bearer tokens
- Enforce per-IP brute-force protection (3 failed attempts = permanent IP block)
- Enforce role-based access control (registration vs admin)
- Manage competitor records including check-in, identity validation, and bulk import
- Manage competition events and track the "current" event
- Manage staff tokens (list, promote, revoke)
- Write immutable audit log entries for all mutating operations
- AutoMigrate the database schema on startup

---

## Key Dependencies

| Library | Version | Purpose |
|---|---|---|
| `github.com/go-chi/chi/v5` | v5.1.0 | HTTP router |
| `github.com/go-chi/cors` | v1.2.1 | CORS middleware |
| `gorm.io/gorm` | v1.25.12 | ORM / query builder |
| `gorm.io/driver/postgres` | v1.5.9 | PostgreSQL GORM driver (pgx v5) |
| `github.com/google/uuid` | v1.6.0 | UUID generation |

---

## Request Lifecycle

Every request to a protected endpoint passes through this chain:

```
HTTP Request
    │
    ▼
chimw.Logger          — logs method, path, status, latency
cors.Handler          — sets CORS headers
chimw.Recoverer       — catches panics, returns 500
authmw.IPBlocklist    — checks ip_blocklists table; blocked IPs get 403 immediately
    │
    ├─── /health       — no auth, returns 200
    ├─── POST /api/auth/token  — no auth, calls AuthService
    │
    └─── authmw.RequireToken  — validates Bearer token via AuthService.ValidateToken
                               — injects *db.StaffToken into context
             │
             ├─── (standard routes) — any valid token
             │
             └─── authmw.RequireAdmin — checks role == "admin"; returns 403 otherwise
                      │
                      └─── (admin routes)
                                │
                                ▼
                           Handler function
                               │ extracts input
                               ▼
                           Service method
                               │ business logic + DB
                               ▼
                           respondJSON(w, status, data)
                               │
                               ▼
                           audit.Log(...)  ← fire-and-forget, never errors caller
```

---

## Auth and Authorization Model

### Shared PIN

All staff authenticate with a single shared access code (`AUTH_PIN` env var). There are no per-user passwords. The PIN is compared using `crypto/subtle.ConstantTimeCompare` to prevent timing attacks.

### Token issuance

When a staff member provides the correct PIN along with their first and last name, the server creates a `StaffToken` record in the database: a UUID ID, a 32-byte random hex token, the name fields, and the default role `"registration"`. The token is returned to the client and used as a Bearer token on all subsequent requests.

Tokens do not expire automatically. They must be revoked via the Manage Users page (admin) or directly in the database.

### IP blocking

Failed PIN attempts are tracked per IP in the `pin_attempts` table. After 3 failed attempts from the same IP, the IP is added to `ip_blocklists` and permanently blocked from all endpoints. The blocking logic uses a PostgreSQL advisory lock (`pg_advisory_xact_lock`) to serialize concurrent login attempts from the same IP, preventing race conditions in the attempt counter.

### Roles

| Role | Created by | Access |
|---|---|---|
| `registration` | Default on login | All non-admin endpoints |
| `admin` | Manual DB update or Manage Users page | All endpoints |

`RequireAdmin` middleware reads the `StaffToken` from context and returns 403 if the role is not `"admin"`.

### Role sync

The frontend calls `GET /api/auth/me` on page mount and on `visibilitychange` events to sync the staff role from the server. This ensures role changes made in the admin UI take effect without requiring a re-login.

---

## Source File Map

| File | Purpose |
|---|---|
| `bin/api/main.go` | Router setup, all HTTP handler functions, server entrypoint |
| `bin/seed/seed.go` | Seeds 100 realistic competitors for local development |
| `bin/import/main.go` | CSV normalization script: reads 4 raw historical CSVs, outputs one normalized CSV |
| `internal/db/db.go` | GORM models (`Competitor`, `Event`, `CompetitorEvent`, `AuditLog`), `Connect()`, `AutoMigrate()` |
| `internal/db/auth.go` | Auth models: `IPBlocklist`, `PINAttempt`, `StaffToken` |
| `internal/service/competitors.go` | `CompetitorService` — all competitor business logic including `BulkImport` |
| `internal/service/events.go` | `EventService` — event CRUD and current-event management |
| `internal/service/staff.go` | `StaffService` — staff token listing, role updates, revocation |
| `internal/service/auth.go` | `AuthService` — PIN verification, token creation, IP blocking |
| `internal/service/audit.go` | `AuditService` — writes audit log entries; `AuditLogView` for API responses |
| `internal/middleware/auth.go` | `IPBlocklist`, `RequireToken`, `RequireAdmin` middleware; `GetClientIP`, `StaffFromContext` helpers |

---

## Related Pages

- [API Reference](api.md)
- [Service Layer](services.md)
- [Middleware & Utilities](functions.md)
- [Database](database.md)
