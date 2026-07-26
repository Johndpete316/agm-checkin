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

// adminEditToken is the bearer used by every test in this file. PATCH
// /api/competitors/{id} is admin-only, so the role is part of the setup rather
// than part of what is under test.
const adminEditToken = "dddd4444444444444444444444444444444444444444444444444444444444dd"

// seedFullCompetitor stores a competitor with every field populated and their
// date of birth already verified, so an edit has something to lose.
func seedFullCompetitor(t *testing.T, database *gorm.DB) db.Competitor {
	t.Helper()

	verifiedAt := time.Date(2026, 4, 13, 0, 0, 0, 0, time.UTC)
	c := db.Competitor{
		NameFirst:     "Ada",
		NameLast:      "Lovelace",
		DateOfBirth:   time.Date(2005, 3, 15, 0, 0, 0, 0, time.UTC),
		ShirtSize:     "M",
		Email:         "ada@example.com",
		Teacher:       "T One",
		Studio:        "S One",
		Note:          "needs step-free access",
		DobVerifiedAt: &verifiedAt,
		DobVerifiedBy: "Alice Admin",
	}
	if err := database.Create(&c).Error; err != nil {
		t.Fatalf("seeding competitor: %v", err)
	}
	return c
}

func decodeCompetitor(t *testing.T, body []byte) db.Competitor {
	t.Helper()
	var out db.Competitor
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decoding competitor: %v", err)
	}
	return out
}

// A PATCH is not a PUT. Sending one field must not blank the rest — the handler
// used to decode into a struct and Save() it, so every field the caller left out
// arrived at the database as a Go zero value and overwrote what was stored.
// Everything lost this way (date of birth, email, studio, teacher, the staff
// note, the verification stamp) is only recoverable by asking the competitor
// again at the desk.
func TestPatchCompetitorPartialBodyPreservesStoredFields(t *testing.T) {
	database, router := newAuthFixture(t, authmw.TrustedProxyDirect)
	mintToken(t, database, "Ada", "Admin", "admin", adminEditToken)

	c := seedFullCompetitor(t, database)

	rec := request(t, router, http.MethodPatch, "/api/competitors/"+c.ID, "192.0.2.80:4000",
		bearer(adminEditToken), map[string]any{"nameFirst": "Adaline"})
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH returned %d: %s", rec.Code, rec.Body.String())
	}

	got := decodeCompetitor(t, rec.Body.Bytes())
	if got.NameFirst != "Adaline" {
		t.Errorf("NameFirst = %q, want Adaline", got.NameFirst)
	}
	for _, f := range []struct{ name, got, want string }{
		{"nameLast", got.NameLast, "Lovelace"},
		{"shirtSize", got.ShirtSize, "M"},
		{"email", got.Email, "ada@example.com"},
		{"teacher", got.Teacher, "T One"},
		{"studio", got.Studio, "S One"},
		{"note", got.Note, "needs step-free access"},
		{"dobVerifiedBy", got.DobVerifiedBy, "Alice Admin"},
	} {
		if f.got != f.want {
			t.Errorf("%s = %q, want %q — a field the body never mentioned was overwritten", f.name, f.got, f.want)
		}
	}
	if got.DateOfBirth.IsZero() {
		t.Error("dateOfBirth was zeroed by an edit that never mentioned it")
	}
	if got.DobVerifiedAt == nil {
		t.Error("dobVerifiedAt was cleared by an edit that never mentioned it")
	}

	// The response is only as good as the row behind it.
	var stored db.Competitor
	if err := database.First(&stored, "id = ?", c.ID).Error; err != nil {
		t.Fatalf("reloading competitor: %v", err)
	}
	if stored.NameLast != "Lovelace" || stored.Email != "ada@example.com" || stored.DobVerifiedAt == nil {
		t.Errorf("stored row was clobbered: %+v", stored)
	}
}

// Sending a field as null or empty is how an admin clears it, and that has to
// keep working — otherwise the fix for the partial body would make stale data
// uncorrectable.
func TestPatchCompetitorClearsFieldsSentAsEmpty(t *testing.T) {
	database, router := newAuthFixture(t, authmw.TrustedProxyDirect)
	mintToken(t, database, "Ada", "Admin", "admin", adminEditToken)

	c := seedFullCompetitor(t, database)

	rec := request(t, router, http.MethodPatch, "/api/competitors/"+c.ID, "192.0.2.81:4000",
		bearer(adminEditToken), map[string]any{"note": "", "dobVerifiedAt": nil})
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH returned %d: %s", rec.Code, rec.Body.String())
	}

	got := decodeCompetitor(t, rec.Body.Bytes())
	if got.Note != "" {
		t.Errorf("note = %q, want it cleared", got.Note)
	}
	if got.DobVerifiedAt != nil {
		t.Error("dobVerifiedAt should be revocable by sending it as null")
	}
	if got.Email != "ada@example.com" {
		t.Errorf("email = %q, want it untouched", got.Email)
	}
}

