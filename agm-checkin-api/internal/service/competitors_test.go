package service

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"johndpete316/agm-checkin-api/internal/db"
)

// newFixture returns a clean database and service. It runs AutoMigrate followed
// by the ordered migrations, so every test run also exercises the migration
// runner itself. Set TEST_DATABASE_URL to a scratch database — never point it at
// a database you care about, since each test truncates.
func newFixture(t *testing.T) (*gorm.DB, *CompetitorService) {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database-backed tests")
	}

	database, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("connecting to test database: %v", err)
	}

	db.AutoMigrate(database)
	if err := db.Migrate(database); err != nil {
		t.Fatalf("running migrations: %v", err)
	}

	if err := database.Exec(
		`TRUNCATE competitors, competitor_events, competitor_schedules, events RESTART IDENTITY CASCADE`,
	).Error; err != nil {
		t.Fatalf("truncating: %v", err)
	}

	return database, NewCompetitorService(database)
}

func seedEvent(t *testing.T, database *gorm.DB, id string, current bool) {
	t.Helper()
	event := db.Event{ID: id, Name: id, IsCurrent: current, StartDate: time.Now(), EndDate: time.Now()}
	if err := database.Create(&event).Error; err != nil {
		t.Fatalf("seeding event %s: %v", id, err)
	}
}

func seedEventOn(t *testing.T, database *gorm.DB, id string, current bool, start time.Time) {
	t.Helper()
	event := db.Event{ID: id, Name: id, IsCurrent: current, StartDate: start, EndDate: start}
	if err := database.Create(&event).Error; err != nil {
		t.Fatalf("seeding event %s: %v", id, err)
	}
}

func seedCompetitor(t *testing.T, database *gorm.DB, first, last string) db.Competitor {
	t.Helper()
	c := db.Competitor{NameFirst: first, NameLast: last}
	if err := database.Create(&c).Error; err != nil {
		t.Fatalf("seeding competitor %s %s: %v", first, last, err)
	}
	return c
}

// register adds the competitor to an event's roster without checking them in.
func register(t *testing.T, database *gorm.DB, competitorID, eventID string) {
	t.Helper()
	ce := db.CompetitorEvent{CompetitorID: competitorID, EventID: eventID}
	if err := database.Create(&ce).Error; err != nil {
		t.Fatalf("registering %s for %s: %v", competitorID, eventID, err)
	}
}

func names(list []CompetitorWithCheckIn) map[string]bool {
	out := map[string]bool{}
	for _, c := range list {
		out[c.NameLast] = true
	}
	return out
}

// Registration staff see the current event's roster; admins see everyone.
func TestGetAllScopesRegistrationUsersToCurrentEvent(t *testing.T) {
	database, svc := newFixture(t)

	seedEvent(t, database, "glr-2026", false)
	seedEvent(t, database, "nat-2026", true)

	onRoster := seedCompetitor(t, database, "Ada", "Lovelace")
	pastOnly := seedCompetitor(t, database, "Grace", "Hopper")
	register(t, database, onRoster.ID, "nat-2026")
	register(t, database, pastOnly.ID, "glr-2026")

	got, err := svc.GetAll("", false, "")
	if err != nil {
		t.Fatalf("GetAll(registration): %v", err)
	}
	found := names(got)
	if !found["Lovelace"] || found["Hopper"] {
		t.Errorf("registration user should see only the current roster, got %v", found)
	}

	got, err = svc.GetAll("", true, "")
	if err != nil {
		t.Fatalf("GetAll(admin): %v", err)
	}
	found = names(got)
	if !found["Lovelace"] || !found["Hopper"] {
		t.Errorf("admin should see every competitor, got %v", found)
	}
}

