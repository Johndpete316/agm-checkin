# Service Layer

All business logic lives in `internal/service/`. Services receive a `*gorm.DB` at construction time and handle database access directly. They do not know about HTTP — that boundary stays in handlers. Audit logging is done in handlers (not services) because only handlers have the staff context and client IP.

---

## CompetitorService

**File:** `internal/service/competitors.go`

**Constructed by:** `service.NewCompetitorService(database *gorm.DB) *CompetitorService`

### Response types

#### CompetitorWithCheckIn

The standard response shape for all list and detail endpoints. Embeds the full `Competitor` struct and adds the current-event check-in record.

```go
type CompetitorWithCheckIn struct {
    db.Competitor
    CurrentCheckIn *db.CompetitorEvent `json:"currentCheckIn"`
}
```
`CurrentCheckIn` is `nil` if the competitor has no `CompetitorEvent` row for the current event.

#### CompetitorEventWithEvent

Used for the per-competitor history endpoint. Embeds a `CompetitorEvent` and adds the full `Event` record.

```go
type CompetitorEventWithEvent struct {
    db.CompetitorEvent
    Event db.Event `json:"event"`
}
```

#### ImportRow

Input type for `BulkImport`. Parsed from a normalized CSV row in the handler.

```go
type ImportRow struct {
    NameFirst          string
    NameLast           string
    Studio             string
    Teacher            string
    Email              string
    ShirtSize          string
    DateOfBirth        *time.Time  // nil if unknown
    RequiresValidation bool
    Validated          bool
    Events             []string    // event IDs oldest→newest, e.g. ["nat-2024","glr-2026"]
}
```

#### FieldConflict

Returned inside `ImportResult` when an import row has a non-blank value for a field that differs from the existing record's non-blank value. Both sides must be non-blank to qualify as a conflict (blank-to-value is auto-filled silently). Dates use `YYYY-MM-DD` string format.

```go
type FieldConflict struct {
    CompetitorID  string `json:"competitorId"`
    Name          string `json:"name"`           // "First Last"
    Field         string `json:"field"`           // "email" | "studio" | "teacher" | "shirtSize" | "dateOfBirth"
    ExistingValue string `json:"existingValue"`
    ImportValue   string `json:"importValue"`
}
```

#### ImportResult

Return type for `BulkImport`.

```go
type ImportResult struct {
    CompetitorsCreated int             `json:"competitorsCreated"`
    CompetitorsMatched int             `json:"competitorsMatched"`
    FieldsUpdated      int             `json:"fieldsUpdated"`
    EventsCreated      int             `json:"eventsCreated"`
    EventEntriesAdded  int             `json:"eventEntriesAdded"`
    FieldConflicts     []FieldConflict `json:"fieldConflicts,omitempty"`
    Errors             []string        `json:"errors,omitempty"`
}
```

### Methods

#### GetAll(search string, adminView bool) ([]CompetitorWithCheckIn, error)

Lists competitors with current-event check-in records attached.

- **adminView = true:** returns all competitors; search applies globally
- **adminView = false:** filters to competitors where `last_registered_event` equals the current event ID. If no current event is set, returns an empty slice.
- **search:** if non-empty, adds `ILIKE` filter on `name_first`, `name_last`, and the concatenated full name

After fetching competitors, makes a second query to `competitor_events` to bulk-fetch check-in records for the current event, then joins them in memory.

#### GetByID(id string) (*CompetitorWithCheckIn, error)

Fetches a single competitor by UUID primary key, then fetches their `CompetitorEvent` for the current event (if any). Returns `nil, err` if not found.

#### Create(competitor *db.Competitor) error

Inserts a new competitor. The `BeforeCreate` GORM hook auto-generates the UUID `id`.

#### CheckIn(id string, staffName string) (*CompetitorWithCheckIn, error)

Marks a competitor as checked in for the current event:

1. Fetches the competitor; returns error if not found
2. Reads the current event ID; returns `ErrNoCurrentEvent` if none is set
3. Upserts a `CompetitorEvent` row using `ON CONFLICT (competitor_id, event_id) DO UPDATE` to set `checked_in = true`, `check_in_datetime = now()`, `checked_in_by = staffName`
4. Re-fetches the `CompetitorEvent` to get the ID (needed if the row was updated rather than inserted)
5. If `last_registered_event` does not already match the current event, updates it

