# API Reference

All endpoints are served on port 8080. In production, they are exposed via Cloudflare Tunnel at `https://api.checkin.reduxit.net`.

Every response body is JSON with `Content-Type: application/json`. Error responses have the shape `{"error": "message string"}`.

---

## Authentication Summary

| Level | Requirement |
|---|---|
| **None** | No header required |
| **Bearer** | `Authorization: Bearer <token>` — token obtained from `POST /api/auth/token` |
| **Admin** | Bearer token with `role == "admin"` |

A globally applied `IPBlocklist` middleware blocks all endpoints for IPs that have 3+ failed login attempts.

---

## Endpoints

### GET /health

**Auth:** None  
**Handler:** inline in `main.go`  
**Description:** Liveness/readiness probe. Used by Kubernetes readiness and liveness probes.

**Response:** `200 OK` (empty body)

---

### POST /api/auth/token

**Auth:** None  
**Handler:** `createToken` in `main.go`  
**Service:** [`AuthService.VerifyPINAndCreateToken`](services.md#authservice)  
**Description:** Verify access code and register name → returns bearer token and role.

**Request body:**
```json
{
  "code": "string",
  "firstName": "string",
  "lastName": "string"
}
```

**Response `201 Created`:**
```json
{
  "token": "hex-string-64-chars",
  "firstName": "string",
  "lastName": "string",
  "role": "registration" | "admin"
}
```

**Error responses:**
| Status | Condition |
|---|---|
| `400` | Missing `code`, `firstName`, or `lastName` |
| `401` | Incorrect PIN (`ErrInvalidPIN`) |
| `403` | IP is blocked (`ErrIPBlocked`) or hit attempt limit (`ErrTooManyAttempts`) |
| `500` | Database or random token generation failure |

---

### GET /api/auth/me

**Auth:** Bearer  
**Handler:** inline in `main.go`  
**Description:** Returns the current staff token record. Used by the frontend to sync role on page focus.

**Response `200 OK`:** A [`StaffToken`](database.md#stafftoken) object.

---

### GET /api/competitors

**Auth:** Bearer  
**Handler:** `listCompetitors` in `main.go`  
**Service:** [`CompetitorService.GetAll`](services.md#competitorservice)  
**Description:** List competitors with their current-event check-in records.

**Query parameters:**
| Param | Type | Description |
|---|---|---|
| `search` | `string` (optional) | Case-insensitive ILIKE filter on `name_first`, `name_last`, or full name |

**Role-based behavior:**
- `admin`: returns all competitors in the database
- `registration`: returns only competitors whose `last_registered_event` matches the current event ID. If no current event is set, returns an empty array.

**Response `200 OK`:** Array of [`CompetitorWithCheckIn`](services.md#competitorwithcheckin) objects.

---

### GET /api/competitors/{id}

**Auth:** Bearer  
**Handler:** `getCompetitor` in `main.go`  
**Service:** [`CompetitorService.GetByID`](services.md#competitorservice)  
**Description:** Get a single competitor with their current-event check-in record.

**Path params:** `id` — competitor UUID

**Response `200 OK`:** A single [`CompetitorWithCheckIn`](services.md#competitorwithcheckin) object.

**Error responses:**
| Status | Condition |
|---|---|
| `404` | No competitor with that ID |

---

### POST /api/competitors

**Auth:** Bearer  
**Handler:** `createCompetitor` in `main.go`  
**Service:** [`CompetitorService.Create`](services.md#competitorservice)  
**Description:** Create a new competitor record.

**Request body:** A [`Competitor`](database.md#competitor) object (excluding `id`, which is auto-generated).

**Audit log:** `competitor.created` with detail `{studio, lastRegisteredEvent}`

**Response `201 Created`:** The created [`Competitor`](database.md#competitor) object with its generated ID.

---

### PATCH /api/competitors/{id}

**Auth:** Admin  
**Handler:** `updateCompetitor` in `main.go`  
**Service:** [`CompetitorService.Update`](services.md#competitorservice)  
**Description:** Update all fields of a competitor record. This is a full replace (GORM `Save`) — all editable fields must be included in the body.

**Path params:** `id` — competitor UUID

**Request body:** A [`Competitor`](database.md#competitor) object with updated field values.

**Audit log:** `competitor.updated` with detail `{studio, teacher, lastRegisteredEvent}`

**Response `200 OK`:** The updated [`Competitor`](database.md#competitor) object.

---

### PATCH /api/competitors/{id}/checkin

**Auth:** Bearer  
**Handler:** `checkInCompetitor` in `main.go`  
**Service:** [`CompetitorService.CheckIn`](services.md#competitorservice)  
**Description:** Mark the competitor as checked in for the current event. Creates or updates a `CompetitorEvent` row using an upsert. Also updates `last_registered_event` on the competitor record if it doesn't already match the current event.

**Path params:** `id` — competitor UUID

**Request body:** None

**Audit log:** `competitor.checked_in` with detail `{eventId}`

**Response `200 OK`:** A [`CompetitorWithCheckIn`](services.md#competitorwithcheckin) with the updated check-in record.

**Error responses:**
| Status | Condition |
|---|---|
| `409 Conflict` | No current event is set (`ErrNoCurrentEvent`) |
| `500` | Database error |

---

### PATCH /api/competitors/{id}/contact

**Auth:** Bearer  
**Handler:** `updateCompetitorContact` in `main.go`  
**Service:** [`CompetitorService.UpdateContact`](services.md#competitorservice)  
**Description:** Update a competitor's `note` and/or `email`. Available to all authenticated staff (registration and admin). Both fields are optional — only fields present in the request body are updated. To clear a field, send an empty string.

**Path params:** `id` — competitor UUID

**Request body:**
```json
{
  "note": "string (optional)",
  "email": "string (optional)"
}
```

**Audit log:** `competitor.contact_updated`

**Response `200 OK`:** The updated [`Competitor`](database.md#competitor) object.

---

### PATCH /api/competitors/{id}/dob

**Auth:** Bearer  
**Handler:** `updateDOB` in `main.go`  
**Service:** [`CompetitorService.UpdateDOB`](services.md#competitorservice)  
**Description:** Update the competitor's date of birth. Called as part of the validation flow when staff corrects an incorrect DOB before checking in.

**Path params:** `id` — competitor UUID

**Request body:**
```json
{
  "dateOfBirth": "2005-03-15T00:00:00Z"
}
```

**Audit log:** `competitor.dob_updated` with detail `{newDob: "2005-03-15"}`

**Response `200 OK`:** The updated [`Competitor`](database.md#competitor) object.

---

### PATCH /api/competitors/{id}/validate

**Auth:** Bearer  
**Handler:** `validateCompetitor` in `main.go`  
**Service:** [`CompetitorService.Validate`](services.md#competitorservice)  
**Description:** Mark the competitor as validated (`validated = true`). Called after staff has verified the competitor's identity.

**Path params:** `id` — competitor UUID

**Request body:** None

**Audit log:** `competitor.validated`

**Response `200 OK`:** The updated [`Competitor`](database.md#competitor) object.

---

### DELETE /api/competitors/{id}

**Auth:** Bearer  
**Handler:** `deleteCompetitor` in `main.go`  
**Service:** [`CompetitorService.Delete`](services.md#competitorservice)  
**Description:** Delete a competitor record. The handler fetches the competitor's name before deletion for the audit record.

**Path params:** `id` — competitor UUID

**Audit log:** `competitor.deleted`

**Response `204 No Content`**

---

### GET /api/competitors/{id}/events

**Auth:** Bearer  
**Handler:** `getCompetitorEvents` in `main.go`  
**Service:** [`CompetitorService.GetEventHistory`](services.md#competitorservice)  
**Description:** Retrieve the full event history (all `CompetitorEvent` records) for a competitor.

**Path params:** `id` — competitor UUID

**Response `200 OK`:** Array of [`CompetitorEventWithEvent`](services.md#competitoreventwithvent) objects.

---

### POST /api/competitors/import

**Auth:** Admin  
**Handler:** `bulkImportCompetitors` in `main.go`  
**Service:** [`CompetitorService.BulkImport`](services.md#competitorservice)  
**Description:** Bulk-import competitors from a normalized CSV file. The handler parses the CSV and delegates to `BulkImport`. Before writing, snapshot backup tables are created in the database (`competitors_backup_<ts>` and `competitor_events_backup_<ts>`).

**Request:** `multipart/form-data` with a field named `file` containing the CSV. Max upload size: 32 MB.

**CSV format** — header row must contain these columns (order-independent, extra columns ignored):

| Column | Format |
|---|---|
| `first_name` | string |
| `last_name` | string |
| `studio` | string |
| `teacher` | string |
| `email` | string |
| `shirt_size` | string |
| `date_of_birth` | `YYYY-MM-DD` or blank |
| `requires_validation` | `true`/`false` |
| `validated` | `true`/`false` |
| `events` | pipe-delimited event IDs, e.g. `nat-2024\|glr-2025` |

**Merge behavior:** Rows are matched to existing competitors by (first\_name, last\_name) case-insensitively.
- **New record:** created with all fields from the import row.
- **Matched, field blank in DB:** the import value is written automatically (`fieldsUpdated` count increases).
- **Matched, field set in both and different:** a `FieldConflict` is returned; the existing DB value is left unchanged. The UI prompts the user to resolve each conflict.
- **Fields never overwritten on an existing record:** `requires_validation`, `validated`, `note`, `last_registered_event`, and all check-in records.
- **Event registrations:** added for any event IDs in the import row that the competitor is not yet registered for. Existing registrations are never removed (`ON CONFLICT DO NOTHING`).
- **Ambiguous name (2+ matches):** row is skipped, added to `errors`.

**Audit log:** `competitor.bulk_import` with detail `{competitorsCreated, eventsCreated, eventEntriesAdded}`

**Response `200 OK`:**
```json
{
  "competitorsCreated": 0,
  "competitorsMatched": 0,
  "fieldsUpdated": 0,
  "eventsCreated": 0,
  "eventEntriesAdded": 0,
  "fieldConflicts": [
    {
      "competitorId": "uuid",
      "name": "Jane Smith",
      "field": "email",
      "existingValue": "old@example.com",
      "importValue": "new@example.com"
    }
  ],
  "errors": ["optional array of non-fatal row errors"]
}
```
`fieldConflicts` and `errors` are omitted when empty.

---

### GET /api/events

**Auth:** Bearer  
**Handler:** `listEvents` in `main.go`  
**Service:** [`EventService.List`](services.md#eventservice)  
**Description:** List all events sorted by `start_date` descending.

**Response `200 OK`:** Array of [`Event`](database.md#event) objects.

---

### GET /api/events/current

**Auth:** Bearer  
**Handler:** `getCurrentEvent` in `main.go`  
**Service:** [`EventService.GetCurrent`](services.md#eventservice)  
**Description:** Get the event marked as current (`is_current = true`).

**Response `200 OK`:** A single [`Event`](database.md#event) object, or `null` if no current event is set.

---

### POST /api/events

**Auth:** Admin  
**Handler:** `createEvent` in `main.go`  
**Service:** [`EventService.Create`](services.md#eventservice)  
**Description:** Create a new event. `id` and `name` are required; `startDate` and `endDate` are optional.

**Request body:**
```json
{
  "id": "glr-2027",
  "name": "GLR 2027",
  "startDate": "2027-03-14T00:00:00Z",
  "endDate": "2027-03-16T00:00:00Z"
}
```

**Audit log:** `event.created` with detail `{startDate, endDate}`

**Response `201 Created`:** The created [`Event`](database.md#event) object.

---

### PATCH /api/events/{id}/current

**Auth:** Admin  
**Handler:** `setCurrentEvent` in `main.go`  
**Service:** [`EventService.SetCurrent`](services.md#eventservice)  
**Description:** Set the specified event as the current event. Clears `is_current` from all other events in a single transaction.

**Path params:** `id` — event slug (e.g. `glr-2027`)

**Audit log:** `event.set_current`

**Response `200 OK`:** The updated [`Event`](database.md#event) object with `isCurrent: true`.

**Error responses:**
| Status | Condition |
|---|---|
| `404` | No event with that ID |

---

### GET /api/staff

**Auth:** Admin  
**Handler:** `listStaff` in `main.go`  
**Service:** [`StaffService.List`](services.md#staffservice)  
**Description:** List all staff tokens, ordered by `created_at` ascending.

**Response `200 OK`:** Array of [`StaffToken`](database.md#stafftoken) objects.

---

### PATCH /api/staff/{id}/role

**Auth:** Admin  
**Handler:** `updateStaffRole` in `main.go`  
**Service:** [`StaffService.UpdateRole`](services.md#staffservice)  
**Description:** Update a staff member's role. Cannot update your own token (`ErrCannotSelfEdit`).

**Path params:** `id` — staff token UUID

**Request body:**
```json
{
  "role": "admin"
}
```
Valid values: `"admin"`, `"registration"`.

**Audit log:** `staff.role_updated` with detail `{oldRole, newRole}`

**Response `200 OK`:** The updated [`StaffToken`](database.md#stafftoken) object.

**Error responses:**
| Status | Condition |
|---|---|
| `400` | Invalid role value or attempting to edit own token |
| `404` | No staff token with that ID |

---

### DELETE /api/staff/{id}

**Auth:** Admin  
**Handler:** `revokeStaff` in `main.go`  
**Service:** [`StaffService.Revoke`](services.md#staffservice)  
**Description:** Permanently delete a staff token. The staff member will need to log in again to get a new token. Cannot revoke your own token.

**Path params:** `id` — staff token UUID

**Audit log:** `staff.revoked`

**Response `204 No Content`**

**Error responses:**
| Status | Condition |
|---|---|
| `400` | Attempting to revoke own token |
| `404` | No staff token with that ID |

---

### GET /api/audit

**Auth:** Admin  
**Handler:** `listAudit` in `main.go`  
**Service:** [`AuditService.List`](services.md#auditservice)  
**Description:** List audit log entries, most recent first.

**Query parameters:**
| Param | Type | Default | Description |
|---|---|---|---|
| `action` | `string` | (all) | Filter by exact action string (e.g. `competitor.checked_in`) |
| `actor` | `string` | (all) | Filter by actor name (case-insensitive ILIKE) |
| `limit` | `int` | 100 | Max entries to return; capped at 500 |

**Response `200 OK`:** Array of [`AuditLogView`](services.md#auditservice) objects (detail as parsed JSON, not raw string).

---

## Audit Action Reference

| Action string | Triggered by |
|---|---|
| `competitor.created` | `POST /api/competitors` |
| `competitor.updated` | `PATCH /api/competitors/{id}` |
| `competitor.deleted` | `DELETE /api/competitors/{id}` |
| `competitor.checked_in` | `PATCH /api/competitors/{id}/checkin` |
| `competitor.dob_updated` | `PATCH /api/competitors/{id}/dob` |
| `competitor.contact_updated` | `PATCH /api/competitors/{id}/contact` |
| `competitor.validated` | `PATCH /api/competitors/{id}/validate` |
| `competitor.bulk_import` | `POST /api/competitors/import` |
| `event.created` | `POST /api/events` |
| `event.set_current` | `PATCH /api/events/{id}/current` |
| `staff.role_updated` | `PATCH /api/staff/{id}/role` |
| `staff.revoked` | `DELETE /api/staff/{id}` |

---

## Related Pages

- [Service Layer](services.md)
- [Middleware & Utilities](functions.md)
- [Database](database.md)
- [Backend Overview](README.md)