// The admin event selector scopes the query server-side so the page loads one
// roster instead of every competitor ever imported.
func TestGetAllScopesAdminsToTheRequestedEvent(t *testing.T) {
	database, svc := newFixture(t)

	seedEventOn(t, database, "glr-2026", false, time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC))
	seedEventOn(t, database, "nat-2026", true, time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC))

	returning := seedCompetitor(t, database, "Ada", "Lovelace")
	pastOnly := seedCompetitor(t, database, "Grace", "Hopper")
	currentOnly := seedCompetitor(t, database, "Alan", "Turing")
	register(t, database, returning.ID, "glr-2026")
	register(t, database, returning.ID, "nat-2026")
	register(t, database, pastOnly.ID, "glr-2026")
	register(t, database, currentOnly.ID, "nat-2026")

	// A past event returns its whole roster, including someone who has since
	// competed again. Filtering on most-recent-event would have dropped Lovelace.
	got, err := svc.GetAll("", true, "glr-2026")
	if err != nil {
		t.Fatalf("GetAll(admin, glr-2026): %v", err)
	}
	found := names(got)
	if !found["Lovelace"] || !found["Hopper"] || found["Turing"] {
		t.Errorf("glr-2026 roster = %v, want Lovelace and Hopper only", found)
	}

	// The "current" sentinel resolves without the caller knowing the event ID.
	got, err = svc.GetAll("", true, EventScopeCurrent)
	if err != nil {
		t.Fatalf("GetAll(admin, current): %v", err)
	}
	found = names(got)
	if !found["Lovelace"] || !found["Turing"] || found["Hopper"] {
		t.Errorf("current roster = %v, want Lovelace and Turing only", found)
	}

	got, err = svc.GetAll("", true, EventScopeAll)
	if err != nil {
		t.Fatalf("GetAll(admin, all): %v", err)
	}
	if len(names(got)) != 3 {
		t.Errorf("EventScopeAll should return every competitor, got %v", names(got))
	}
}

// Scope is an admin affordance. A registration user asking for a past roster
// must still get the current one rather than a back door into history.
func TestGetAllIgnoresEventScopeForRegistrationUsers(t *testing.T) {
	database, svc := newFixture(t)

	seedEvent(t, database, "glr-2026", false)
	seedEvent(t, database, "nat-2026", true)

	pastOnly := seedCompetitor(t, database, "Grace", "Hopper")
	currentOnly := seedCompetitor(t, database, "Alan", "Turing")
	register(t, database, pastOnly.ID, "glr-2026")
	register(t, database, currentOnly.ID, "nat-2026")

	for _, scope := range []string{"glr-2026", EventScopeAll} {
		got, err := svc.GetAll("", false, scope)
		if err != nil {
			t.Fatalf("GetAll(registration, %q): %v", scope, err)
		}
		found := names(got)
		if found["Hopper"] || !found["Turing"] {
			t.Errorf("scope %q leaked past the registration filter: %v", scope, found)
		}
	}
}

// The reason for the whole conversion: last_registered_event was one string, so
// it could not say "registered for glr-2026 and nat-2026". Under the old filter a
// returning competitor was invisible to registration staff until an import
// rewrote that column, which is what made opening a new event manual work.
func TestReturningCompetitorIsVisibleForEveryEventTheyAreOn(t *testing.T) {
	database, svc := newFixture(t)

	seedEventOn(t, database, "glr-2026", false, time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC))
	seedEventOn(t, database, "nat-2026", true, time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC))

	returning := seedCompetitor(t, database, "Ada", "Lovelace")
	register(t, database, returning.ID, "glr-2026")
	register(t, database, returning.ID, "nat-2026")

	got, err := svc.GetAll("", false, "")
	if err != nil {
		t.Fatalf("GetAll(registration): %v", err)
	}
	if !names(got)["Lovelace"] {
		t.Fatal("returning competitor is on the current roster but not visible to registration staff")
	}
	if got[0].MostRecentEvent != "nat-2026" {
		t.Errorf("expected mostRecentEvent nat-2026, got %q", got[0].MostRecentEvent)
	}
}

// Derived from event start dates, not from a hardcoded list of event slugs.
func TestMostRecentEventPrefersDatedEvents(t *testing.T) {
	database, svc := newFixture(t)

	seedEventOn(t, database, "nat-2026", true, time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC))
	// A stub imported without dates, exactly as BulkImport creates them.
	seedEventOn(t, database, "zzz-legacy", false, time.Time{})

	c := seedCompetitor(t, database, "Grace", "Hopper")
	register(t, database, c.ID, "zzz-legacy")
	register(t, database, c.ID, "nat-2026")

	got, err := svc.GetByID(c.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.MostRecentEvent != "nat-2026" {
		t.Errorf("undated stub should lose to a dated event, got %q", got.MostRecentEvent)
	}
}

