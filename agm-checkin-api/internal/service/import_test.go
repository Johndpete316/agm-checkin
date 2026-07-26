package service

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"johndpete316/agm-checkin-api/internal/db"
)

// clearImportSnapshots drops every BulkImport snapshot table. They are named from
// time.Now().Unix() and survive the fixture TRUNCATE, so without this a second
// import inside the same wall-clock second collides with the first.
func clearImportSnapshots(t *testing.T, database *gorm.DB) {
	t.Helper()
	var names []string
	if err := database.Raw(
		`SELECT table_name FROM information_schema.tables
		 WHERE table_schema = 'public' AND table_name ~ '_backup_\d+$'`,
	).Scan(&names).Error; err != nil {
		t.Fatalf("listing snapshot tables: %v", err)
	}
	for _, n := range names {
		if err := database.Exec("DROP TABLE IF EXISTS " + n).Error; err != nil {
			t.Fatalf("dropping %s: %v", n, err)
		}
	}
}

func snapshotTables(t *testing.T, database *gorm.DB) []string {
	t.Helper()
	var names []string
	if err := database.Raw(
		`SELECT table_name FROM information_schema.tables
		 WHERE table_schema = 'public' AND table_name ~ '_backup_\d+$'
		 ORDER BY table_name`,
	).Scan(&names).Error; err != nil {
		t.Fatalf("listing snapshot tables: %v", err)
	}
	return names
}

func rowCounts(t *testing.T, database *gorm.DB) (competitors, events, competitorEvents int64) {
	t.Helper()
	database.Model(&db.Competitor{}).Count(&competitors)
	database.Model(&db.Event{}).Count(&events)
	database.Model(&db.CompetitorEvent{}).Count(&competitorEvents)
	return
}

// TestBulkImportRollsBackOnFailure is the transaction boundary. A row Postgres
// cannot store aborts the import, and nothing it had already written — including
// the stub events created before the competitor insert — may survive.
func TestBulkImportRollsBackOnFailure(t *testing.T) {
	database, svc := newFixture(t)
	clearImportSnapshots(t, database)

	rows := []ImportRow{
		{NameFirst: "Valid", NameLast: "Person", Events: []string{"brandnew-2099"}},
		// A NUL byte cannot be stored in a Postgres text column, so the
		// competitor insert fails after the stub event has been created.
		{NameFirst: "Bad\x00Person", NameLast: "Row", Events: []string{"brandnew-2099"}},
	}

	if _, err := svc.BulkImport(rows); err == nil {
		t.Fatal("BulkImport succeeded; want an error for the unstorable row")
	}

	competitors, events, ces := rowCounts(t, database)
	if competitors != 0 || events != 0 || ces != 0 {
		t.Fatalf("partial write: competitors=%d events=%d competitor_events=%d, want 0/0/0",
			competitors, events, ces)
	}
}

// TestBulkImportRollsBackAutoFillOnFailure covers the merge path: an existing
// competitor updated early in the import must not keep that update when a later
// row aborts the run.
func TestBulkImportRollsBackAutoFillOnFailure(t *testing.T) {
	database, svc := newFixture(t)
	clearImportSnapshots(t, database)
	seedEvent(t, database, "glr-2026", true)
	existing := seedCompetitor(t, database, "Merge", "Target")

	rows := []ImportRow{
		{NameFirst: "Merge", NameLast: "Target", Email: "filled@example.com", Studio: "Studio Z", Events: []string{"glr-2026"}},
		{NameFirst: "Bad\x00Person", NameLast: "Row", Events: []string{"glr-2026"}},
	}
	if _, err := svc.BulkImport(rows); err == nil {
		t.Fatal("BulkImport succeeded; want an error")
	}

	var after db.Competitor
	if err := database.First(&after, "id = ?", existing.ID).Error; err != nil {
		t.Fatalf("re-reading competitor: %v", err)
	}
	if after.Email != "" || after.Studio != "" {
		t.Fatalf("auto-filled fields survived a failed import: email=%q studio=%q", after.Email, after.Studio)
	}

	_, _, ces := rowCounts(t, database)
	if ces != 0 {
		t.Fatalf("competitor_events = %d after a failed import, want 0", ces)
	}
}

