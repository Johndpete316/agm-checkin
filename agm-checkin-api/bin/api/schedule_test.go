package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"johndpete316/agm-checkin-api/internal/db"
	authmw "johndpete316/agm-checkin-api/internal/middleware"
)

// Tokens are hex so uuidLike can derive a valid UUID from them.
const (
	scheduleAdminToken = "5c11ed01e5c11ed01e5c11ed01e5c11ed01e5c11ed01e5c11ed01e5c11ed01e5"
	scheduleRegToken   = "6e6f7461646d696e6e6f7461646d696e6e6f7461646d696e6e6f7461646d696e"
)

// scheduleBody is the JSON POST /api/competitors/{id}/schedule accepts. It is
// spelled out here rather than reused from the handler so a change to the wire
// format has to be made deliberately in both places.
func scheduleBody(eventID string) map[string]any {
	return map[string]any{
		"eventId":      eventID,
		"instrument":   "Piano",
		"scheduleDate": "2026-06-01T00:00:00Z",
		"scheduleTime": "10:30 AM",
		"room":         "Room 1",
		"category":     "Concerto",
		"division":     "Senior",
		"sortOrder":    630,
	}
}

// scheduleEntry is the response shape of every schedule endpoint.
type scheduleEntry struct {
	ID           string    `json:"id"`
	CompetitorID string    `json:"competitorId"`
	EventID      string    `json:"eventId"`
	Instrument   string    `json:"instrument"`
	ScheduleDate time.Time `json:"scheduleDate"`
	ScheduleTime string    `json:"scheduleTime"`
	Room         string    `json:"room"`
	Category     string    `json:"category"`
	Division     string    `json:"division"`
	SortOrder    int       `json:"sortOrder"`
}

func decodeEntry(t *testing.T, rec *httptest.ResponseRecorder) scheduleEntry {
	t.Helper()
	var out scheduleEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decoding schedule entry from %q: %v", rec.Body.String(), err)
	}
	return out
}

func decodeEntries(t *testing.T, rec *httptest.ResponseRecorder) []scheduleEntry {
	t.Helper()
	var out []scheduleEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decoding schedule list from %q: %v", rec.Body.String(), err)
	}
	return out
}

// createSlot posts one fully-populated entry and returns it.
func createSlot(t *testing.T, router *chi.Mux, token, competitorID, eventID string) scheduleEntry {
	t.Helper()
	rec := request(t, router, http.MethodPost,
		"/api/competitors/"+competitorID+"/schedule", "", bearer(token), scheduleBody(eventID))
	if rec.Code != http.StatusCreated {
		t.Fatalf("creating a slot: status %d body %s", rec.Code, rec.Body.String())
	}
	return decodeEntry(t, rec)
}

// QA-SCH-02: PATCH /api/schedule/{id} wrote the whole row, so a request naming
// one field blanked every field it omitted and reset schedule_date to
// 0001-01-01. Editing a room number destroyed the slot's time and category.
func TestSchedulePatchOnlyChangesNamedFields(t *testing.T) {
	database, router := newAuthFixture(t, authmw.TrustedProxyDirect)
	admin := mintToken(t, database, "Ad", "Min", "admin", scheduleAdminToken)
	c := seedRoster(t, database, "nat-2026", true, "Patch", "Probe")

	entry := createSlot(t, router, admin.Token, c.ID, "nat-2026")

	rec := request(t, router, http.MethodPatch, "/api/schedule/"+entry.ID, "",
		bearer(admin.Token), map[string]any{"room": "Room 2"})
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH status %d body %s", rec.Code, rec.Body.String())
	}
	got := decodeEntry(t, rec)

	if got.Room != "Room 2" {
		t.Fatalf("room = %q, want %q", got.Room, "Room 2")
	}
	if got.Instrument != entry.Instrument {
		t.Errorf("instrument = %q, want it unchanged at %q", got.Instrument, entry.Instrument)
	}
	if got.ScheduleTime != entry.ScheduleTime {
		t.Errorf("scheduleTime = %q, want it unchanged at %q", got.ScheduleTime, entry.ScheduleTime)
	}
	if got.Category != entry.Category {
		t.Errorf("category = %q, want it unchanged at %q", got.Category, entry.Category)
	}
	if got.Division != entry.Division {
		t.Errorf("division = %q, want it unchanged at %q", got.Division, entry.Division)
	}
	if got.SortOrder != entry.SortOrder {
		t.Errorf("sortOrder = %d, want it unchanged at %d", got.SortOrder, entry.SortOrder)
	}
	if !got.ScheduleDate.Equal(entry.ScheduleDate) {
		t.Errorf("scheduleDate = %s, want it unchanged at %s", got.ScheduleDate, entry.ScheduleDate)
	}

	// The response must match what was actually persisted.
	var stored db.CompetitorSchedule
	if err := database.First(&stored, "id = ?", entry.ID).Error; err != nil {
		t.Fatalf("re-reading the stored row: %v", err)
	}
	if stored.ScheduleTime != entry.ScheduleTime || stored.Category != entry.Category {
		t.Fatalf("stored row was flattened: %+v", stored)
	}
}