// Adding a competitor has to put them on a roster, or the visibility filter
// hides them from the staff member who just created them.
func TestCreateRegistersForSelectedEvent(t *testing.T) {
	database, svc := newFixture(t)
	seedEvent(t, database, "nat-2026", true)

	c := db.Competitor{NameFirst: "Ada", NameLast: "Lovelace", RegisterForEvent: "nat-2026"}
	if err := svc.Create(&c, "Alice Admin"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := svc.GetAll("", false, "")
	if err != nil {
		t.Fatalf("GetAll(registration): %v", err)
	}
	if !names(got)["Lovelace"] {
		t.Error("newly created competitor is not visible to registration staff")
	}

	orphan := db.Competitor{NameFirst: "Grace", NameLast: "Hopper", RegisterForEvent: "no-such-event"}
	if err := svc.Create(&orphan, "Alice Admin"); !errors.Is(err, ErrUnknownEvent) {
		t.Errorf("expected ErrUnknownEvent, got %v", err)
	}
	var count int64
	database.Model(&db.Competitor{}).Where("name_last = ?", "Hopper").Count(&count)
	if count != 0 {
		t.Error("failed registration should roll back the competitor row")
	}
}

func TestCheckInIsIdempotent(t *testing.T) {
	database, svc := newFixture(t)

	seedEvent(t, database, "nat-2026", true)
	c := seedCompetitor(t, database, "Ada", "Lovelace")
	register(t, database, c.ID, "nat-2026")

	first, err := svc.CheckIn(c.ID, "Staff Member")
	if err != nil {
		t.Fatalf("first CheckIn: %v", err)
	}
	if first.CurrentCheckIn == nil || !first.CurrentCheckIn.CheckedIn {
		t.Fatal("expected competitor to be checked in")
	}
	if first.CurrentCheckIn.CheckedInBy != "Staff Member" {
		t.Errorf("CheckedInBy = %q, want %q", first.CurrentCheckIn.CheckedInBy, "Staff Member")
	}
	if first.CurrentCheckIn.CheckInDatetime == nil {
		t.Error("expected a check-in timestamp")
	}

	// A second desk working from a stale roster re-checks the same competitor in.
	second, err := svc.CheckIn(c.ID, "Other Staff")
	if err != nil {
		t.Fatalf("second CheckIn: %v", err)
	}
	if second.CurrentCheckIn.ID != first.CurrentCheckIn.ID {
		t.Errorf("check-in created a second row: %s then %s",
			first.CurrentCheckIn.ID, second.CurrentCheckIn.ID)
	}

	// The duplicate must not rewrite who checked them in, or when.
	if second.CurrentCheckIn.CheckedInBy != "Staff Member" {
		t.Errorf("duplicate check-in overwrote attribution: CheckedInBy = %q, want %q",
			second.CurrentCheckIn.CheckedInBy, "Staff Member")
	}
	if !second.CurrentCheckIn.CheckInDatetime.Equal(*first.CurrentCheckIn.CheckInDatetime) {
		t.Errorf("duplicate check-in overwrote the timestamp: %v then %v",
			first.CurrentCheckIn.CheckInDatetime, second.CurrentCheckIn.CheckInDatetime)
	}

	var rows int64
	database.Model(&db.CompetitorEvent{}).Where("competitor_id = ?", c.ID).Count(&rows)
	if rows != 1 {
		t.Errorf("expected exactly 1 attendance row, got %d", rows)
	}
}

// A competitor already on the roster but not yet checked in must still be
// updated by the conflict path — the guard applies only to real check-ins.
func TestCheckInUpdatesAnUncheckedRosterRow(t *testing.T) {
	database, svc := newFixture(t)

	seedEvent(t, database, "nat-2026", true)
	c := seedCompetitor(t, database, "Grace", "Hopper")
	register(t, database, c.ID, "nat-2026")

	result, err := svc.CheckIn(c.ID, "Staff Member")
	if err != nil {
		t.Fatalf("CheckIn: %v", err)
	}
	if !result.CurrentCheckIn.CheckedIn {
		t.Error("expected the existing roster row to be marked checked in")
	}
	if result.CurrentCheckIn.CheckedInBy != "Staff Member" {
		t.Errorf("CheckedInBy = %q, want %q", result.CurrentCheckIn.CheckedInBy, "Staff Member")
	}
	if result.CurrentCheckIn.CheckInDatetime == nil {
		t.Error("expected a check-in timestamp on the roster row")
	}

	var rows int64
	database.Model(&db.CompetitorEvent{}).Where("competitor_id = ?", c.ID).Count(&rows)
	if rows != 1 {
		t.Errorf("expected exactly 1 attendance row, got %d", rows)
	}
}

func TestCheckInWithoutCurrentEvent(t *testing.T) {
	database, svc := newFixture(t)

	seedEvent(t, database, "nat-2026", false)
	c := seedCompetitor(t, database, "Ada", "Lovelace")

	if _, err := svc.CheckIn(c.ID, "Staff Member"); !errors.Is(err, ErrNoCurrentEvent) {
		t.Errorf("expected ErrNoCurrentEvent, got %v", err)
	}
}

// Verification is permanent and records who did it. Re-verifying must not
// re-stamp the original provenance, or the audit trail silently reattributes
// the check to whoever touched the record last.
func TestValidateIsIdempotentAndRecordsProvenance(t *testing.T) {
	database, svc := newFixture(t)

	c := seedCompetitor(t, database, "Ada", "Lovelace")

	first, err := svc.Validate(c.ID, "Alice Admin")
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if first.DobVerifiedAt == nil {
		t.Fatal("expected dobVerifiedAt to be set")
	}
	if first.DobVerifiedBy != "Alice Admin" {
		t.Errorf("expected verifier Alice Admin, got %q", first.DobVerifiedBy)
	}
	second, err := svc.Validate(c.ID, "Bob Staff")
	if err != nil {
		t.Fatalf("re-Validate: %v", err)
	}
	if !second.DobVerifiedAt.Equal(*first.DobVerifiedAt) {
		t.Errorf("timestamp was re-stamped: %v then %v", first.DobVerifiedAt, second.DobVerifiedAt)
	}
	if second.DobVerifiedBy != "Alice Admin" {
		t.Errorf("verifier was overwritten, got %q", second.DobVerifiedBy)
	}
}

// fields builds the key set a PATCH body would have carried.
func fields(keys ...string) map[string]bool {
	out := make(map[string]bool, len(keys))
	for _, k := range keys {
		out[k] = true
	}
	return out
}

// The client may say whether a competitor is verified, never when or by whom.
func TestUpdateDoesNotLetClientsForgeVerificationProvenance(t *testing.T) {
	database, svc := newFixture(t)

	c := seedCompetitor(t, database, "Grace", "Hopper")

	forged := time.Date(1999, 1, 1, 0, 0, 0, 0, time.UTC)
	got, err := svc.Update(c.ID, db.Competitor{
		NameFirst:     "Grace",
		NameLast:      "Hopper",
		DobVerifiedAt: &forged,
		DobVerifiedBy: "Somebody Else",
	}, fields("nameFirst", "nameLast", "dobVerifiedAt", "dobVerifiedBy"), "Alice Admin")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.DobVerifiedAt.Equal(forged) {
		t.Error("client-supplied verification timestamp was persisted")
	}
	if got.DobVerifiedBy != "Alice Admin" {
		t.Errorf("expected verifier Alice Admin, got %q", got.DobVerifiedBy)
	}

	// Revoking is an explicit null, not an omission — the caller has to name the
	// field to clear it.
	cleared, err := svc.Update(c.ID, db.Competitor{NameFirst: "Grace", NameLast: "Hopper"},
		fields("nameFirst", "nameLast", "dobVerifiedAt"), "Alice Admin")
	if err != nil {
		t.Fatalf("clearing Update: %v", err)
	}
	if cleared.DobVerifiedAt != nil {
		t.Error("expected admin to be able to revoke verification")
	}
}

// A PATCH carrying one field must write one field. Save() wrote the whole
// struct, so a body of {"nameFirst": ...} silently blanked the surname, the date
// of birth, the shirt size, the email, the studio, the teacher, the staff note,
// and the verification stamp — every one of which is re-collected only by
// asking the competitor again at the desk.
func TestUpdateLeavesOmittedFieldsAlone(t *testing.T) {
	database, svc := newFixture(t)

	dob := time.Date(2005, 3, 15, 0, 0, 0, 0, time.UTC)
	c := db.Competitor{
		NameFirst:   "Ada",
		NameLast:    "Lovelace",
		DateOfBirth: dob,
		ShirtSize:   "M",
		Email:       "ada@example.com",
		Teacher:     "T One",
		Studio:      "S One",
		Note:        "needs step-free access",
	}
	if err := database.Create(&c).Error; err != nil {
		t.Fatalf("seeding competitor: %v", err)
	}
	verified, err := svc.Validate(c.ID, "Alice Admin")
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}

	got, err := svc.Update(c.ID, db.Competitor{NameFirst: "Adaline"}, fields("nameFirst"), "Bob Staff")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if got.NameFirst != "Adaline" {
		t.Errorf("NameFirst = %q, want Adaline", got.NameFirst)
	}
	for _, f := range []struct{ name, got, want string }{
		{"NameLast", got.NameLast, "Lovelace"},
		{"ShirtSize", got.ShirtSize, "M"},
		{"Email", got.Email, "ada@example.com"},
		{"Teacher", got.Teacher, "T One"},
		{"Studio", got.Studio, "S One"},
		{"Note", got.Note, "needs step-free access"},
		{"DobVerifiedBy", got.DobVerifiedBy, "Alice Admin"},
	} {
		if f.got != f.want {
			t.Errorf("%s = %q, want %q — omitted field was overwritten", f.name, f.got, f.want)
		}
	}
	if !got.DateOfBirth.Equal(dob) {
		t.Errorf("DateOfBirth = %v, want %v — omitted field was overwritten", got.DateOfBirth, dob)
	}
	if got.DobVerifiedAt == nil {
		t.Fatal("DobVerifiedAt was cleared by an edit that never mentioned it")
	}
	if !got.DobVerifiedAt.Equal(*verified.DobVerifiedAt) {
		t.Errorf("DobVerifiedAt = %v, want %v", got.DobVerifiedAt, verified.DobVerifiedAt)
	}
}

