# DB Review

## Summary

The schema is straightforward and mostly sound for the application's needs. The auth subsystem (`VerifyPINAndCreateToken`, `SetCurrent`) uses transactions and advisory locks correctly. However, there are several real correctness issues: **no foreign key constraints** on `CompetitorEvent` leave the door open for orphaned rows, the admin `Update` endpoint silently zeros boolean fields due to a `Save()` misuse, and the `CheckIn` and `BulkImport` flows perform multiple related writes without transactions. None of these are currently causing visible bugs in normal operation, but they will surface under concurrent use, careless API calls, or partial failures during imports.

---

## Findings

### 1. No foreign key constraints on CompetitorEvent

**Severity:** Critical
**Area:** Schema
**File/Location:** `internal/db/db.go:38-45` (`CompetitorEvent` model)

**Problem:**
`CompetitorEvent.CompetitorID` and `CompetitorEvent.EventID` reference `competitors.id` and `events.id` respectively, but no GORM foreign key tags are defined. GORM's `AutoMigrate` does not create FK constraints unless explicitly told to. This means:

- Deleting a Competitor via `DELETE /api/competitors/{id}` (competitors.go:529-530) leaves orphaned `CompetitorEvent` rows in the database.
- A `CompetitorEvent` can be created referencing a non-existent Competitor or Event ID.
- The `GetEventHistory` endpoint will return entries for events that may no longer exist.

**Proposed Change:**

Option A (recommended) -- add FK constraints via GORM tags with cascade delete on the Competitor side:

```go
type CompetitorEvent struct {
	ID              string     `gorm:"primaryKey;type:uuid" json:"id"`
	CompetitorID    string     `gorm:"not null;uniqueIndex:idx_competitor_event;constraint:OnDelete:CASCADE" json:"competitorId"`
	EventID         string     `gorm:"not null;uniqueIndex:idx_competitor_event;constraint:OnDelete:RESTRICT" json:"eventId"`
	CheckedIn       bool       `gorm:"not null;default:false" json:"checkedIn"`
	CheckInDatetime *time.Time `json:"checkInDatetime"`
	CheckedInBy     string     `json:"checkedInBy"`
}
```

Option B -- if GORM tags don't pick up FKs on existing tables, add them via raw migration:

```sql
ALTER TABLE competitor_events
  ADD CONSTRAINT fk_competitor
    FOREIGN KEY (competitor_id) REFERENCES competitors(id) ON DELETE CASCADE;

ALTER TABLE competitor_events
  ADD CONSTRAINT fk_event
    FOREIGN KEY (event_id) REFERENCES events(id) ON DELETE RESTRICT;
```

After adding constraints, clean up any existing orphaned rows first:

```sql
DELETE FROM competitor_events
WHERE competitor_id NOT IN (SELECT id FROM competitors);

DELETE FROM competitor_events
WHERE event_id NOT IN (SELECT id FROM events);
```

**Impact:** Without this fix, every competitor deletion silently corrupts the event history. CASCADE on Competitor means check-in records are cleaned up automatically. RESTRICT on Event prevents deleting an event that has registrations, which is the correct safeguard.

---

### 2. Update (admin edit) uses Save() which zeros boolean fields

**Severity:** High
**Area:** GORM
**File/Location:** `internal/service/competitors.go:517-527`, `bin/api/handlers_competitors.go:185-215`

**Problem:**
The admin `PATCH /api/competitors/{id}` handler decodes the request body into a `db.Competitor` struct and passes it to `Update()`, which calls `s.db.Save(&input)`. GORM's `Save()` writes **every field**, including zero values. Since `RequiresValidation` and `Validated` are `bool` (not `*bool`), if the JSON body omits them, they decode as `false`.

This means an admin editing a competitor's studio or teacher can silently reset `requiresValidation` to `false` and `validated` to `false`, breaking the validation workflow. The same applies to `DateOfBirth` (reset to zero time) if omitted.

**Proposed Change:**