// An empty PATCH body is a no-op. Before the partial-update fix it wiped the row.
func TestSchedulePatchWithEmptyBodyIsANoOp(t *testing.T) {
	database, router := newAuthFixture(t, authmw.TrustedProxyDirect)
	admin := mintToken(t, database, "Ad", "Min", "admin", scheduleAdminToken)
	c := seedRoster(t, database, "nat-2026", true, "Empty", "Patch")

	entry := createSlot(t, router, admin.Token, c.ID, "nat-2026")

	rec := request(t, router, http.MethodPatch, "/api/schedule/"+entry.ID, "",
		bearer(admin.Token), map[string]any{})
	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH status %d body %s", rec.Code, rec.Body.String())
	}
	got := decodeEntry(t, rec)
	if got != entry {
		t.Fatalf("empty PATCH changed the entry:\n got  %+v\n want %+v", got, entry)
	}
}

// The competitor and event a slot belongs to are fixed at creation. PATCH takes
// no id from the body, and the service refuses the columns outright, so neither
// a JSON key nor a column name can move a slot to somebody else.
func TestSchedulePatchCannotReassignOwnership(t *testing.T) {
	database, router := newAuthFixture(t, authmw.TrustedProxyDirect)
	admin := mintToken(t, database, "Ad", "Min", "admin", scheduleAdminToken)
	owner := seedRoster(t, database, "nat-2026", true, "Slot", "Owner")
	victim := seedRoster(t, database, "nat-2026", true, "Slot", "Victim")

	entry := createSlot(t, router, admin.Token, owner.ID, "nat-2026")

	for _, body := range []map[string]any{
		{"competitorId": victim.ID},
		{"competitor_id": victim.ID},
		{"eventId": "glr-2025"},
		{"id": "00000000-0000-0000-0000-0000000000ff"},
	} {
		rec := request(t, router, http.MethodPatch, "/api/schedule/"+entry.ID, "",
			bearer(admin.Token), body)
		if rec.Code != http.StatusOK {
			t.Fatalf("PATCH %v status %d body %s", body, rec.Code, rec.Body.String())
		}
		got := decodeEntry(t, rec)
		if got.ID != entry.ID || got.CompetitorID != owner.ID || got.EventID != "nat-2026" {
			t.Fatalf("PATCH %v reassigned the slot: %+v", body, got)
		}
	}
}

