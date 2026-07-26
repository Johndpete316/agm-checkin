package service

import (
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	"johndpete316/agm-checkin-api/internal/db"
)

// newScheduleFixture reuses the competitor fixture's clean database so both
// suites share one TRUNCATE and one migration run, then hands back the schedule
// service under test alongside the raw handle for direct assertions.
func newScheduleFixture(t *testing.T) (*gorm.DB, *ScheduleService) {
	t.Helper()
	database, _ := newFixture(t)
	return database, NewScheduleService(database)
}

// scheduleSlot builds one fully-populated entry so tests can assert that fields
// they did not touch survived.
func scheduleSlot(competitorID, eventID string) db.CompetitorSchedule {
	return db.CompetitorSchedule{
		CompetitorID: competitorID,
		EventID:      eventID,
		Instrument:   "Piano",
		ScheduleDate: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
		ScheduleTime: "10:30 AM",
		Room:         "Room 1",
		Category:     "Concerto",
		Division:     "Senior",
		SortOrder:    630,
	}
}

func countSchedule(t *testing.T, database *gorm.DB, competitorID, eventID string) int64 {
	t.Helper()
	var n int64
	if err := database.Model(&db.CompetitorSchedule{}).
		Where("competitor_id = ? AND event_id = ?", competitorID, eventID).
		Count(&n).Error; err != nil {
		t.Fatalf("counting schedule rows: %v", err)
	}
	return n
}

// QA-SCH-01: a failed bulk insert used to leave the competitor with no schedule
// at all, because the delete that clears the old rows was committed on its own
// before the insert ran. The replacement has to be all-or-nothing.
func TestBulkUpsertRollsBackWhenInsertFails(t *testing.T) {
	database, svc := newScheduleFixture(t)
	seedEvent(t, database, "nat-2026", true)
	c := seedCompetitor(t, database, "Rollback", "Probe")

	existing := scheduleSlot(c.ID, "nat-2026")
	if err := database.Create(&existing).Error; err != nil {
		t.Fatalf("seeding the existing slot: %v", err)
	}

	// A check constraint is the most direct way to make the insert — and only
	// the insert — fail after the delete has already run.
	if err := database.Exec(
		`ALTER TABLE competitor_schedules
		 ADD CONSTRAINT qa_sch_01_reject_room CHECK (room <> 'REJECT')`,
	).Error; err != nil {
		t.Fatalf("adding the probe constraint: %v", err)
	}
	defer func() {
		if err := database.Exec(
			`ALTER TABLE competitor_schedules DROP CONSTRAINT qa_sch_01_reject_room`,
		).Error; err != nil {
			t.Fatalf("dropping the probe constraint: %v", err)
		}
	}()

	bad := scheduleSlot(c.ID, "nat-2026")
	bad.Room = "REJECT"

	if _, err := svc.BulkUpsert(c.ID, "nat-2026", []db.CompetitorSchedule{bad}); err == nil {
		t.Fatal("BulkUpsert succeeded despite the check constraint")
	}

	if got := countSchedule(t, database, c.ID, "nat-2026"); got != 1 {
		t.Fatalf("schedule rows after the failed import = %d, want 1 (the pre-existing slot)", got)
	}

	var kept db.CompetitorSchedule
	if err := database.First(&kept, "competitor_id = ?", c.ID).Error; err != nil {
		t.Fatalf("re-reading the surviving slot: %v", err)
	}
	if kept.ID != existing.ID {
		t.Fatalf("surviving row id = %q, want the pre-existing %q", kept.ID, existing.ID)
	}
}

// QA-SCH-01: the unbatched insert blew past the Postgres 65535-bind-parameter
// ceiling. Ten columns per row means anything over ~6500 rows failed outright.
func TestBulkUpsertHandlesBatchesOverTheParameterLimit(t *testing.T) {
	database, svc := newScheduleFixture(t)
	seedEvent(t, database, "nat-2026", true)
	c := seedCompetitor(t, database, "Large", "Batch")

	const rows = 10000
	entries := make([]db.CompetitorSchedule, rows)
	for i := range entries {
		entries[i] = scheduleSlot(c.ID, "nat-2026")
	}

	inserted, err := svc.BulkUpsert(c.ID, "nat-2026", entries)
	if err != nil {
		t.Fatalf("BulkUpsert of %d rows: %v", rows, err)
	}
	if inserted != rows {
		t.Fatalf("inserted = %d, want %d", inserted, rows)
	}
	if got := countSchedule(t, database, c.ID, "nat-2026"); got != rows {
		t.Fatalf("rows in the database = %d, want %d", got, rows)
	}
}