Replace `Save()` with selective update. Fetch the existing record, apply only the fields that were provided:

```go
func (s *CompetitorService) Update(id string, input db.Competitor) (*db.Competitor, error) {
	var competitor db.Competitor
	if err := s.db.First(&competitor, "id = ?", id).Error; err != nil {
		return nil, err
	}

	if err := s.db.Model(&competitor).Updates(input).Error; err != nil {
		return nil, err
	}
	return &competitor, nil
}
```

However, `Updates` with a struct still skips zero values by default in GORM, which means you can't set a field TO false/empty. For a full admin edit that should be able to set any value, use a `map[string]any` approach in the handler: decode into a map, then call `Updates(map)`. This lets you distinguish "not sent" from "explicitly set to false/empty".

Alternatively, if the frontend always sends the complete object (verify this in `EditCompetitorDialog.jsx`), document and enforce that contract. But the API is still broken for any other caller.

**Impact:** Without this fix, the validation workflow can be silently broken by any admin edit. A competitor flagged as `requiresValidation=true` could have that flag cleared by an unrelated field update.

---

### 3. CheckIn performs multiple writes without a transaction

**Severity:** High
**Area:** Logic
**File/Location:** `internal/service/competitors.go:418-458`

**Problem:**
`CheckIn` does three separate DB operations:
1. Upsert `CompetitorEvent` (line 438-443)
2. Re-fetch the `CompetitorEvent` to get its ID (line 446)
3. Update `Competitor.LastRegisteredEvent` (line 451)

If step 3 fails, the competitor is checked in (CompetitorEvent exists) but `LastRegisteredEvent` is stale. This means the competitor disappears from the registration user's view (which filters by `last_registered_event = currentEvent`), even though they ARE checked in.

The re-fetch in step 2 also swallows errors -- if it fails, `ce.ID` is empty, which is returned to the handler and ultimately to the frontend.

**Proposed Change:**

Wrap in a transaction:

```go
func (s *CompetitorService) CheckIn(id string, staffName string) (*CompetitorWithCheckIn, error) {
	var competitor db.Competitor
	if err := s.db.First(&competitor, "id = ?", id).Error; err != nil {
		return nil, err
	}

	eventID := s.currentEventID()
	if eventID == "" {
		return nil, ErrNoCurrentEvent
	}

	now := time.Now()
	ce := db.CompetitorEvent{
		CompetitorID:    id,
		EventID:         eventID,
		CheckedIn:       true,
		CheckInDatetime: &now,
		CheckedInBy:     staffName,
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "competitor_id"}, {Name: "event_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"checked_in", "check_in_datetime", "checked_in_by"}),
		}).Create(&ce).Error; err != nil {
			return err
		}
		if err := tx.Where("competitor_id = ? AND event_id = ?", id, eventID).First(&ce).Error; err != nil {
			return err
		}
		if competitor.LastRegisteredEvent != eventID {
			if err := tx.Model(&competitor).Update("last_registered_event", eventID).Error; err != nil {
				return err
			}
			competitor.LastRegisteredEvent = eventID
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &CompetitorWithCheckIn{Competitor: competitor, CurrentCheckIn: &ce}, nil
}
```

**Impact:** Without this fix, a partial failure during check-in leaves the competitor in a state where they're checked in but invisible to registration staff. The transaction also surfaces the re-fetch error rather than silently returning an empty ID.

---

### 4. BulkImport has no transaction boundary

**Severity:** High
**Area:** Logic
**File/Location:** `internal/service/competitors.go:91-289`

**Problem:**
BulkImport performs 6 distinct steps as separate DB operations:
1. Create backup tables (raw SQL)
2. Load existing competitors
3. Auto-create stub events
4. Auto-fill fields on matched competitors (individual `Updates` per match)
5. Bulk-insert new competitors
6. Bulk-insert CompetitorEvents

If step 5 fails (e.g., unique constraint violation), steps 3 and 4 have already committed: stub events exist and some competitors have been updated. The backup tables exist but are never automatically used for rollback.

