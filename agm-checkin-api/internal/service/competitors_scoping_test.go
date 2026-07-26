package service

import (
	"errors"
	"testing"
	"time"

	"johndpete316/agm-checkin-api/internal/db"
)

// offRosterFixture sets up the situation every test in this file is about: a
// past event, a current event, one competitor on each roster. The off-roster
// competitor carries real personal data so a leak is visible rather than
// implied.
func offRosterFixture(t *testing.T) (*CompetitorService, db.Competitor, db.Competitor) {
	t.Helper()
	database, svc := newFixture(t)

	seedEventOn(t, database, "glr-2026", false, time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC))
	seedEventOn(t, database, "nat-2026", true, time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC))

	onRoster := seedCompetitor(t, database, "Ada", "Lovelace")
	offRoster := db.Competitor{
		NameFirst:   "Grace",
		NameLast:    "Hopper",
		Email:       "grace@example.com",
		DateOfBirth: time.Date(2009, 12, 9, 0, 0, 0, 0, time.UTC),
		Note:        "internal staff note",
	}
	if err := database.Create(&offRoster).Error; err != nil {
		t.Fatalf("seeding off-roster competitor: %v", err)
	}
	register(t, database, onRoster.ID, "nat-2026")
	register(t, database, offRoster.ID, "glr-2026")

	return svc, onRoster, offRoster
}

// The roster filter and the name filter are two Where clauses on one query, and
// the second one is an OR. If they were ever composed without parentheses the
// name match would escape the roster match by SQL precedence and searching would
// return the whole database — the list would look scoped only until someone
// typed into the search box.
func TestSearchDoesNotEscapeTheRosterFilter(t *testing.T) {
	svc, _, offRoster := offRosterFixture(t)

	// Every one of these matches the off-roster competitor and nobody else.
	for _, search := range []string{"Hop", "Hopper", "Grace", "Grace Hopper", "grace hopper", "race Hop"} {
		got, err := svc.GetAll(search, false, "")
		if err != nil {
			t.Fatalf("GetAll(registration, search=%q): %v", search, err)
		}
		for _, c := range got {
			if c.ID == offRoster.ID {
				t.Errorf("search %q returned an off-roster competitor to a registration user", search)
			}
		}
	}

	// A search that matches both must still return only the one on the roster.
	got, err := svc.GetAll("a", false, "")
	if err != nil {
		t.Fatalf("GetAll(registration, search=%q): %v", "a", err)
	}
	found := names(got)
	if !found["Lovelace"] || found["Hopper"] {
		t.Errorf("search matching both rosters = %v, want Lovelace only", found)
	}
}

// Scoping the list alone is not scoping. Staff keep competitor IDs from every
// event they have worked, so a single-get that ignores the roster hands back a
// full record — date of birth, email, staff note — for someone outside the event
// the caller is staffing.
func TestGetByIDHidesOffRosterCompetitorsFromRegistrationUsers(t *testing.T) {
	svc, onRoster, offRoster := offRosterFixture(t)

	got, err := svc.GetByID(offRoster.ID, false)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetByID(off-roster, registration) = %v, %v; want ErrNotFound", got, err)
	}
	if got != nil {
		t.Errorf("off-roster record leaked to a registration user: email=%q dob=%v note=%q",
			got.Email, got.DateOfBirth, got.Note)
	}

	// The roster boundary is the only thing being enforced: a competitor on the
	// current roster still reads normally.
	visible, err := svc.GetByID(onRoster.ID, false)
	if err != nil {
		t.Fatalf("GetByID(on-roster, registration): %v", err)
	}
	if visible.NameLast != "Lovelace" {
		t.Errorf("expected Lovelace, got %q", visible.NameLast)
	}

	// Admins are unaffected — the import conflict-resolution flow reads by ID.
	if _, err := svc.GetByID(offRoster.ID, true); err != nil {
		t.Errorf("GetByID(off-roster, admin): %v", err)
	}
}

// The history endpoint is the one response that names every other event a
// competitor has attended, so an unscoped read discloses both that the person
// exists and where they have competed.
func TestGetEventHistoryHidesOffRosterCompetitorsFromRegistrationUsers(t *testing.T) {
	svc, onRoster, offRoster := offRosterFixture(t)

	history, err := svc.GetEventHistory(offRoster.ID, false)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("GetEventHistory(off-roster, registration) = %v, %v; want ErrNotFound", history, err)
	}
	if len(history) != 0 {
		t.Errorf("off-roster attendance leaked to a registration user: %d entries", len(history))
	}

	if _, err := svc.GetEventHistory(onRoster.ID, false); err != nil {
		t.Errorf("GetEventHistory(on-roster, registration): %v", err)
	}
	if _, err := svc.GetEventHistory(offRoster.ID, true); err != nil {
		t.Errorf("GetEventHistory(off-roster, admin): %v", err)
	}
}