// The other half of the contract: a field the caller did send as empty must be
// written as empty, or an admin can never clear a stale note or a wrong email.
func TestUpdateWritesFieldsSentAsEmpty(t *testing.T) {
	database, svc := newFixture(t)

	c := db.Competitor{NameFirst: "Ada", NameLast: "Lovelace", Email: "stale@example.com", Note: "old note"}
	if err := database.Create(&c).Error; err != nil {
		t.Fatalf("seeding competitor: %v", err)
	}

	got, err := svc.Update(c.ID, db.Competitor{Email: "", Note: ""}, fields("email", "note"), "Alice Admin")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Email != "" || got.Note != "" {
		t.Errorf("explicitly emptied fields were not cleared: email=%q note=%q", got.Email, got.Note)
	}
	if got.NameLast != "Lovelace" {
		t.Errorf("NameLast = %q, want Lovelace", got.NameLast)
	}
}

// Roster membership is attendance rows, not a column, so an edit must not be
// able to drop one. registerForEvent only ever adds.
func TestUpdateKeepsRosterMembership(t *testing.T) {
	database, svc := newFixture(t)

	seedEventOn(t, database, "glr-2026", false, time.Date(2026, 4, 17, 0, 0, 0, 0, time.UTC))
	seedEventOn(t, database, "nat-2026", true, time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC))

	c := seedCompetitor(t, database, "Ada", "Lovelace")
	register(t, database, c.ID, "glr-2026")
	register(t, database, c.ID, "nat-2026")

	if _, err := svc.Update(c.ID, db.Competitor{NameFirst: "Adaline"}, fields("nameFirst"), "Alice Admin"); err != nil {
		t.Fatalf("Update: %v", err)
	}

	var events []string
	if err := database.Model(&db.CompetitorEvent{}).
		Where("competitor_id = ?", c.ID).Order("event_id").Pluck("event_id", &events).Error; err != nil {
		t.Fatalf("reading roster: %v", err)
	}
	if !slices.Equal(events, []string{"glr-2026", "nat-2026"}) {
		t.Errorf("roster after edit = %v, want both events", events)
	}

	// An unknown event is rejected rather than silently registering nothing.
	if _, err := svc.Update(c.ID, db.Competitor{RegisterForEvent: "no-such-event"},
		fields(), "Alice Admin"); !errors.Is(err, ErrUnknownEvent) {
		t.Errorf("expected ErrUnknownEvent, got %v", err)
	}
}