Additionally, step 4 issues one `UPDATE` per matched competitor inside a loop (line 218), which is both slow and non-atomic with the surrounding operations.

**Proposed Change:**

Wrap steps 3-6 in a single transaction. The backup table creation (step 1) should stay outside the transaction since it's a DDL safety net:

```go
// Step 1: backup (outside tx -- DDL)
// ...

// Steps 2-6: within transaction
err := s.db.Transaction(func(tx *gorm.DB) error {
    // load existing (step 2) -- read against tx for consistency
    // auto-create events (step 3)
    // classify and auto-fill (step 4) -- use tx for Updates
    // bulk insert competitors (step 5)
    // bulk insert CompetitorEvents (step 6)
    return nil
})
```

**Impact:** Without this fix, a failed import can leave the database in a state where some competitors are updated, some events are created, but the import is "failed." The backup tables exist but require manual SQL intervention to restore. With a transaction, the entire import either succeeds or rolls back completely.

---

### 5. Missing NOT NULL on Competitor.NameFirst and NameLast

**Severity:** Medium
**Area:** Schema
**File/Location:** `internal/db/db.go:13-14`

**Problem:**
`NameFirst` and `NameLast` have no `gorm:"not null"` tag. The API can accept a POST with blank or missing name fields, creating a competitor with empty strings for both names. This competitor would be unmatchable in BulkImport (empty name key), unsearchable, and displayed as blank in the UI.

**Proposed Change:**

```go
NameFirst string `gorm:"not null" json:"nameFirst"`
NameLast  string `gorm:"not null" json:"nameLast"`
```

Add application-level validation in the Create handler to reject blank names before they hit the database:

```go
if strings.TrimSpace(input.NameFirst) == "" || strings.TrimSpace(input.NameLast) == "" {
    respondJSON(w, http.StatusBadRequest, map[string]string{"error": "first and last name are required"})
    return
}
```

Note: `NOT NULL` alone allows empty strings in PostgreSQL -- the handler-level check is necessary for full protection.

**Impact:** Without this fix, empty-name competitors can be created via the API, causing subtle UI and import issues. The fix is backward-compatible since existing data should all have names populated.

---

### 6. Hardcoded eventOrder for determining LastRegisteredEvent

**Severity:** Medium
**Area:** Logic
**File/Location:** `internal/service/competitors.go:52-61`

**Problem:**
`eventOrder` is a hardcoded slice: `["nat-2024", "glr-2025", "nat-2025", "glr-2026"]`. The `mostRecentEvent()` function uses this to determine which event is "newest" during BulkImport. When a new event is created (e.g., "nat-2026"), it won't appear in this list and will be treated as "after all known" events (line 70), which happens to sort correctly -- but only by accident. If two unknown events exist, they'll be compared as equal, and the first one in the slice wins arbitrarily.

**Proposed Change:**

Replace the hardcoded list with a database query. Use `Event.StartDate` as the canonical chronological ordering:

```go
func (s *CompetitorService) mostRecentEvent(events []string) (string, error) {
	if len(events) == 0 {
		return "", nil
	}
	var event db.Event
	err := s.db.Where("id IN ?", events).Order("start_date desc").First(&event).Error
	if err != nil {
		return events[len(events)-1], nil // fallback: last in list
	}
	return event.ID, nil
}
```

**Impact:** The current code happens to work when exactly one unknown event is present (the typical case), but will produce incorrect `LastRegisteredEvent` values if future imports reference multiple events not in the hardcoded list. The fix also removes a maintenance burden -- no one needs to remember to update this slice when creating new events.

---

### 7. Delete handler returns 204 for non-existent competitors

**Severity:** Low
**Area:** Logic
**File/Location:** `bin/api/handlers_competitors.go:217-241`, `internal/service/competitors.go:529-531`

