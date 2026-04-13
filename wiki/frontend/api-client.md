# API Client

All API communication is handled by thin fetch-based modules in `src/api/`. There is no axios, SWR, React Query, or other HTTP library. Each module exports named async functions.

**Base URL:** `import.meta.env.VITE_API_URL || 'http://localhost:8080'`  
This is resolved at **build time** by Vite. In production, `VITE_API_URL` is injected as a Docker build argument (`--build-arg VITE_API_URL=https://apicheckin.reduxit.net`).

---

## Common Pattern: apiFetch

Most modules define a local `apiFetch` helper:

```js
async function apiFetch(url, options = {}) {
  const res = await fetch(url, {
    ...options,
    headers: { ...options.headers, ...authHeaders() },
  })
  if (res.status === 401) {
    localStorage.removeItem('agm_token')
    localStorage.removeItem('agm_staff')
    window.location.href = '/login'
    throw new Error('unauthorized')
  }
  return res
}
```

`authHeaders()` reads `agm_token` from `localStorage` and returns `{ Authorization: 'Bearer <token>' }` (or empty object if no token).

**On 401:** Clears localStorage and performs a hard redirect to `/login`. This is separate from the AuthContext `logout()` function — it is a bailout path for any authenticated request that returns unauthorized.

The `staff.js` module does not use `apiFetch` — it accepts the token as a parameter and handles errors differently (throwing `'unauthorized'` or `'forbidden'` strings for the caller to handle).

---

## src/api/auth.js

**Purpose:** Authentication endpoint.  
**No `apiFetch` — this module does not inject auth headers** (login is unauthenticated).

### `requestToken(pin, firstName, lastName)`

**API call:** `POST /api/auth/token`  
**Body:** `{ code: pin, firstName, lastName }`

**Returns:** `{ token, firstName, lastName, role }`

**Throws:**
| Error message | HTTP status | Meaning |
|---|---|---|
| `'invalid_auth'` | 401 | Wrong PIN |
| `'blocked'` | 403 | IP is blocked |
| `'server_error'` | other non-OK | Unexpected error |

---

## src/api/competitors.js

**Purpose:** All competitor-related API calls.  
**Uses:** `apiFetch` with auto-injected auth headers.

### `getCompetitors(search = '')`

**API call:** `GET /api/competitors?search=<encoded>`  
**Returns:** `CompetitorWithCheckIn[]`

### `getCompetitor(id)`

**API call:** `GET /api/competitors/{id}`  
**Returns:** `CompetitorWithCheckIn`

### `createCompetitor(data)`

**API call:** `POST /api/competitors` with JSON body  
**Returns:** `Competitor` (the created record)

### `checkInCompetitor(id)`

**API call:** `PATCH /api/competitors/{id}/checkin` (no body)  
**Returns:** `CompetitorWithCheckIn`

### `updateCompetitorDOB(id, dateOfBirth)`

**API call:** `PATCH /api/competitors/{id}/dob`  
**Body:** `{ dateOfBirth: "<date>T00:00:00Z" }` — the `dateOfBirth` param is a YYYY-MM-DD string; the function appends `T00:00:00Z`  
**Returns:** `Competitor`

### `validateCompetitor(id)`

**API call:** `PATCH /api/competitors/{id}/validate` (no body)  
**Returns:** `Competitor`

### `updateCompetitor(id, data)`

**API call:** `PATCH /api/competitors/{id}` with JSON body  
**Returns:** `Competitor` (full updated record)

### `getCompetitorEvents(id)`

**API call:** `GET /api/competitors/{id}/events`  
**Returns:** `CompetitorEventWithEvent[]`

### `deleteCompetitor(id)`

**API call:** `DELETE /api/competitors/{id}`  
**Returns:** void (no response body expected)

### `importCompetitors(file)`

**API call:** `POST /api/competitors/import` with `multipart/form-data` body containing `file` field  
**Returns:** `{ competitorsCreated, eventsCreated, eventEntriesAdded, errors? }`  
**Error handling:** Attempts to parse the JSON error body on non-OK response; throws with `body.error || 'Import failed'`

---

## src/api/events.js

**Purpose:** Event management API calls.  
**Uses:** `apiFetch` with auto-injected auth headers.

### `listEvents()`

**API call:** `GET /api/events`  
**Returns:** `Event[]`

### `getCurrentEvent()`

**API call:** `GET /api/events/current`  
**Returns:** `Event | null`

### `createEvent(data)`

**API call:** `POST /api/events` with JSON body `{ id, name, startDate, endDate }`  
**Returns:** `Event`  
**Error handling:** Parses JSON error body; throws with `body.error || 'Failed to create event'`

### `setCurrentEvent(id)`

**API call:** `PATCH /api/events/{id}/current` (no body)  
**Returns:** `Event`  
**Error handling:** Parses JSON error body; throws with `body.error || 'Failed to set current event'`

---

## src/api/staff.js

**Purpose:** Staff management API calls.  
**Note:** This module does NOT use `apiFetch`. It accepts the token as an explicit parameter and uses a `handleResponse` helper that throws string literals (`'unauthorized'`, `'forbidden'`, or `body.error`) rather than auto-redirecting.

```js
async function handleResponse(res) {
  if (res.status === 401) throw new Error('unauthorized')
  if (res.status === 403) throw new Error('forbidden')
  if (!res.ok) {
    const body = await res.json().catch(() => ({}))
    throw new Error(body.error || 'server_error')
  }
  return res
}
```

### `listStaff(token)`

**API call:** `GET /api/staff` with `Authorization: Bearer <token>`  
**Returns:** `StaffToken[]`

### `updateStaffRole(token, id, role)`

**API call:** `PATCH /api/staff/{id}/role` with JSON body `{ role }`  
**Returns:** `StaffToken`

### `revokeStaff(token, id)`

**API call:** `DELETE /api/staff/{id}`  
**Returns:** void

---

## src/api/audit.js

**Purpose:** Audit log retrieval.  
**Uses:** `apiFetch` with auto-injected auth headers.

### `listAuditLogs({ action = '', actor = '', limit = 100 } = {})`

**API call:** `GET /api/audit?action=<>&actor=<>&limit=<>`  
**Query params:** Only appends params that are non-empty strings or non-zero numbers  
**Returns:** `AuditLogView[]` (detail is a parsed JSON object, not a string)

---

## 401 Handling Summary

| Module | On 401 |
|---|---|
| `auth.js` | Throws `Error('invalid_auth')` |
| `competitors.js` | Clears localStorage + hard redirect to `/login` |
| `events.js` | Clears localStorage + hard redirect to `/login` |
| `audit.js` | Clears localStorage + hard redirect to `/login` |
| `staff.js` | Throws `Error('unauthorized')` (caller handles) |

The `AuthContext` also handles 401 on the `syncRole` call by calling `logout()` which clears localStorage and sets auth state to null, causing `ProtectedRoute` to redirect.

---

## Related Pages

- [Frontend Overview](README.md)
- [State Management](state.md)
- [API Reference](../backend/api.md)
