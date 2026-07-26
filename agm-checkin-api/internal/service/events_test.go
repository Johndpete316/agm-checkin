package service

import (
	"errors"
	"testing"

	"gorm.io/gorm"

	"johndpete316/agm-checkin-api/internal/db"
)

func rosterOf(t *testing.T, database *gorm.DB, eventID string) map[string]bool {
	t.Helper()
	var rows []db.CompetitorEvent
	if err := database.Where("event_id = ?", eventID).Find(&rows).Error; err != nil {
		t.Fatalf("loading roster for %s: %v", eventID, err)
	}
	out := map[string]bool{}
	for _, r := range rows {
		out[r.CompetitorID] = r.CheckedIn
	}
	return out
}

func TestCopyRosterCarriesForwardOnlyMissingCompetitors(t *testing.T) {
	database, _ := newFixture(t)
	events := NewEventService(database)

	seedEvent(t, database, "nat-2026", true)
	seedEvent(t, database, "nat-2027", false)

	returning := seedCompetitor(t, database, "Ada", "Lovelace")
	alsoReturning := seedCompetitor(t, database, "Grace", "Hopper")
	already := seedCompetitor(t, database, "Alan", "Turing")

	for _, c := range []db.Competitor{returning, alsoReturning, already} {
		register(t, database, c.ID, "nat-2026")
	}
	register(t, database, already.ID, "nat-2027")

	result, err := events.CopyRoster("nat-2027", "nat-2026", false)
	if err != nil {
		t.Fatalf("copying roster: %v", err)
	}
	if result.SourceRoster != 3 || result.AlreadyOnTarget != 1 || result.Copied != 2 {
		t.Fatalf("expected 3 source / 1 already / 2 copied, got %d / %d / %d",
			result.SourceRoster, result.AlreadyOnTarget, result.Copied)
	}

	roster := rosterOf(t, database, "nat-2027")
	if len(roster) != 3 {
		t.Fatalf("expected 3 competitors on nat-2027, got %d", len(roster))
	}
	for id, checkedIn := range roster {
		if checkedIn {
			t.Fatalf("competitor %s was carried forward already checked in", id)
		}
	}
}

// A carry-forward may be run again after check-in has started, so it must not
// reset anyone who has already arrived.
func TestCopyRosterIsIdempotentAndPreservesCheckIns(t *testing.T) {
	database, competitors := newFixture(t)
	events := NewEventService(database)

	seedEvent(t, database, "nat-2026", false)
	seedEvent(t, database, "nat-2027", true)

	c := seedCompetitor(t, database, "Ada", "Lovelace")
	register(t, database, c.ID, "nat-2026")

	if _, err := events.CopyRoster("nat-2027", "nat-2026", false); err != nil {
		t.Fatalf("first copy: %v", err)
	}
	if _, err := competitors.CheckIn(c.ID, "Front Desk"); err != nil {
		t.Fatalf("checking in: %v", err)
	}

	second, err := events.CopyRoster("nat-2027", "nat-2026", false)
	if err != nil {
		t.Fatalf("second copy: %v", err)
	}
	if second.Copied != 0 {
		t.Fatalf("expected the second copy to be a no-op, copied %d", second.Copied)
	}

	roster := rosterOf(t, database, "nat-2027")
	if len(roster) != 1 {
		t.Fatalf("expected 1 competitor on nat-2027, got %d", len(roster))
	}
	if !roster[c.ID] {
		t.Fatal("re-running the copy cleared an existing check-in")
	}
}

func TestCopyRosterDryRunPredictsWithoutWriting(t *testing.T) {
	database, _ := newFixture(t)
	events := NewEventService(database)

	seedEvent(t, database, "nat-2026", true)
	seedEvent(t, database, "nat-2027", false)

	for _, name := range []string{"Ada", "Grace"} {
		c := seedCompetitor(t, database, name, "Tester")
		register(t, database, c.ID, "nat-2026")
	}

	preview, err := events.CopyRoster("nat-2027", "nat-2026", true)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if preview.Copied != 2 || !preview.DryRun {
		t.Fatalf("expected a dry run predicting 2, got %d (dryRun=%v)", preview.Copied, preview.DryRun)
	}
	if n := len(rosterOf(t, database, "nat-2027")); n != 0 {
		t.Fatalf("dry run wrote %d rows", n)
	}

	committed, err := events.CopyRoster("nat-2027", "nat-2026", false)
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if committed.Copied != preview.Copied {
		t.Fatalf("dry run predicted %d but commit copied %d", preview.Copied, committed.Copied)
	}
}

func TestCopyRosterRejectsBadEventPairs(t *testing.T) {
	database, _ := newFixture(t)
	events := NewEventService(database)

	seedEvent(t, database, "nat-2026", true)

	if _, err := events.CopyRoster("nat-2026", "nat-2026", false); !errors.Is(err, ErrSameEvent) {
		t.Fatalf("expected ErrSameEvent, got %v", err)
	}
	if _, err := events.CopyRoster("nat-2027", "nat-2026", false); !errors.Is(err, ErrEventNotFound) {
		t.Fatalf("expected ErrEventNotFound for an unknown target, got %v", err)
	}
	if _, err := events.CopyRoster("nat-2026", "glr-1999", false); !errors.Is(err, ErrEventNotFound) {
		t.Fatalf("expected ErrEventNotFound for an unknown source, got %v", err)
	}
}