// Re-running an import must replace rather than accumulate — this is the one
// duplicate-protection the feature actually has, since idx_cs_competitor_event
// is not unique.
func TestBulkUpsertIsIdempotent(t *testing.T) {
	database, svc := newScheduleFixture(t)
	seedEvent(t, database, "nat-2026", true)
	c := seedCompetitor(t, database, "Repeat", "Import")

	entries := []db.CompetitorSchedule{scheduleSlot(c.ID, "nat-2026")}
	for round := 1; round <= 3; round++ {
		if _, err := svc.BulkUpsert(c.ID, "nat-2026", entries); err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		if got := countSchedule(t, database, c.ID, "nat-2026"); got != 1 {
			t.Fatalf("round %d: rows = %d, want 1", round, got)
		}
	}
}

// An empty entries list is a full wipe of that competitor's schedule for the
// event. Documented rather than prevented: the import tool relies on it.
func TestBulkUpsertWithNoEntriesClearsTheSchedule(t *testing.T) {
	database, svc := newScheduleFixture(t)
	seedEvent(t, database, "nat-2026", true)
	c := seedCompetitor(t, database, "Empty", "Import")

	entry := scheduleSlot(c.ID, "nat-2026")
	if err := database.Create(&entry).Error; err != nil {
		t.Fatalf("seeding: %v", err)
	}

	inserted, err := svc.BulkUpsert(c.ID, "nat-2026", nil)
	if err != nil {
		t.Fatalf("BulkUpsert with no entries: %v", err)
	}
	if inserted != 0 {
		t.Fatalf("inserted = %d, want 0", inserted)
	}
	if got := countSchedule(t, database, c.ID, "nat-2026"); got != 0 {
		t.Fatalf("rows = %d, want 0", got)
	}
}

// QA-SCH-02: Update wrote the whole row, so a caller changing one column
// silently blanked the rest and reset schedule_date to the zero time.
func TestUpdateLeavesUnsuppliedColumnsAlone(t *testing.T) {
	database, svc := newScheduleFixture(t)
	seedEvent(t, database, "nat-2026", true)
	c := seedCompetitor(t, database, "Partial", "Update")

	entry := scheduleSlot(c.ID, "nat-2026")
	if err := database.Create(&entry).Error; err != nil {
		t.Fatalf("seeding: %v", err)
	}

	updated, err := svc.Update(entry.ID, map[string]any{"room": "Room 2"})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if updated.Room != "Room 2" {
		t.Fatalf("room = %q, want %q", updated.Room, "Room 2")
	}
	if updated.Instrument != "Piano" {
		t.Errorf("instrument = %q, want it unchanged at %q", updated.Instrument, "Piano")
	}
	if updated.ScheduleTime != "10:30 AM" {
		t.Errorf("scheduleTime = %q, want it unchanged at %q", updated.ScheduleTime, "10:30 AM")
	}
	if updated.Category != "Concerto" {
		t.Errorf("category = %q, want it unchanged at %q", updated.Category, "Concerto")
	}
	if updated.Division != "Senior" {
		t.Errorf("division = %q, want it unchanged at %q", updated.Division, "Senior")
	}
	if updated.SortOrder != 630 {
		t.Errorf("sortOrder = %d, want it unchanged at 630", updated.SortOrder)
	}
	if !updated.ScheduleDate.Equal(entry.ScheduleDate) {
		t.Errorf("scheduleDate = %s, want it unchanged at %s", updated.ScheduleDate, entry.ScheduleDate)
	}
}