// The competitor a new slot belongs to comes from the URL only; a competitorId
// in the body is not a second, competing source of truth.
func TestScheduleCreateTakesTheCompetitorFromTheURL(t *testing.T) {
	database, router := newAuthFixture(t, authmw.TrustedProxyDirect)
	admin := mintToken(t, database, "Ad", "Min", "admin", scheduleAdminToken)
	target := seedRoster(t, database, "nat-2026", true, "Url", "Target")
	other := seedRoster(t, database, "nat-2026", true, "Body", "Claimant")

	body := scheduleBody("nat-2026")
	body["competitorId"] = other.ID
	body["competitor_id"] = other.ID
	body["id"] = "00000000-0000-0000-0000-0000000000ff"

	rec := request(t, router, http.MethodPost,
		"/api/competitors/"+target.ID+"/schedule", "", bearer(admin.Token), body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST status %d body %s", rec.Code, rec.Body.String())
	}
	got := decodeEntry(t, rec)
	if got.CompetitorID != target.ID {
		t.Fatalf("competitorId = %q, want the URL's %q", got.CompetitorID, target.ID)
	}
	if got.ID == "00000000-0000-0000-0000-0000000000ff" {
		t.Fatal("the body's id was accepted as the row's primary key")
	}
}

// QA-SCH-01: BulkUpsert deletes the competitor's existing schedule before
// inserting the new one. Without a transaction around both, an insert that
// fails — here by exceeding the 65535-bind-parameter ceiling — committed the
// delete and left the competitor with nothing.
func TestScheduleImportSurvivesALargePayload(t *testing.T) {
	database, router := newAuthFixture(t, authmw.TrustedProxyDirect)
	admin := mintToken(t, database, "Ad", "Min", "admin", scheduleAdminToken)
	c := seedRoster(t, database, "nat-2026", true, "Import", "Probe")

	createSlot(t, router, admin.Token, c.ID, "nat-2026")

	const rows = 10000
	entries := make([]map[string]any, rows)
	for i := range entries {
		entries[i] = map[string]any{
			"instrument":   "Piano",
			"scheduleDate": "2026-06-01T00:00:00Z",
			"scheduleTime": "9:00 AM",
			"room":         "Room 1",
			"category":     "Solo",
			"division":     "Junior",
			"sortOrder":    540,
		}
	}

	rec := request(t, router, http.MethodPost,
		"/api/competitors/"+c.ID+"/schedule/import", "", bearer(admin.Token),
		map[string]any{"eventId": "nat-2026", "entries": entries})

	var stored int64
	if err := database.Model(&db.CompetitorSchedule{}).
		Where("competitor_id = ? AND event_id = ?", c.ID, "nat-2026").
		Count(&stored).Error; err != nil {
		t.Fatalf("counting: %v", err)
	}

	if rec.Code != http.StatusOK {
		// A rejected import is defensible; silently destroying the schedule it
		// was replacing is not.
		if stored == 0 {
			t.Fatalf("import failed with %d and wiped the existing schedule: %s",
				rec.Code, rec.Body.String())
		}
		return
	}
	if stored != rows {
		t.Fatalf("stored rows = %d, want %d", stored, rows)
	}
}

// Re-importing replaces rather than accumulates, which is the feature's only
// duplicate protection: idx_cs_competitor_event is deliberately non-unique.
func TestScheduleImportTwiceDoesNotDuplicate(t *testing.T) {
	database, router := newAuthFixture(t, authmw.TrustedProxyDirect)
	admin := mintToken(t, database, "Ad", "Min", "admin", scheduleAdminToken)
	c := seedRoster(t, database, "nat-2026", true, "Twice", "Import")

	payload := map[string]any{"eventId": "nat-2026", "entries": []map[string]any{{
		"instrument":   "Piano",
		"scheduleDate": "2026-06-01T00:00:00Z",
		"scheduleTime": "9:00 AM",
		"room":         "Room 1",
		"category":     "Solo",
		"division":     "Junior",
		"sortOrder":    540,
	}}}

	for round := 1; round <= 2; round++ {
		rec := request(t, router, http.MethodPost,
			"/api/competitors/"+c.ID+"/schedule/import", "", bearer(admin.Token), payload)
		if rec.Code != http.StatusOK {
			t.Fatalf("round %d: status %d body %s", round, rec.Code, rec.Body.String())
		}
	}

	rec := request(t, router, http.MethodGet,
		"/api/competitors/"+c.ID+"/schedule?eventId=nat-2026", "", bearer(admin.Token), nil)
	if got := len(decodeEntries(t, rec)); got != 1 {
		t.Fatalf("entries after two imports = %d, want 1", got)
	}
}