// TestBulkImportLargeRosterStaysUnderParameterLimit imports enough rows that a
// single unbatched INSERT would exceed Postgres' 65535 bind parameters. Before
// batching this failed with the competitors already committed and not one
// attendance row written.
func TestBulkImportLargeRosterStaysUnderParameterLimit(t *testing.T) {
	database, svc := newFixture(t)
	clearImportSnapshots(t, database)

	const people = 4000
	events := []string{"nat-2024", "glr-2025", "glr-2026"}

	rows := make([]ImportRow, 0, people)
	for i := 0; i < people; i++ {
		rows = append(rows, ImportRow{
			NameFirst: fmt.Sprintf("Bulk%04d", i),
			NameLast:  fmt.Sprintf("Row%04d", i),
			Events:    events,
		})
	}

	result, err := svc.BulkImport(rows)
	if err != nil {
		t.Fatalf("BulkImport: %v", err)
	}
	if result.CompetitorsCreated != people {
		t.Fatalf("competitorsCreated = %d, want %d", result.CompetitorsCreated, people)
	}

	wantCEs := int64(people * len(events))
	competitors, _, ces := rowCounts(t, database)
	if competitors != people {
		t.Fatalf("competitors = %d, want %d", competitors, people)
	}
	if ces != wantCEs {
		t.Fatalf("competitor_events = %d, want %d", ces, wantCEs)
	}
	if result.EventEntriesAdded != int(wantCEs) {
		t.Fatalf("eventEntriesAdded = %d, want %d", result.EventEntriesAdded, wantCEs)
	}
}

// TestBulkImportIsIdempotent re-imports the same file and asserts nothing
// duplicates: the same people, the same attendance rows.
func TestBulkImportIsIdempotent(t *testing.T) {
	database, svc := newFixture(t)
	clearImportSnapshots(t, database)

	dob := time.Date(2005, 3, 15, 0, 0, 0, 0, time.UTC)
	rows := []ImportRow{
		{NameFirst: "Idem", NameLast: "One", Email: "one@example.com", DateOfBirth: &dob,
			Validated: true, ShirtSize: "Adult L", Events: []string{"glr-2026", "nat-2025"}},
		{NameFirst: "Idem", NameLast: "Two", Events: []string{"glr-2026"}},
	}

	first, err := svc.BulkImport(rows)
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	if first.CompetitorsCreated != 2 || first.EventEntriesAdded != 3 {
		t.Fatalf("first import = %+v, want 2 created / 3 entries", first)
	}

	clearImportSnapshots(t, database)
	second, err := svc.BulkImport(rows)
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if second.CompetitorsCreated != 0 {
		t.Fatalf("second import created %d competitors, want 0", second.CompetitorsCreated)
	}
	if second.EventEntriesAdded != 0 {
		t.Fatalf("second import added %d attendance rows, want 0", second.EventEntriesAdded)
	}
	if len(second.FieldConflicts) != 0 {
		t.Fatalf("second import reported conflicts against itself: %+v", second.FieldConflicts)
	}

	competitors, _, ces := rowCounts(t, database)
	if competitors != 2 || ces != 3 {
		t.Fatalf("after re-import competitors=%d competitor_events=%d, want 2/3", competitors, ces)
	}
}

// TestBulkImportDuplicateRowsInOneFile documents an unfixed defect: two identical
// rows in one file become two competitors, and every later import of that file
// then refuses to touch either of them.
func TestBulkImportDuplicateRowsInOneFile(t *testing.T) {
	database, svc := newFixture(t)
	clearImportSnapshots(t, database)

	rows := []ImportRow{
		{NameFirst: "Twice", NameLast: "Listed", Email: "a@example.com", Events: []string{"glr-2026"}},
		{NameFirst: "Twice", NameLast: "Listed", Email: "b@example.com", Events: []string{"glr-2026"}},
	}
	if _, err := svc.BulkImport(rows); err != nil {
		t.Fatalf("import: %v", err)
	}

	competitors, _, ces := rowCounts(t, database)
	if competitors != 2 || ces != 2 {
		t.Fatalf("competitors=%d competitor_events=%d, want the current 2/2 behaviour", competitors, ces)
	}

	// And the pair is now permanently unimportable: the name is ambiguous forever,
	// so no later file can ever add them to a roster again.
	clearImportSnapshots(t, database)
	second, err := svc.BulkImport(rows)
	if err != nil {
		t.Fatalf("re-import: %v", err)
	}
	if len(second.Errors) != 2 {
		t.Fatalf("re-import errors = %v, want two skip messages", second.Errors)
	}
	for _, e := range second.Errors {
		if !strings.Contains(e, "resolve manually") {
			t.Fatalf("unexpected error %q", e)
		}
	}
	t.Log("KNOWN DEFECT: one person listed twice in a file becomes two competitors, " +
		"and every later import of that file refuses to touch either of them")
}