// An empty update is a no-op, not a wipe.
func TestUpdateWithNoColumnsChangesNothing(t *testing.T) {
	database, svc := newScheduleFixture(t)
	seedEvent(t, database, "nat-2026", true)
	c := seedCompetitor(t, database, "NoOp", "Update")

	entry := scheduleSlot(c.ID, "nat-2026")
	if err := database.Create(&entry).Error; err != nil {
		t.Fatalf("seeding: %v", err)
	}

	updated, err := svc.Update(entry.ID, map[string]any{})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Instrument != entry.Instrument || updated.ScheduleTime != entry.ScheduleTime ||
		updated.Room != entry.Room || updated.SortOrder != entry.SortOrder {
		t.Fatalf("empty update changed the row: %+v", *updated)
	}
	if !updated.ScheduleDate.Equal(entry.ScheduleDate) {
		t.Fatalf("empty update moved scheduleDate to %s", updated.ScheduleDate)
	}
}

// Ownership columns are not on the updatable list, so no caller can re-point a
// slot at another competitor or event by naming the column.
func TestUpdateRejectsOwnershipColumns(t *testing.T) {
	database, svc := newScheduleFixture(t)
	seedEvent(t, database, "nat-2026", true)
	seedEvent(t, database, "glr-2025", false)
	c := seedCompetitor(t, database, "Ownership", "Probe")
	other := seedCompetitor(t, database, "Other", "Competitor")

	entry := scheduleSlot(c.ID, "nat-2026")
	if err := database.Create(&entry).Error; err != nil {
		t.Fatalf("seeding: %v", err)
	}

	for _, column := range []string{"competitor_id", "event_id", "id"} {
		value := any(other.ID)
		if column == "event_id" {
			value = "glr-2025"
		}
		_, err := svc.Update(entry.ID, map[string]any{column: value})
		if !errors.Is(err, ErrScheduleColumnNotUpdatable) {
			t.Fatalf("Update(%s) error = %v, want ErrScheduleColumnNotUpdatable", column, err)
		}
	}

	var after db.CompetitorSchedule
	if err := database.First(&after, "id = ?", entry.ID).Error; err != nil {
		t.Fatalf("re-reading: %v", err)
	}
	if after.CompetitorID != c.ID || after.EventID != "nat-2026" {
		t.Fatalf("ownership changed to competitor %q event %q", after.CompetitorID, after.EventID)
	}
}

// Migration 001's fk_competitor_schedules_competitor is ON DELETE CASCADE, so
// deleting a competitor must take their schedule with them. Production still
// has no foreign keys, where the same delete orphans the rows instead.
func TestCompetitorDeleteCascadesToSchedule(t *testing.T) {
	database, _ := newScheduleFixture(t)
	seedEvent(t, database, "nat-2026", true)
	c := seedCompetitor(t, database, "Cascade", "Probe")

	entry := scheduleSlot(c.ID, "nat-2026")
	if err := database.Create(&entry).Error; err != nil {
		t.Fatalf("seeding: %v", err)
	}

	if err := database.Delete(&db.Competitor{}, "id = ?", c.ID).Error; err != nil {
		t.Fatalf("deleting the competitor: %v", err)
	}

	var n int64
	if err := database.Model(&db.CompetitorSchedule{}).
		Where("competitor_id = ?", c.ID).Count(&n).Error; err != nil {
		t.Fatalf("counting: %v", err)
	}
	if n != 0 {
		t.Fatalf("orphaned schedule rows = %d, want 0", n)
	}
}

// fk_competitor_schedules_event is ON DELETE RESTRICT: an event that anything
// still references cannot be deleted out from under it.
func TestEventDeleteIsRestrictedByScheduleRows(t *testing.T) {
	database, _ := newScheduleFixture(t)
	seedEvent(t, database, "nat-2026", true)
	c := seedCompetitor(t, database, "Restrict", "Probe")

	entry := scheduleSlot(c.ID, "nat-2026")
	if err := database.Create(&entry).Error; err != nil {
		t.Fatalf("seeding: %v", err)
	}

	err := database.Exec(`DELETE FROM events WHERE id = ?`, "nat-2026").Error
	if err == nil {
		t.Fatal("deleting an event with schedule rows succeeded; RESTRICT is not in force")
	}

	var n int64
	if err := database.Model(&db.Event{}).Where("id = ?", "nat-2026").Count(&n).Error; err != nil {
		t.Fatalf("counting events: %v", err)
	}
	if n != 1 {
		t.Fatalf("events remaining = %d, want 1", n)
	}
}