// The import endpoint is JSON-only. The competitor CSV import next to it takes a
// multipart "file" field, and the two being different is easy to trip over.
func TestScheduleImportRejectsNonJSONPayloads(t *testing.T) {
	database, router := newAuthFixture(t, authmw.TrustedProxyDirect)
	admin := mintToken(t, database, "Ad", "Min", "admin", scheduleAdminToken)
	c := seedRoster(t, database, "nat-2026", true, "Csv", "Probe")

	createSlot(t, router, admin.Token, c.ID, "nat-2026")

	for name, body := range map[string]any{
		"csv text":        "name,day,time\nA B,6/1/2026,9:00 AM\n",
		"missing eventId": map[string]any{"entries": []any{}},
	} {
		rec := request(t, router, http.MethodPost,
			"/api/competitors/"+c.ID+"/schedule/import", "", bearer(admin.Token), body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400 (body %s)", name, rec.Code, rec.Body.String())
		}
	}

	// A rejected import must not have touched the existing schedule.
	var stored int64
	if err := database.Model(&db.CompetitorSchedule{}).
		Where("competitor_id = ?", c.ID).Count(&stored).Error; err != nil {
		t.Fatalf("counting: %v", err)
	}
	if stored != 1 {
		t.Fatalf("rows after rejected imports = %d, want 1", stored)
	}
}

// QA-SCH-03: nothing ties a schedule slot to a roster. An admin can attach a
// slot to an event the competitor was never registered for, and it then shows
// up in that event's schedule read.
//
// Skipped: unfixed. Whether the roster is a precondition for scheduling is a
// product decision, so this records the intended behaviour without failing the
// build.
func TestScheduleCreateRequiresTheCompetitorToBeOnTheRoster(t *testing.T) {
	t.Skip("QA-SCH-03: unfixed — slots can be attached to events the competitor is not registered for")

	database, router := newAuthFixture(t, authmw.TrustedProxyDirect)
	admin := mintToken(t, database, "Ad", "Min", "admin", scheduleAdminToken)
	c := seedRoster(t, database, "nat-2026", true, "Roster", "Probe")
	seedRoster(t, database, "glr-2025", false, "Other", "Event")

	rec := request(t, router, http.MethodPost,
		"/api/competitors/"+c.ID+"/schedule", "", bearer(admin.Token), scheduleBody("glr-2025"))
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusConflict {
		t.Fatalf("status %d, want 400/409 for an off-roster event (body %s)", rec.Code, rec.Body.String())
	}
}

// QA-SCH-04: nothing validates schedule_time, sort_order, schedule_date against
// the event window, or the NOT NULL text columns being empty. Every payload
// below is stored verbatim and served back to the schedule display.
//
// Skipped: unfixed. Production already carries 43 rows with an empty division,
// so tightening this needs a data decision first.
func TestScheduleCreateValidatesItsFields(t *testing.T) {
	t.Skip("QA-SCH-04: unfixed — schedule_time, sort_order, date range and empty required fields are all unvalidated")

	database, router := newAuthFixture(t, authmw.TrustedProxyDirect)
	admin := mintToken(t, database, "Ad", "Min", "admin", scheduleAdminToken)
	c := seedRoster(t, database, "nat-2026", true, "Validate", "Probe")

	cases := map[string]func(map[string]any){
		"unparseable time":    func(b map[string]any) { b["scheduleTime"] = "not a time" },
		"hour out of range":   func(b map[string]any) { b["scheduleTime"] = "25:00" },
		"no meridiem":         func(b map[string]any) { b["scheduleTime"] = "9:00" },
		"empty time":          func(b map[string]any) { b["scheduleTime"] = "" },
		"sortOrder disagrees": func(b map[string]any) { b["sortOrder"] = 0 },
		"date before event":   func(b map[string]any) { b["scheduleDate"] = "1999-01-01T00:00:00Z" },
		"empty division":      func(b map[string]any) { b["division"] = "" },
		"empty category":      func(b map[string]any) { b["category"] = "" },
		"empty instrument":    func(b map[string]any) { b["instrument"] = "" },
		"overlong room": func(b map[string]any) {
			long := make([]byte, 100000)
			for i := range long {
				long[i] = 'x'
			}
			b["room"] = string(long)
		},
	}

	for name, mutate := range cases {
		body := scheduleBody("nat-2026")
		mutate(body)
		rec := request(t, router, http.MethodPost,
			"/api/competitors/"+c.ID+"/schedule", "", bearer(admin.Token), body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400 (body %s)", name, rec.Code, rec.Body.String())
		}
	}
}