**Returns:** `ErrNoCurrentEvent` if no current event is set.

#### UpdateDOB(id string, dob time.Time) (*db.Competitor, error)

Updates the `date_of_birth` column on a single competitor. Part of the validation flow.

#### Validate(id string) (*db.Competitor, error)

Sets `validated = true` on a competitor. Does not affect `requires_validation`.

#### UpdateContact(id string, note *string, email *string) (*db.Competitor, error)

Updates only the `note` and/or `email` fields on a competitor. Pointer arguments distinguish "not provided" (nil — field is left unchanged) from "explicitly cleared" (pointer to empty string — field is set to `""`). Uses a `map[string]any` GORM update so zero-value strings are honoured. Returns the competitor with updated field values reflected in the struct.

#### Update(id string, input db.Competitor) (*db.Competitor, error)

Full record update using GORM `Save`. Copies the existing `id` onto the input struct (preserving identity) then saves all fields. Used by the admin edit dialog.

#### Delete(id string) error

Deletes the competitor row by UUID. GORM soft-delete is not used — this is a hard delete.

#### GetEventHistory(competitorID string) ([]CompetitorEventWithEvent, error)

Fetches all `CompetitorEvent` rows for a competitor, then bulk-fetches the associated `Event` rows, joining them in memory. Returns an empty slice if no entries exist.

#### BulkImport(rows []ImportRow) (*ImportResult, error)

Imports a list of competitors and their event registrations:

1. Creates timestamped backup tables: `competitors_backup_<unix>` and `competitor_events_backup_<unix>` via raw SQL
2. Loads all existing competitors and builds a `(lower(first)|lower(last)) → []Competitor` lookup map
3. Collects all referenced event IDs; for any that don't exist in `events`, creates a stub record (zero dates, display name derived from slug)
4. Classifies each row:
   - **Ambiguous** (2+ existing competitors share the name): skipped, added to `Errors`
   - **Matched** (exactly 1 existing competitor): merges string fields and date of birth — blank DB fields are auto-filled, conflicting non-blank fields produce a `FieldConflict` entry. Fields never touched on an existing record: `requires_validation`, `validated`, `note`, `last_registered_event`
   - **No match**: queued for bulk creation with all fields from the import row; `LastRegisteredEvent` is set to the most recent event according to canonical order `["nat-2024", "glr-2025", "nat-2025", "glr-2026"]`
5. Bulk-inserts new `Competitor` records (UUIDs assigned via `BeforeCreate` GORM hook)
6. Bulk-inserts `CompetitorEvent` rows using `ON CONFLICT DO NOTHING` — safe to re-run

**Errors:** Returns a fatal error if backup table creation or bulk competitor insertion fails. Non-fatal errors (stub event creation failures, ambiguous name collisions, parse errors from the handler) are collected in `ImportResult.Errors`.

---

## EventService

**File:** `internal/service/events.go`

**Constructed by:** `service.NewEventService(database *gorm.DB) *EventService`

**Errors defined:**
- `ErrEventNotFound` — returned when the target event ID does not exist
- `ErrNoCurrentEvent` — returned when no event has `is_current = true`

### Methods

#### List() ([]db.Event, error)

Returns all events ordered by `start_date DESC`.

#### Create(event *db.Event) error

Inserts a new event. The `id` is a human-readable slug (e.g. `glr-2027`) set by the caller; it is not auto-generated.

#### GetCurrent() (*db.Event, error)

Returns the first event where `is_current = true`. Returns `ErrNoCurrentEvent` if none exists.

#### SetCurrent(id string) (*db.Event, error)

In a single database transaction:
1. Clears `is_current` from all events
2. Sets `is_current = true` on the target event
3. Re-fetches and returns the updated event

Returns `ErrEventNotFound` if `RowsAffected == 0` on the update.

---

## StaffService

**File:** `internal/service/staff.go`

**Constructed by:** `service.NewStaffService(database *gorm.DB) *StaffService`

**Errors defined:**
- `ErrStaffNotFound` — target token ID does not exist
- `ErrInvalidRole` — role is not `"registration"` or `"admin"`
- `ErrCannotSelfEdit` — the requestor is trying to modify their own token