// TestBulkImportTwoPeopleSameName covers the genuine-collision case the importer
// warns about: two different people with the same name can never be imported.
func TestBulkImportTwoPeopleSameName(t *testing.T) {
	database, svc := newFixture(t)
	clearImportSnapshots(t, database)
	seedEvent(t, database, "glr-2026", true)

	// Two real, distinct people who happen to share a name.
	a := db.Competitor{NameFirst: "Same", NameLast: "Name", Email: "person-a@example.com"}
	b := db.Competitor{NameFirst: "Same", NameLast: "Name", Email: "person-b@example.com"}
	if err := database.Create(&a).Error; err != nil {
		t.Fatalf("seeding a: %v", err)
	}
	if err := database.Create(&b).Error; err != nil {
		t.Fatalf("seeding b: %v", err)
	}

	result, err := svc.BulkImport([]ImportRow{
		{NameFirst: "Same", NameLast: "Name", Email: "person-a@example.com", Events: []string{"glr-2026"}},
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if result.EventEntriesAdded != 0 {
		t.Fatalf("eventEntriesAdded = %d, want 0 — the row is skipped", result.EventEntriesAdded)
	}
	if len(result.Errors) != 1 || !strings.Contains(result.Errors[0], "resolve manually") {
		t.Fatalf("errors = %v, want one 'resolve manually' entry", result.Errors)
	}

	// The failure that matters: neither person is on the roster, and the only
	// signal is a line in an errors array the UI may not surface.
	_, _, ces := rowCounts(t, database)
	if ces != 0 {
		t.Fatalf("competitor_events = %d, want 0", ces)
	}
	t.Logf("both same-name competitors were left off the roster with only: %s", result.Errors[0])
}

// TestBulkImportRenamedPersonDuplicates shows that the match key is the name
// alone: a competitor whose surname changed is imported as a second person even
// when the email is identical.
func TestBulkImportRenamedPersonDuplicates(t *testing.T) {
	database, svc := newFixture(t)
	clearImportSnapshots(t, database)

	if _, err := svc.BulkImport([]ImportRow{
		{NameFirst: "Rename", NameLast: "Before", Email: "stable@example.com", Events: []string{"glr-2026"}},
	}); err != nil {
		t.Fatalf("first import: %v", err)
	}

	clearImportSnapshots(t, database)
	if _, err := svc.BulkImport([]ImportRow{
		{NameFirst: "Rename", NameLast: "After", Email: "stable@example.com", Events: []string{"glr-2026"}},
	}); err != nil {
		t.Fatalf("second import: %v", err)
	}

	competitors, _, _ := rowCounts(t, database)
	if competitors != 2 {
		t.Fatalf("competitors = %d, want 2 — matching is by name only", competitors)
	}
	t.Log("a surname change produces a second competitor record even with an identical email")
}

// TestBulkImportSnapshotIsPreWriteAndComplete proves the snapshot is taken before
// any row is written and holds the full prior state, so it is genuinely restorable.
func TestBulkImportSnapshotIsPreWriteAndComplete(t *testing.T) {
	database, svc := newFixture(t)
	clearImportSnapshots(t, database)
	seedEvent(t, database, "glr-2026", true)

	before := seedCompetitor(t, database, "Pre", "Existing")
	if err := database.Create(&db.CompetitorEvent{
		CompetitorID: before.ID, EventID: "glr-2026", CheckedIn: true,
	}).Error; err != nil {
		t.Fatalf("seeding attendance: %v", err)
	}

	if _, err := svc.BulkImport([]ImportRow{
		{NameFirst: "Newly", NameLast: "Imported", Events: []string{"glr-2026"}},
	}); err != nil {
		t.Fatalf("import: %v", err)
	}

	names := snapshotTables(t, database)
	if len(names) != 2 {
		t.Fatalf("snapshot tables = %v, want one competitors/competitor_events pair", names)
	}

	var snapCompetitors, snapCEs int64
	for _, n := range names {
		var count int64
		if err := database.Raw("SELECT count(*) FROM " + n).Scan(&count).Error; err != nil {
			t.Fatalf("counting %s: %v", n, err)
		}
		if strings.HasPrefix(n, "competitors_backup_") {
			snapCompetitors = count
		} else {
			snapCEs = count
		}
	}
	if snapCompetitors != 1 || snapCEs != 1 {
		t.Fatalf("snapshot holds %d competitors / %d attendance rows, want the 1/1 pre-import state",
			snapCompetitors, snapCEs)
	}

	// And it restores: the rollback documented in IMPORT.md, run verbatim and in
	// the documented order, returns the database to that state.
	var suffix string
	for _, n := range names {
		if strings.HasPrefix(n, "competitors_backup_") {
			suffix = strings.TrimPrefix(n, "competitors_backup_")
		}
	}
	for _, stmt := range []string{
		"TRUNCATE competitors CASCADE",
		"INSERT INTO competitors SELECT * FROM competitors_backup_" + suffix,
		"INSERT INTO competitor_events SELECT * FROM competitor_events_backup_" + suffix,
	} {
		if err := database.Exec(stmt).Error; err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	competitors, _, ces := rowCounts(t, database)
	if competitors != 1 || ces != 1 {
		t.Fatalf("after restore competitors=%d competitor_events=%d, want 1/1", competitors, ces)
	}
}

// TestBulkImportSnapshotOmitsSchedules is the hole in the safety net: the
// documented rollback truncates competitors with CASCADE, which takes
// competitor_schedules with it, and no snapshot of that table is ever taken.
func TestBulkImportSnapshotOmitsSchedules(t *testing.T) {
	database, svc := newFixture(t)
	clearImportSnapshots(t, database)
	seedEvent(t, database, "glr-2026", true)

	competitor := seedCompetitor(t, database, "Has", "Schedule")
	if err := database.Create(&db.CompetitorSchedule{
		CompetitorID: competitor.ID,
		EventID:      "glr-2026",
		Instrument:   "Piano",
		ScheduleDate: time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC),
		ScheduleTime: "09:00",
		Category:     "Solo",
		Division:     "Junior",
	}).Error; err != nil {
		t.Fatalf("seeding schedule: %v", err)
	}

	if _, err := svc.BulkImport([]ImportRow{
		{NameFirst: "Some", NameLast: "Import", Events: []string{"glr-2026"}},
	}); err != nil {
		t.Fatalf("import: %v", err)
	}

	for _, n := range snapshotTables(t, database) {
		if strings.HasPrefix(n, "competitor_schedules_backup_") {
			t.Fatalf("unexpected schedule snapshot %s — this test is stale", n)
		}
	}

	// Now run the documented rollback and count what is left.
	if err := database.Exec("TRUNCATE competitors CASCADE").Error; err != nil {
		t.Fatalf("truncating: %v", err)
	}
	var schedules int64
	database.Model(&db.CompetitorSchedule{}).Count(&schedules)
	if schedules != 0 {
		t.Fatalf("competitor_schedules = %d after TRUNCATE competitors CASCADE, want 0", schedules)
	}
	t.Log("KNOWN DEFECT: the documented rollback destroys competitor_schedules, " +
		"which BulkImport never snapshots — that data is unrecoverable")
}

// TestBulkImportSnapshotRetention imports repeatedly and asserts snapshot tables
// do not accumulate without bound.
func TestBulkImportSnapshotRetention(t *testing.T) {
	database, svc := newFixture(t)
	clearImportSnapshots(t, database)

	const imports = backupRetention + 3
	for i := 0; i < imports; i++ {
		if _, err := svc.BulkImport([]ImportRow{
			{NameFirst: fmt.Sprintf("Retain%d", i), NameLast: "Row", Events: []string{"glr-2026"}},
		}); err != nil {
			t.Fatalf("import %d: %v", i, err)
		}
		// Snapshot names have one-second resolution.
		time.Sleep(1050 * time.Millisecond)
	}

	names := snapshotTables(t, database)
	if len(names) != backupRetention*2 {
		t.Fatalf("snapshot tables after %d imports = %d (%v), want %d",
			imports, len(names), names, backupRetention*2)
	}
}

// TestBulkImportSnapshotOrphanHalfIsNeverPruned documents that retention keys off
// the competitors_backup_ tables only, so a competitor_events_backup_ table whose
// partner is gone is kept forever.
func TestBulkImportSnapshotOrphanHalfIsNeverPruned(t *testing.T) {
	database, svc := newFixture(t)
	clearImportSnapshots(t, database)

	if err := database.Exec(
		"CREATE TABLE competitor_events_backup_1 AS SELECT * FROM competitor_events",
	).Error; err != nil {
		t.Fatalf("creating orphan snapshot half: %v", err)
	}
	t.Cleanup(func() { database.Exec("DROP TABLE IF EXISTS competitor_events_backup_1") })

	for i := 0; i < backupRetention+2; i++ {
		if _, err := svc.BulkImport([]ImportRow{
			{NameFirst: fmt.Sprintf("Orphan%d", i), NameLast: "Row", Events: []string{"glr-2026"}},
		}); err != nil {
			t.Fatalf("import %d: %v", i, err)
		}
		time.Sleep(1050 * time.Millisecond)
	}

	for _, n := range snapshotTables(t, database) {
		if n == "competitor_events_backup_1" {
			t.Log("KNOWN DEFECT: an orphaned snapshot half survives every prune")
			return
		}
	}
	t.Fatal("expected the orphaned snapshot half to still be present")
}

// TestBulkImportTwiceInOneSecondCollides shows the snapshot name is only
// second-resolution, so a second import inside the same second is refused
// outright. It fails closed — nothing is written — but the operation is not
// retryable for up to a second, and the error names an internal table.
func TestBulkImportTwiceInOneSecondCollides(t *testing.T) {
	database, svc := newFixture(t)
	clearImportSnapshots(t, database)

	if _, err := svc.BulkImport([]ImportRow{
		{NameFirst: "First", NameLast: "Go", Events: []string{"glr-2026"}},
	}); err != nil {
		t.Fatalf("first import: %v", err)
	}
	_, err := svc.BulkImport([]ImportRow{
		{NameFirst: "Second", NameLast: "Go", Events: []string{"glr-2026"}},
	})
	if err == nil {
		t.Skip("the two imports did not land in the same second")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("error = %v, want a duplicate snapshot table error", err)
	}

	competitors, _, _ := rowCounts(t, database)
	if competitors != 1 {
		t.Fatalf("competitors = %d, want 1 — the refused import must not write", competitors)
	}
	t.Logf("KNOWN DEFECT: a second import inside the same second fails with %v", err)
}

// TestBulkImportLeavesNoCurrentEvent covers what happens with no current event
// set: the import succeeds, creates the roster, and the whole roster is invisible
// to every reader because no event was ever marked current.
func TestBulkImportLeavesNoCurrentEvent(t *testing.T) {
	database, svc := newFixture(t)
	clearImportSnapshots(t, database)

	result, err := svc.BulkImport([]ImportRow{
		{NameFirst: "Invisible", NameLast: "Entrant", Events: []string{"glr-2026"}},
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if result.EventsCreated != 1 || result.EventEntriesAdded != 1 {
		t.Fatalf("result = %+v, want one event and one entry", result)
	}

	var stub db.Event
	if err := database.First(&stub, "id = ?", "glr-2026").Error; err != nil {
		t.Fatalf("reading stub event: %v", err)
	}
	if stub.IsCurrent {
		t.Fatal("the stub event was marked current")
	}
	if !stub.StartDate.IsZero() && stub.StartDate.Year() > 1 {
		t.Logf("stub start date = %v", stub.StartDate)
	}

	admin, err := svc.GetAll("", true, EventScopeCurrent)
	if err != nil {
		t.Fatalf("GetAll admin: %v", err)
	}
	registration, err := svc.GetAll("", false, "")
	if err != nil {
		t.Fatalf("GetAll registration: %v", err)
	}
	if len(admin) != 0 || len(registration) != 0 {
		t.Fatalf("admin=%d registration=%d, want 0/0", len(admin), len(registration))
	}
	t.Log("imported competitors are invisible until an admin sets a current event by hand")
}

// TestBulkImportRowWithNoEventsIsOrphaned covers a row whose events column is
// empty: the competitor is created but joins no roster, with no warning.
func TestBulkImportRowWithNoEventsIsOrphaned(t *testing.T) {
	database, svc := newFixture(t)
	clearImportSnapshots(t, database)

	result, err := svc.BulkImport([]ImportRow{{NameFirst: "No", NameLast: "Roster"}})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if result.CompetitorsCreated != 1 {
		t.Fatalf("competitorsCreated = %d, want 1", result.CompetitorsCreated)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("errors = %v, want none reported", result.Errors)
	}

	_, _, ces := rowCounts(t, database)
	if ces != 0 {
		t.Fatalf("competitor_events = %d, want 0", ces)
	}
	t.Log("a row with an empty events column creates a competitor on no roster and reports success")
}

// TestBulkImportUnknownEventIDCreatesStub confirms an events value naming an
// event that does not exist silently invents it rather than rejecting the file.
func TestBulkImportUnknownEventIDCreatesStub(t *testing.T) {
	database, svc := newFixture(t)
	clearImportSnapshots(t, database)

	result, err := svc.BulkImport([]ImportRow{
		{NameFirst: "Typo", NameLast: "Victim", Events: []string{"glr-20266"}},
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if result.EventsCreated != 1 {
		t.Fatalf("eventsCreated = %d, want 1", result.EventsCreated)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("errors = %v, want none — a typo'd event ID is accepted", result.Errors)
	}

	var stub db.Event
	if err := database.First(&stub, "id = ?", "glr-20266").Error; err != nil {
		t.Fatalf("reading stub: %v", err)
	}
	t.Logf("a mistyped event ID silently created event %q named %q", stub.ID, stub.Name)
}
