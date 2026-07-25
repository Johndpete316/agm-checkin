package service

import (
	"errors"
	"os"
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

func seedCompetitor(t *testing.T, database *gorm.DB, first, last, lastEvent string) db.Competitor {
	t.Helper()
	c := db.Competitor{NameFirst: first, NameLast: last, LastRegisteredEvent: lastEvent}
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
// This contract must survive the switch to competitor_events as the source of
// truth, so the fixture keeps last_registered_event and the roster row in
// agreement — a regression in either implementation fails this test.
func TestGetAllScopesRegistrationUsersToCurrentEvent(t *testing.T) {
	database, svc := newFixture(t)

	seedEvent(t, database, "glr-2026", false)
	seedEvent(t, database, "nat-2026", true)

	onRoster := seedCompetitor(t, database, "Ada", "Lovelace", "nat-2026")
	pastOnly := seedCompetitor(t, database, "Grace", "Hopper", "glr-2026")
	register(t, database, onRoster.ID, "nat-2026")
	register(t, database, pastOnly.ID, "glr-2026")

	got, err := svc.GetAll("", false)
	if err != nil {
		t.Fatalf("GetAll(registration): %v", err)
	}
	found := names(got)
	if !found["Lovelace"] || found["Hopper"] {
		t.Errorf("registration user should see only the current roster, got %v", found)
	}

	got, err = svc.GetAll("", true)
	if err != nil {
		t.Fatalf("GetAll(admin): %v", err)
	}
	found = names(got)
	if !found["Lovelace"] || !found["Hopper"] {
		t.Errorf("admin should see every competitor, got %v", found)
	}
}

func TestCheckInIsIdempotent(t *testing.T) {
	database, svc := newFixture(t)

	seedEvent(t, database, "nat-2026", true)
	c := seedCompetitor(t, database, "Ada", "Lovelace", "nat-2026")
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

	second, err := svc.CheckIn(c.ID, "Other Staff")
	if err != nil {
		t.Fatalf("second CheckIn: %v", err)
	}
	if second.CurrentCheckIn.ID != first.CurrentCheckIn.ID {
		t.Errorf("check-in created a second row: %s then %s",
			first.CurrentCheckIn.ID, second.CurrentCheckIn.ID)
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
	c := seedCompetitor(t, database, "Ada", "Lovelace", "nat-2026")

	if _, err := svc.CheckIn(c.ID, "Staff Member"); !errors.Is(err, ErrNoCurrentEvent) {
		t.Errorf("expected ErrNoCurrentEvent, got %v", err)
	}
}

func TestValidateOnlyAppliesWhenRequired(t *testing.T) {
	database, svc := newFixture(t)

	needsCheck := db.Competitor{NameFirst: "Ada", NameLast: "Lovelace", RequiresValidation: true}
	if err := database.Create(&needsCheck).Error; err != nil {
		t.Fatalf("seeding: %v", err)
	}
	got, err := svc.Validate(needsCheck.ID)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if !got.Validated {
		t.Error("expected competitor to be marked validated")
	}

	noCheck := seedCompetitor(t, database, "Grace", "Hopper", "")
	if _, err := svc.Validate(noCheck.ID); !errors.Is(err, ErrValidationNotRequired) {
		t.Errorf("expected ErrValidationNotRequired, got %v", err)
	}
}

// Migration 001 adds the foreign key that makes this true. Before it, deleting a
// competitor left their attendance rows behind as orphans.
func TestDeleteCascadesToAttendance(t *testing.T) {
	database, svc := newFixture(t)

	seedEvent(t, database, "nat-2026", true)
	c := seedCompetitor(t, database, "Ada", "Lovelace", "nat-2026")
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

// An event with registrations must not be deletable.
func TestEventDeleteIsRestrictedByAttendance(t *testing.T) {
	database, _ := newFixture(t)

	seedEvent(t, database, "nat-2026", true)
	c := seedCompetitor(t, database, "Ada", "Lovelace", "nat-2026")
	register(t, database, c.ID, "nat-2026")

	if err := database.Delete(&db.Event{}, "id = ?", "nat-2026").Error; err == nil {
		t.Error("expected deleting an event with registrations to be rejected")
	}
}