// Provenance is server-owned end to end, not just at the service boundary: the
// HTTP handler decodes straight into the same struct the client controls, so
// the guard has to hold on this path too.
func TestPatchCompetitorCannotForgeVerificationProvenance(t *testing.T) {
	database, router := newAuthFixture(t, authmw.TrustedProxyDirect)
	mintToken(t, database, "Ada", "Admin", "admin", adminEditToken)

	c := seedRoster(t, database, "nat-2026", true, "Grace", "Hopper")
	forged := "1999-01-01T00:00:00Z"

	rec := request(t, router, http.MethodPatch, "/api/competitors/"+c.ID, "192.0.2.82:4000",
		bearer(adminEditToken), map[string]any{
			"dobVerifiedAt": forged,
			"dobVerifiedBy": "Somebody Else",
		})
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH returned %d: %s", rec.Code, rec.Body.String())
	}

	got := decodeCompetitor(t, rec.Body.Bytes())
	if got.DobVerifiedAt == nil {
		t.Fatal("expected the competitor to end up verified")
	}
	if got.DobVerifiedAt.UTC().Year() == 1999 {
		t.Errorf("client-supplied verification timestamp was persisted: %v", got.DobVerifiedAt)
	}
	if got.DobVerifiedBy != "Ada Admin" {
		t.Errorf("dobVerifiedBy = %q, want the authenticated staff name", got.DobVerifiedBy)
	}
}

// The same guard on the create path, which any authenticated role can reach.
func TestPostCompetitorCannotForgeVerificationProvenance(t *testing.T) {
	database, router := newAuthFixture(t, authmw.TrustedProxyDirect)
	mintToken(t, database, "Ada", "Admin", "admin", adminEditToken)

	rec := request(t, router, http.MethodPost, "/api/competitors", "192.0.2.83:4000",
		bearer(adminEditToken), map[string]any{
			"nameFirst":     "Grace",
			"nameLast":      "Hopper",
			"dobVerifiedAt": "1999-01-01T00:00:00Z",
			"dobVerifiedBy": "Somebody Else",
		})
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST returned %d: %s", rec.Code, rec.Body.String())
	}

	got := decodeCompetitor(t, rec.Body.Bytes())
	if got.DobVerifiedAt != nil && got.DobVerifiedAt.UTC().Year() == 1999 {
		t.Errorf("client-supplied verification timestamp was persisted: %v", got.DobVerifiedAt)
	}
	if got.DobVerifiedBy != "Ada Admin" {
		t.Errorf("dobVerifiedBy = %q, want the authenticated staff name", got.DobVerifiedBy)
	}
}

// A nameless competitor is unsearchable and blank on every screen, and the
// import name key cannot tell two of them apart.
func TestCompetitorNamesAreRequired(t *testing.T) {
	database, router := newAuthFixture(t, authmw.TrustedProxyDirect)
	mintToken(t, database, "Ada", "Admin", "admin", adminEditToken)

	for _, body := range []map[string]any{
		{"nameFirst": "", "nameLast": ""},
		{"nameFirst": "   ", "nameLast": "\t"},
		{"shirtSize": "L"},
		{"nameFirst": "Ada"},
	} {
		rec := request(t, router, http.MethodPost, "/api/competitors", "192.0.2.84:4000",
			bearer(adminEditToken), body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("POST %v returned %d; want 400", body, rec.Code)
		}
	}

	var created int64
	database.Model(&db.Competitor{}).Count(&created)
	if created != 0 {
		t.Errorf("%d nameless competitors were created", created)
	}

	c := seedRoster(t, database, "nat-2026", true, "Ada", "Lovelace")
	rec := request(t, router, http.MethodPatch, "/api/competitors/"+c.ID, "192.0.2.84:4000",
		bearer(adminEditToken), map[string]any{"nameLast": "  "})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("PATCH blanking a surname returned %d; want 400", rec.Code)
	}
}

// Editing a competitor who has already been deleted is a 404. It used to be a
// 500 carrying GORM's "record not found" straight to the client.
func TestPatchMissingCompetitorReturns404(t *testing.T) {
	database, router := newAuthFixture(t, authmw.TrustedProxyDirect)
	mintToken(t, database, "Ada", "Admin", "admin", adminEditToken)

	rec := request(t, router, http.MethodPatch, "/api/competitors/00000000-0000-0000-0000-000000000000",
		"192.0.2.85:4000", bearer(adminEditToken), map[string]any{"nameFirst": "Ghost"})
	if rec.Code != http.StatusNotFound {
		t.Errorf("PATCH of a missing competitor returned %d; want 404", rec.Code)
	}
}