// A competitor with no name is unsearchable, blank on every screen, and
// collides with every other nameless row on the import name key.
func TestCreateAndUpdateRejectBlankNames(t *testing.T) {
	database, svc := newFixture(t)

	for _, tc := range []struct{ name, first, last string }{
		{"both empty", "", ""},
		{"whitespace only", "   ", "\t\n"},
		{"missing surname", "Ada", ""},
	} {
		c := db.Competitor{NameFirst: tc.first, NameLast: tc.last}
		if err := svc.Create(&c, "Alice Admin"); !errors.Is(err, ErrBlankName) {
			t.Errorf("Create(%s): expected ErrBlankName, got %v", tc.name, err)
		}
	}
	var created int64
	database.Model(&db.Competitor{}).Count(&created)
	if created != 0 {
		t.Errorf("%d nameless competitors were created", created)
	}

	existing := seedCompetitor(t, database, "Ada", "Lovelace")
	if _, err := svc.Update(existing.ID, db.Competitor{NameLast: "  "},
		fields("nameLast"), "Alice Admin"); !errors.Is(err, ErrBlankName) {
		t.Errorf("Update: expected ErrBlankName, got %v", err)
	}
	var after db.Competitor
	database.First(&after, "id = ?", existing.ID)
	if after.NameLast != "Lovelace" {
		t.Errorf("rejected edit still wrote: NameLast = %q", after.NameLast)
	}
}