// With no current event GetAll already returns nothing to registration staff.
// The single-get has to agree, or "no event is running" becomes the one state in
// which every record in the database is readable one ID at a time.
func TestReadsWithNoCurrentEventAreClosedToRegistrationUsers(t *testing.T) {
	database, svc := newFixture(t)

	seedEvent(t, database, "glr-2026", false)
	c := seedCompetitor(t, database, "Grace", "Hopper")
	register(t, database, c.ID, "glr-2026")

	got, err := svc.GetAll("", false, "")
	if err != nil {
		t.Fatalf("GetAll(registration): %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected an empty list with no current event, got %v", names(got))
	}

	if _, err := svc.GetByID(c.ID, false); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetByID with no current event = %v; want ErrNotFound", err)
	}
	if _, err := svc.GetEventHistory(c.ID, false); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetEventHistory with no current event = %v; want ErrNotFound", err)
	}

	// Admins still have to be able to work between events.
	if _, err := svc.GetByID(c.ID, true); err != nil {
		t.Errorf("GetByID(admin) with no current event: %v", err)
	}
}

// FINDING (S3, unfixed): the write side carries no roster check at all. Every
// mutation a registration token can reach applies to any competitor whose ID the
// caller knows, including one on no current roster. This test pins that as it
// stands today; when the boundary is extended to writes, the assertions below
// invert.
func TestMutationsAreNotScopedToTheCurrentRoster(t *testing.T) {
	svc, _, offRoster := offRosterFixture(t)

	note := "written by a registration user"
	email := "rewritten@example.com"
	contact, err := svc.UpdateContact(offRoster.ID, &note, &email)
	if err != nil {
		t.Fatalf("UpdateContact(off-roster): %v", err)
	}
	if contact.Note != note || contact.Email != email {
		t.Fatalf("UpdateContact did not apply: note=%q email=%q", contact.Note, contact.Email)
	}

	forged := time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)
	dob, err := svc.UpdateDOB(offRoster.ID, forged)
	if err != nil {
		t.Fatalf("UpdateDOB(off-roster): %v", err)
	}
	if !dob.DateOfBirth.Equal(forged) {
		t.Fatalf("UpdateDOB did not apply: %v", dob.DateOfBirth)
	}

	validated, err := svc.Validate(offRoster.ID, "Reg Staff")
	if err != nil {
		t.Fatalf("Validate(off-roster): %v", err)
	}
	if validated.DobVerifiedAt == nil {
		t.Fatal("Validate did not stamp verification")
	}

	// Check-in auto-registering a walk-up is deliberate; it is recorded here so
	// the difference from the reads above is explicit rather than assumed.
	checked, err := svc.CheckIn(offRoster.ID, "Reg Staff")
	if err != nil {
		t.Fatalf("CheckIn(off-roster): %v", err)
	}
	if checked.CurrentCheckIn.EventID != "nat-2026" {
		t.Errorf("check-in should have added them to the current roster, got %q",
			checked.CurrentCheckIn.EventID)
	}
}

// FINDING (S3, unfixed): schedule reads take the event from the caller and check
// neither the role nor the roster, so a registration token can read any
// competitor's schedule at any event by passing ?eventId=.
func TestScheduleReadsAreNotScopedToAnyRoster(t *testing.T) {
	database, _ := newFixture(t)
	scheduleSvc := NewScheduleService(database)

	seedEventOn(t, database, "glr-2026", false, time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC))
	seedEventOn(t, database, "nat-2026", true, time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC))

	offRoster := seedCompetitor(t, database, "Grace", "Hopper")
	register(t, database, offRoster.ID, "glr-2026")

	entry := db.CompetitorSchedule{
		CompetitorID: offRoster.ID,
		EventID:      "glr-2026",
		Instrument:   "Piano",
		ScheduleDate: time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC),
		ScheduleTime: "09:00",
		Room:         "Recital Hall",
		Category:     "Solo",
		Division:     "Junior",
	}
	if err := scheduleSvc.Create(&entry); err != nil {
		t.Fatalf("seeding schedule entry: %v", err)
	}

	// GetByCompetitorEvent has no role parameter to pass, which is the finding:
	// there is no layer at which the roster could be consulted.
	got, err := scheduleSvc.GetByCompetitorEvent(offRoster.ID, "glr-2026")
	if err != nil {
		t.Fatalf("GetByCompetitorEvent: %v", err)
	}
	if len(got) != 1 || got[0].Room != "Recital Hall" {
		t.Fatalf("expected the past event's schedule to come back unscoped, got %v", got)
	}
}

