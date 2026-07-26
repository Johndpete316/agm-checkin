package main

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"gorm.io/gorm"

	"johndpete316/agm-checkin-api/internal/db"
	authmw "johndpete316/agm-checkin-api/internal/middleware"
)

// seedRoster creates an event and a competitor registered for it.
func seedRoster(t *testing.T, database *gorm.DB, eventID string, current bool, first, last string) db.Competitor {
	t.Helper()

	var existing int64
	database.Model(&db.Event{}).Where("id = ?", eventID).Count(&existing)
	if existing == 0 {
		event := db.Event{ID: eventID, Name: eventID, IsCurrent: current, StartDate: time.Now(), EndDate: time.Now()}
		if err := database.Create(&event).Error; err != nil {
			t.Fatalf("seeding event %s: %v", eventID, err)
		}
	}

	c := db.Competitor{NameFirst: first, NameLast: last}
	if err := database.Create(&c).Error; err != nil {
		t.Fatalf("seeding competitor: %v", err)
	}
	if err := database.Create(&db.CompetitorEvent{CompetitorID: c.ID, EventID: eventID}).Error; err != nil {
		t.Fatalf("registering competitor: %v", err)
	}
	return c
}

// TestRegistrationRoleCannotWidenRosterScope checks that the eventId query
// parameter is an admin-only lever. A registration token is meant to see only
// the current roster; if eventId were honoured for them they could read every
// past event's competitor list by guessing event slugs.
func TestRegistrationRoleCannotWidenRosterScope(t *testing.T) {
	database, router := newAuthFixture(t, authmw.TrustedProxyDirect)

	current := seedRoster(t, database, "glr-2026", true, "Current", "Competitor")
	past := seedRoster(t, database, "nat-2024", false, "Past", "Competitor")

	regToken := "aaaa1111111111111111111111111111111111111111111111111111111111aa"
	adminToken := "bbbb2222222222222222222222222222222222222222222222222222222222bb"
	mintToken(t, database, "Rex", "Reg", "registration", regToken)
	mintToken(t, database, "Ada", "Admin", "admin", adminToken)

	names := func(headers map[string]string, query string) map[string]bool {
		rec := request(t, router, http.MethodGet, "/api/competitors"+query, "192.0.2.70:4000", headers, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET /api/competitors%s returned %d; want 200", query, rec.Code)
		}
		var out []struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decoding competitors: %v", err)
		}
		ids := map[string]bool{}
		for _, c := range out {
			ids[c.ID] = true
		}
		return ids
	}

	for _, query := range []string{"", "?eventId=nat-2024", "?eventId=all", "?eventId="} {
		got := names(bearer(regToken), query)
		if got[past.ID] {
			t.Errorf("registration token saw the nat-2024 competitor via %q; roster scope was widened", query)
		}
		if !got[current.ID] {
			t.Errorf("registration token could not see the current-roster competitor via %q", query)
		}
	}

	// Control: an admin can reach the past roster, so the assertions above are
	// about the role and not about the data being absent.
	adminPast := names(bearer(adminToken), "?eventId=nat-2024")
	if !adminPast[past.ID] {
		t.Errorf("admin could not scope to nat-2024; the control for this test is broken")
	}
}

// TestRegistrationRoleCanReadAnyCompetitorByID documents a gap in the scoping
// story rather than asserting the behaviour is right. GET /api/competitors is
// deliberately restricted to the current roster for registration staff, but
// GET /api/competitors/{id} and its /events and /schedule siblings apply no
// scope at all, so a known ID returns full PII (DOB, email, staff notes) for a
// competitor who is not on the current roster.
//
// Exploitability is limited: IDs are v4 UUIDs and the list endpoint will not
// hand a registration user an off-roster ID, so this is a defence-in-depth gap,
// not a live leak. Update this test if the by-ID routes are ever scoped.
func TestRegistrationRoleCanReadAnyCompetitorByID(t *testing.T) {
	database, router := newAuthFixture(t, authmw.TrustedProxyDirect)

	seedRoster(t, database, "glr-2026", true, "Current", "Competitor")
	past := seedRoster(t, database, "nat-2024", false, "Past", "Competitor")

	regToken := "cccc3333333333333333333333333333333333333333333333333333333333cc"
	mintToken(t, database, "Rex", "Reg", "registration", regToken)

	for _, path := range []string{
		"/api/competitors/" + past.ID,
		"/api/competitors/" + past.ID + "/events",
		"/api/competitors/" + past.ID + "/schedule",
	} {
		rec := request(t, router, http.MethodGet, path, "192.0.2.71:4000", bearer(regToken), nil)
		if rec.Code != http.StatusOK {
			t.Errorf("%s returned %d; expected 200 — current behaviour is unscoped by-ID access. "+
				"If this now 403s or 404s the gap has been closed and this test should be updated.",
				path, rec.Code)
		}
	}
}
