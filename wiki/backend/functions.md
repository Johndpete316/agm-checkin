# Middleware and Utility Functions

**File:** `internal/middleware/auth.go`

All middleware and helper functions live in the `middleware` package. They are applied in `main.go` using chi's router grouping.

---

## Context Key

```go
type contextKey string
const StaffTokenKey contextKey = "staffToken"
```

A typed context key used to store and retrieve the authenticated `*db.StaffToken` in the request context. Using a typed key prevents collisions with other packages that also use string keys.

---

## GetClientIP

```go
func GetClientIP(r *http.Request) string
```

Returns the real client IP address. Checks headers in priority order:

1. `CF-Connecting-IP` — set by Cloudflare Tunnel when traffic passes through the tunnel. This is the authoritative IP when running in production.
2. `X-Forwarded-For` — takes the first IP (leftmost) from the comma-separated list.
3. `r.RemoteAddr` — the direct TCP connection address. Strips the port suffix.

Used in every handler that logs an audit entry, and in `IPBlocklist` middleware for the blocklist lookup.

---

## StaffFromContext

```go
func StaffFromContext(ctx context.Context) *db.StaffToken
```

Retrieves the `*db.StaffToken` stored in the request context by `RequireToken`. Returns `nil` if the value is not present (i.e., on unprotected routes such as `/health` or `POST /api/auth/token`).

Used in:
- `actorFrom()` helper in `main.go` — extracts actor ID and name for audit entries
- `listCompetitors` handler — reads `staff.Role` to determine admin vs. registration view
- `updateStaffRole` and `revokeStaff` handlers — reads `requestor.ID` to prevent self-edit
- `RequireAdmin` middleware — reads `staff.Role` to enforce admin access

---

## IPBlocklist (middleware)

```go
func IPBlocklist(authSvc *service.AuthService) func(http.Handler) http.Handler
```

Applied globally to all routes (before the auth group). On every request:

1. Calls `GetClientIP(r)` to determine the client IP
2. Calls `authSvc.IsIPBlocked(ip)` — queries the `ip_blocklists` table
3. If blocked: responds with `403 {"error": "access denied"}` and stops the middleware chain
4. If not blocked: calls `next.ServeHTTP(w, r)`

Because this middleware applies before all route handlers including `/health` and `POST /api/auth/token`, a blocked IP cannot reach any endpoint at all.

**Applied in:** `r.Use(authmw.IPBlocklist(authSvc))` before all route registrations.

---

## RequireToken (middleware)

```go
func RequireToken(authSvc *service.AuthService) func(http.Handler) http.Handler
```

Applied to all protected route groups. On every request:

1. Reads the `Authorization` header
2. If the header does not start with `"Bearer "`: responds with `401 {"error": "unauthorized"}` and stops
3. Strips the `"Bearer "` prefix to get the raw token string
4. Calls `authSvc.ValidateToken(token)` — queries the `staff_tokens` table and compares tokens using `ConstantTimeCompare`
5. If invalid: responds with `401 {"error": "unauthorized"}` and stops
6. If valid: stores the `*db.StaffToken` in the request context under `StaffTokenKey`
7. Calls `next.ServeHTTP(w, r.WithContext(ctx))`

**Applied in:** `r.Group(func(r chi.Router) { r.Use(authmw.RequireToken(authSvc)); ... })`

---

## RequireAdmin (middleware)

```go
func RequireAdmin(next http.Handler) http.Handler
```

Applied inside the `RequireToken` group, as a further gate for admin-only routes. This middleware has no dependency on `AuthService` — it reads the already-authenticated `StaffToken` from context.

On every request:

1. Calls `StaffFromContext(r.Context())`
2. If `staff == nil` or `staff.Role != "admin"`: responds with `403 {"error": "admin access required"}` and stops
3. Otherwise: calls `next.ServeHTTP(w, r)`

**Applied in:** `r.Group(func(r chi.Router) { r.Use(authmw.RequireAdmin); ... })`

**Note:** `RequireAdmin` must always be nested inside a `RequireToken` group. It does not perform token validation itself — it assumes the token is already validated and the staff is in context.

---

## actorFrom (handler utility)

```go
func actorFrom(r *http.Request) (id, name string)
```

Defined in `main.go` (not in the middleware package). A convenience wrapper around `StaffFromContext` that extracts the actor ID and formatted full name for use in audit log entries. Returns `("", "unknown")` if staff is not in context.

---

## respondJSON (handler utility)

```go
func respondJSON(w http.ResponseWriter, status int, data any)
```

Defined in `main.go`. Sets `Content-Type: application/json`, writes the status code, and encodes `data` as JSON to the response body. Used by every handler for both success and error responses.

---

## Related Pages

- [Backend Overview](README.md)
- [API Reference](api.md)
- [Service Layer](services.md)
- [External Integrations](external-integrations.md)