// QA-SCH-05: unknown or malformed identifiers reach Postgres and come back as a
// 500 carrying the raw driver message — constraint names, SQLSTATE codes and
// column types included.
//
// Skipped: unfixed.
func TestScheduleUnknownIdentifiersDoNotReturn500(t *testing.T) {
	t.Skip("QA-SCH-05: unfixed — bad competitor/event ids surface as 500 with the raw Postgres error")

	database, router := newAuthFixture(t, authmw.TrustedProxyDirect)
	admin := mintToken(t, database, "Ad", "Min", "admin", scheduleAdminToken)
	reg := mintToken(t, database, "Reg", "User", "registration", scheduleRegToken)
	c := seedRoster(t, database, "nat-2026", true, "Ids", "Probe")

	cases := []struct {
		name   string
		method string
		path   string
		token  string
		body   any
	}{
		{"unknown competitor", http.MethodPost,
			"/api/competitors/00000000-0000-0000-0000-0000000000ff/schedule", admin.Token, scheduleBody("nat-2026")},
		{"malformed competitor", http.MethodPost,
			"/api/competitors/not-a-uuid/schedule", admin.Token, scheduleBody("nat-2026")},
		{"unknown event", http.MethodPost,
			"/api/competitors/" + c.ID + "/schedule", admin.Token, scheduleBody("no-such-event")},
		{"malformed schedule id on delete", http.MethodDelete,
			"/api/schedule/not-a-uuid", admin.Token, nil},
		{"malformed competitor on read", http.MethodGet,
			"/api/competitors/not-a-uuid/schedule", reg.Token, nil},
	}

	for _, tc := range cases {
		rec := request(t, router, tc.method, tc.path, "", bearer(tc.token), tc.body)
		if rec.Code >= 500 {
			t.Errorf("%s: status %d, want a 4xx (body %s)", tc.name, rec.Code, rec.Body.String())
		}
	}
}

// QA-SCH-06: idx_cs_competitor_event is non-unique and nothing else enforces a
// slot key, so the same competitor can be booked twice at the same date and
// time. Two competitors sharing a room at one time is legitimate — group
// categories do exactly that — so the invariant is per competitor, not per room.
//
// Skipped: unfixed. Production already carries two exact-duplicate slots, so a
// unique index needs those cleaned up in the same migration.
func TestScheduleRejectsDoubleBookingTheSameCompetitor(t *testing.T) {
	t.Skip("QA-SCH-06: unfixed — no uniqueness on (competitor_id, event_id, schedule_date, schedule_time)")

	database, router := newAuthFixture(t, authmw.TrustedProxyDirect)
	admin := mintToken(t, database, "Ad", "Min", "admin", scheduleAdminToken)
	c := seedRoster(t, database, "nat-2026", true, "Double", "Booked")
	other := seedRoster(t, database, "nat-2026", true, "Group", "Partner")

	createSlot(t, router, admin.Token, c.ID, "nat-2026")

	// Same competitor, same slot: must be refused.
	rec := request(t, router, http.MethodPost,
		"/api/competitors/"+c.ID+"/schedule", "", bearer(admin.Token), scheduleBody("nat-2026"))
	if rec.Code != http.StatusConflict {
		t.Errorf("duplicate slot for one competitor: status %d, want 409", rec.Code)
	}

	// A second competitor in the same room at the same time is a group entry
	// and must keep working.
	rec = request(t, router, http.MethodPost,
		"/api/competitors/"+other.ID+"/schedule", "", bearer(admin.Token), scheduleBody("nat-2026"))
	if rec.Code != http.StatusCreated {
		t.Errorf("group entry sharing a room: status %d, want 201", rec.Code)
	}
}