// Editing a competitor who is already gone is a 404 case, not a 500.
func TestUpdateReturnsNotFoundForMissingCompetitor(t *testing.T) {
	_, svc := newFixture(t)

	_, err := svc.Update("00000000-0000-0000-0000-000000000000",
		db.Competitor{NameFirst: "Ghost"}, fields("nameFirst"), "Alice Admin")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// Migration 001 adds the foreign key that makes this true. Before it, deleting a
// competitor left their attendance rows behind as orphans.
func TestDeleteCascadesToAttendance(t *testing.T) {
	database, svc := newFixture(t)

	seedEvent(t, database, "nat-2026", true)
	c := seedCompetitor(t, database, "Ada", "Lovelace")
	register(t, database, c.ID, "nat-2026")

	if err := svc.Delete(c.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	var orphans int64
	database.Model(&db.CompetitorEvent{}).Where("competitor_id = ?", c.ID).Count(&orphans)
	if orphans != 0 {
		t.Errorf("expected attendance rows to cascade, %d left behind", orphans)
	}
}

// Migration 003 drops these, but AutoMigrate runs first and happily re-creates a
// column for any struct field that comes back — and it would not re-run the
// migration to undo that. This test is the guard against a legacy field being
// reintroduced by accident.
func TestLegacyColumnsAreGone(t *testing.T) {
	database, _ := newFixture(t)

	for _, column := range []string{
		"requires_validation", "validated", "last_registered_event",
		"is_checked_in", "check_in_date_time", "checked_in_by",
	} {
		var found int64
		if err := database.Raw(`
			SELECT count(*) FROM information_schema.columns
			WHERE table_schema = 'public' AND table_name = 'competitors' AND column_name = ?
		`, column).Scan(&found).Error; err != nil {
			t.Fatalf("inspecting columns: %v", err)
		}
		if found != 0 {
			t.Errorf("competitors.%s should have been dropped", column)
		}
	}
}

func TestPruneBackupsKeepsTheNewestSnapshots(t *testing.T) {
	database, svc := newFixture(t)

	var suffixes []int64
	for i := int64(1); i <= 5; i++ {
		suffixes = append(suffixes, i)
		if err := database.Exec(fmt.Sprintf(`
			CREATE TABLE competitors_backup_%d AS SELECT * FROM competitors;
			CREATE TABLE competitor_events_backup_%d AS SELECT * FROM competitor_events;
		`, i, i)).Error; err != nil {
			t.Fatalf("creating snapshot %d: %v", i, err)
		}
	}
	t.Cleanup(func() {
		for _, s := range suffixes {
			database.Exec(fmt.Sprintf(
				"DROP TABLE IF EXISTS competitors_backup_%d, competitor_events_backup_%d", s, s))
		}
	})

	if err := svc.pruneBackups(2); err != nil {
		t.Fatalf("pruneBackups: %v", err)
	}

	var remaining []string
	if err := database.Raw(`
		SELECT table_name FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name ~ '_backup_\d+$'
		ORDER BY table_name
	`).Scan(&remaining).Error; err != nil {
		t.Fatalf("listing backups: %v", err)
	}
	want := []string{
		"competitor_events_backup_4", "competitor_events_backup_5",
		"competitors_backup_4", "competitors_backup_5",
	}
	if !slices.Equal(remaining, want) {
		t.Errorf("expected the two newest snapshot pairs %v, got %v", want, remaining)
	}
}

// An event with registrations must not be deletable.
func TestEventDeleteIsRestrictedByAttendance(t *testing.T) {
	database, _ := newFixture(t)

	seedEvent(t, database, "nat-2026", true)
	c := seedCompetitor(t, database, "Ada", "Lovelace")
	register(t, database, c.ID, "nat-2026")

	if err := database.Delete(&db.Event{}, "id = ?", "nat-2026").Error; err == nil {
		t.Error("expected deleting an event with registrations to be rejected")
	}
}