// FINDING (S4, unfixed): nothing stops two events being flagged current at once.
// SetCurrent clears the others in a transaction, but an import or a hand-written
// UPDATE can leave two rows set, and the resolver then silently picks the lowest
// event ID rather than reporting the ambiguity.
func TestNothingEnforcesASingleCurrentEvent(t *testing.T) {
	database, svc := newFixture(t)

	seedEventOn(t, database, "glr-2026", true, time.Date(2026, 3, 14, 0, 0, 0, 0, time.UTC))
	seedEventOn(t, database, "nat-2026", true, time.Date(2026, 7, 30, 0, 0, 0, 0, time.UTC))

	var current int64
	database.Model(&db.Event{}).Where("is_current = true").Count(&current)
	if current != 2 {
		t.Fatalf("expected two events to be flagged current, got %d", current)
	}

	// Lowest ID wins, so the older event shadows the one that was set current
	// most recently — the roster staff see is the wrong one, with no error.
	if got := svc.currentEventID(); got != "glr-2026" {
		t.Errorf("currentEventID() = %q, want the lowest ID glr-2026", got)
	}
}

// The staff note rides along in every competitor read, including a registration
// user's, while only an admin route is documented as being able to edit it. The
// contact route, open to all roles, writes it.
func TestNoteIsReadableAndWritableByRegistrationUsers(t *testing.T) {
	database, svc := newFixture(t)

	seedEvent(t, database, "nat-2026", true)
	c := db.Competitor{NameFirst: "Ada", NameLast: "Lovelace", Note: "internal staff note"}
	if err := database.Create(&c).Error; err != nil {
		t.Fatalf("seeding competitor: %v", err)
	}
	register(t, database, c.ID, "nat-2026")

	got, err := svc.GetAll("", false, "")
	if err != nil {
		t.Fatalf("GetAll(registration): %v", err)
	}
	if len(got) != 1 || got[0].Note != "internal staff note" {
		t.Errorf("expected the note to be returned to registration users, got %v", got)
	}

	// FINDING (S4, unfixed): PATCH /competitors/{id}/contact is open to all roles
	// and writes note, which the documentation reserves for the admin-only
	// PATCH /competitors/{id}.
	note := "rewritten by a registration user"
	updated, err := svc.UpdateContact(c.ID, &note, nil)
	if err != nil {
		t.Fatalf("UpdateContact: %v", err)
	}
	if updated.Note != note {
		t.Errorf("expected the note to be writable through the contact route, got %q", updated.Note)
	}
}

// UpdateContact takes note and email as explicit arguments rather than a decoded
// struct, so there is no field a caller could add to the request body to reach
// another column. This is the guard against that changing.
func TestUpdateContactCannotReachOtherFields(t *testing.T) {
	database, svc := newFixture(t)

	seedEvent(t, database, "nat-2026", true)
	c := db.Competitor{
		NameFirst:   "Ada",
		NameLast:    "Lovelace",
		Studio:      "Original Studio",
		Teacher:     "Original Teacher",
		ShirtSize:   "M",
		DateOfBirth: time.Date(2009, 12, 9, 0, 0, 0, 0, time.UTC),
	}
	if err := database.Create(&c).Error; err != nil {
		t.Fatalf("seeding competitor: %v", err)
	}
	register(t, database, c.ID, "nat-2026")

	note := "note"
	if _, err := svc.UpdateContact(c.ID, &note, nil); err != nil {
		t.Fatalf("UpdateContact: %v", err)
	}

	var after db.Competitor
	if err := database.First(&after, "id = ?", c.ID).Error; err != nil {
		t.Fatalf("reloading competitor: %v", err)
	}
	switch {
	case after.NameLast != "Lovelace":
		t.Errorf("name was modified: %q", after.NameLast)
	case after.Studio != "Original Studio" || after.Teacher != "Original Teacher":
		t.Errorf("studio/teacher were modified: %q / %q", after.Studio, after.Teacher)
	case after.ShirtSize != "M":
		t.Errorf("shirt size was modified: %q", after.ShirtSize)
	case after.DobVerifiedAt != nil || after.DobVerifiedBy != "":
		t.Errorf("verification provenance was forged: %v / %q", after.DobVerifiedAt, after.DobVerifiedBy)
	case !after.DateOfBirth.Equal(c.DateOfBirth):
		t.Errorf("date of birth was modified: %v", after.DateOfBirth)
	}
}