### Methods

#### GetByID(id string) (*db.StaffToken, error)

Fetches a single staff token by UUID. Returns `nil, err` if not found.

#### List() ([]db.StaffToken, error)

Returns all staff tokens ordered by `created_at ASC`.

#### UpdateRole(id, role, requestorID string) (*db.StaffToken, error)

Validates that `id != requestorID` and that `role` is one of the two valid values, then updates the role field with GORM `Save`.

#### Revoke(id, requestorID string) error

Validates `id != requestorID`, then hard-deletes the staff token record. Returns `ErrStaffNotFound` if `RowsAffected == 0`.

---

## AuthService

**File:** `internal/service/auth.go`

**Constructed by:** `service.NewAuthService(database *gorm.DB, pin string) *AuthService`

**Errors defined:**
- `ErrIPBlocked` — IP is in the blocklist
- `ErrInvalidPIN` — PIN does not match
- `ErrTooManyAttempts` — 3 failed attempts reached; IP has been blocked

**Constant:** `maxPINAttempts = 3`

### Methods

#### IsIPBlocked(ip string) bool

Counts `ip_blocklists` rows matching the IP address. Returns `true` if count > 0.

#### VerifyPINAndCreateToken(ip, pin, firstName, lastName string) (*db.StaffToken, error)

1. Fast-path call to `IsIPBlocked`; returns `ErrIPBlocked` immediately if blocked
2. Opens a database transaction
3. Acquires a PostgreSQL advisory lock keyed by the IP (`pg_advisory_xact_lock(hashtext(ip))`) to serialize concurrent login attempts from the same IP
4. Counts existing `pin_attempts` for the IP
5. If already at `maxPINAttempts`, calls `blockIPTx` and returns `ErrIPBlocked`
6. Compares PIN using `crypto/subtle.ConstantTimeCompare`
7. On mismatch: inserts a `PINAttempt` record; if this brings the count to the limit, calls `blockIPTx` and returns `ErrTooManyAttempts`; otherwise returns `ErrInvalidPIN`
8. On match: generates 32 random bytes, hex-encodes to form the token string; creates a `StaffToken` with UUID, token, name, and `CreatedAt = now()` (role defaults to `"registration"`)
9. Returns the created `StaffToken`

#### ValidateToken(token string) (*db.StaffToken, bool)

Looks up a `StaffToken` by the token string. If the record is not found, performs a dummy `ConstantTimeCompare` against a fixed 64-char zero string to prevent timing oracles. If found, performs a `ConstantTimeCompare` of the stored token against the provided token. Returns `(staffToken, true)` on success, `(nil, false)` on any failure.

---

## AuditService

**File:** `internal/service/audit.go`

**Constructed by:** `service.NewAuditService(database *gorm.DB) *AuditService`

### Types

#### LogEntry

Input type for `Log`. All fields are set by handlers.

```go
type LogEntry struct {
    ActorID    string
    ActorName  string
    Action     string          // e.g. "competitor.checked_in"
    EntityType string          // "competitor" | "staff_token" | "event"
    EntityID   string
    EntityName string          // human-readable snapshot at time of action
    Detail     map[string]any  // action-specific JSON fields
    IP         string
}
```

#### AuditLogView

API response shape for audit entries. Embeds the `AuditLog` model and adds a `Detail` field as parsed `json.RawMessage` instead of the raw string stored in the database.

```go
type AuditLogView struct {
    db.AuditLog
    Detail json.RawMessage `json:"detail"`
}
```

### Methods

#### Log(entry LogEntry)

Marshals `entry.Detail` to JSON, creates an `AuditLog` record with a new UUID and `CreatedAt = now()`, and inserts it into the database. **Errors are logged to stderr and never returned to the caller** — a failed audit write never breaks the primary operation.

#### List(action, actorName string, limit int) ([]AuditLogView, error)

Queries audit logs ordered by `created_at DESC`. Optional filters:
- `action`: exact match
- `actorName`: case-insensitive ILIKE
- `limit`: defaults to 100, capped at 500

Returns `[]AuditLogView` with `Detail` as parsed JSON.

---

## Related Pages

- [API Reference](api.md)
- [Database](database.md)
- [Middleware & Utilities](functions.md)