**Problem:**
`CompetitorService.Delete` calls `s.db.Delete(&db.Competitor{}, "id = ?", id)` and returns the error. GORM does not return an error when the WHERE clause matches zero rows -- it returns `RowsAffected == 0` with `nil` error. The handler doesn't check `RowsAffected`, so it returns 204 and logs an audit entry for a competitor that didn't exist.

**Proposed Change:**

```go
func (s *CompetitorService) Delete(id string) error {
	result := s.db.Delete(&db.Competitor{}, "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
```

Then handle the 404 in the handler.

**Impact:** Low operational risk -- the phantom audit entry is misleading but harmless. The fix prevents ghost delete logs from cluttering the audit trail.

---

### 8. Missing index on Competitor.LastRegisteredEvent

**Severity:** Low
**Area:** Schema
**File/Location:** `internal/db/db.go:22`

**Problem:**
`GetAll` for non-admin users filters by `WHERE last_registered_event = ?` (competitors.go:349). This column has no index. With the current data volume (hundreds of competitors), this is fine. If the table grows to thousands of rows, this becomes a sequential scan on every search request.

**Proposed Change:**

```go
LastRegisteredEvent string `gorm:"index" json:"lastRegisteredEvent"`
```

**Impact:** No current performance problem, but proactively prevents a future bottleneck as the competitor table grows.

---

### 9. Re-fetch after upsert in CheckIn swallows errors

**Severity:** Low
**Area:** GORM
**File/Location:** `internal/service/competitors.go:446`

**Problem:**
After the upsert at line 438-443, the code re-fetches the CompetitorEvent at line 446:
```go
s.db.Where("competitor_id = ? AND event_id = ?", id, eventID).First(&ce)
```
The return value is discarded. If this query fails (unlikely but possible -- e.g., connection drop), the `ce` struct retains whatever state it had from the `Create` call, which may have an empty `ID` on the conflict/update path.

**Proposed Change:**
This is addressed by Finding 3's transaction fix, which properly handles the re-fetch error.

**Impact:** An empty `ce.ID` would be returned to the frontend. Low likelihood but easy to fix as part of the transaction work.

---

## No-Action Items

- **Audit logging outside transactions**: The wiki explicitly states "Audit writes are fire-and-forget -- a failed audit write is logged to stderr but never returns an error to the caller." This is an intentional design choice. The audit log is non-authoritative by design.
- **Non-admin access to POST/DELETE competitors**: The API spec explicitly marks these as "Required" (not "Admin"). Registration staff need to create and delete competitors as part of the check-in workflow.
- **Two-query pattern in GetAll/GetByID instead of JOINs or Preloads**: The manual two-query approach produces correct results and avoids GORM association complexity. It's a style choice, not a bug.
- **Event.StartDate/EndDate not NOT NULL**: Intentional -- BulkImport creates stub events with zero dates for events not yet in the system.
- **Authentication events not in audit log**: Out of scope. PIN attempts and token creation are tracked in dedicated tables (`pin_attempts`, `ip_blocklists`), which serve the security purpose. Audit log is for business operations.
- **CompetitorEvent.BeforeCreate hook generates UUID during upsert conflicts**: On the conflict path, the generated UUID is discarded since the existing row's ID is preserved. Wasteful but harmless -- one extra `uuid.New()` per check-in is negligible.
- **Backup tables from BulkImport accumulate**: These are never cleaned up, but they're small (competitor table snapshots) and serve as a manual safety net. Could be cleaned up periodically but not a correctness issue.

---

## Verdict

**Schema:** Minor Issues -- The main gap is missing FK constraints on `CompetitorEvent`, which allows orphaned rows. The missing NOT NULL on name fields and missing index on `LastRegisteredEvent` are minor.

**Application Logic:** Critical Issues -- The `Save()` misuse in Update can silently corrupt validation state. The lack of transactions in CheckIn and BulkImport can produce inconsistent state on partial failures. The hardcoded event order is fragile.

**Recommendation:** Fix findings 1-4 before the next event. Findings 1 (FK constraints) and 2 (Save -> Updates) are the highest priority -- they can corrupt data in normal operation, not just during failures.