// QA-SCH-07 (see also QA-025): GET /api/competitors/{id}/schedule applies no
// roster or event scoping. A registration token reads any competitor's schedule
// for any event by naming the slug, including competitors who are not on the
// current roster and events that are not current.
//
// Skipped: unfixed.
func TestScheduleReadIsScopedToTheCurrentRoster(t *testing.T) {
	t.Skip("QA-SCH-07: unfixed — schedule reads honour an arbitrary eventId for any competitor (see QA-025)")

	database, router := newAuthFixture(t, authmw.TrustedProxyDirect)
	admin := mintToken(t, database, "Ad", "Min", "admin", scheduleAdminToken)
	reg := mintToken(t, database, "Reg", "User", "registration", scheduleRegToken)

	onRoster := seedRoster(t, database, "nat-2026", true, "Current", "Roster")
	offRoster := seedRoster(t, database, "glr-2025", false, "Past", "Roster")

	createSlot(t, router, admin.Token, onRoster.ID, "nat-2026")
	createSlot(t, router, admin.Token, offRoster.ID, "glr-2025")

	rec := request(t, router, http.MethodGet,
		"/api/competitors/"+offRoster.ID+"/schedule?eventId=glr-2025", "", bearer(reg.Token), nil)
	if rec.Code == http.StatusOK && len(decodeEntries(t, rec)) > 0 {
		t.Fatalf("a registration token read an off-roster competitor's past-event schedule: %s", rec.Body.String())
	}
}

// Deleting a slot removes exactly that slot and leaves the competitor's other
// entries in place.
func TestScheduleDeleteRemovesOnlyTheNamedSlot(t *testing.T) {
	database, router := newAuthFixture(t, authmw.TrustedProxyDirect)
	admin := mintToken(t, database, "Ad", "Min", "admin", scheduleAdminToken)
	c := seedRoster(t, database, "nat-2026", true, "Delete", "Probe")

	first := createSlot(t, router, admin.Token, c.ID, "nat-2026")
	second := createSlot(t, router, admin.Token, c.ID, "nat-2026")

	rec := request(t, router, http.MethodDelete, "/api/schedule/"+first.ID, "", bearer(admin.Token), nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE status %d body %s", rec.Code, rec.Body.String())
	}

	rec = request(t, router, http.MethodGet,
		"/api/competitors/"+c.ID+"/schedule?eventId=nat-2026", "", bearer(admin.Token), nil)
	remaining := decodeEntries(t, rec)
	if len(remaining) != 1 || remaining[0].ID != second.ID {
		t.Fatalf("remaining entries = %+v, want only %s", remaining, second.ID)
	}

	// Deleting again is a 404, not a silent success.
	rec = request(t, router, http.MethodDelete, "/api/schedule/"+first.ID, "", bearer(admin.Token), nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("second DELETE status %d, want 404", rec.Code)
	}
}

// Deleting a competitor takes their schedule with them, so a subsequent read
// returns nothing rather than another competitor's rows.
func TestCompetitorDeleteClearsTheirSchedule(t *testing.T) {
	database, router := newAuthFixture(t, authmw.TrustedProxyDirect)
	admin := mintToken(t, database, "Ad", "Min", "admin", scheduleAdminToken)
	c := seedRoster(t, database, "nat-2026", true, "Cascade", "Probe")

	createSlot(t, router, admin.Token, c.ID, "nat-2026")

	rec := request(t, router, http.MethodDelete, "/api/competitors/"+c.ID, "", bearer(admin.Token), nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE competitor status %d body %s", rec.Code, rec.Body.String())
	}

	var n int64
	if err := database.Model(&db.CompetitorSchedule{}).
		Where("competitor_id = ?", c.ID).Count(&n).Error; err != nil {
		t.Fatalf("counting: %v", err)
	}
	if n != 0 {
		t.Fatalf("schedule rows left behind = %d, want 0", n)
	}
}
