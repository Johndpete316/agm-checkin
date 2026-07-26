package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"johndpete316/agm-checkin-api/internal/db"
	authmw "johndpete316/agm-checkin-api/internal/middleware"
	"johndpete316/agm-checkin-api/internal/service"
)

const (
	testPIN      = "correct-horse-battery"
	testClientIP = "203.0.113.9"

	// testDBLockKey serialises the Postgres-backed test packages. bin/api and
	// internal/service share one scratch database and both truncate it on every
	// fixture, so they must not interleave when `go test ./...` runs packages in
	// parallel. internal/service holds the same lock.
	testDBLockKey = 8724532
)

func TestMain(m *testing.M) {
	os.Exit(runWithDatabaseLock(m.Run))
}

// runWithDatabaseLock holds a session-level advisory lock on the scratch
// database for the whole package run.
func runWithDatabaseLock(run func() int) int {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		return run()
	}
	database, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "acquiring test database lock: %v\n", err)
		return 1
	}
	sqlDB, err := database.DB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "acquiring test database lock: %v\n", err)
		return 1
	}
	defer sqlDB.Close()

	ctx := context.Background()
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "acquiring test database lock: %v\n", err)
		return 1
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "SELECT pg_advisory_lock($1)", testDBLockKey); err != nil {
		fmt.Fprintf(os.Stderr, "acquiring test database lock: %v\n", err)
		return 1
	}
	defer conn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", testDBLockKey)

	return run()
}

// apiFixture builds the production router (newRouter) against a scratch database
// so route coverage is measured against the real route table rather than a copy.
type apiFixture struct {
	t        *testing.T
	database *gorm.DB
	router   *chi.Mux
	audit    *service.AuditService
	admin    db.StaffToken
	adminTok string
}

// newAPIFixture returns a clean database plus the real router. Set
// TEST_DATABASE_URL to a scratch database — every run truncates.
func newAPIFixture(t *testing.T) *apiFixture {
	t.Helper()
	return newAPIFixtureWith(t, nil)
}

// newAPIFixtureWith allows a test to substitute a deliberately broken
// AuditService (see TestAuditWriteFailureDoesNotFailRequest).
func newAPIFixtureWith(t *testing.T, auditOverride *service.AuditService) *apiFixture {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database-backed tests")
	}

	// chi's request logger writes a line per request; silence it so the audit
	// assertions are readable under -v.
	log.SetOutput(io.Discard)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

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
		`TRUNCATE competitors, competitor_events, competitor_schedules, events,
		 audit_logs, staff_tokens, ip_blocklists, pin_attempts RESTART IDENTITY CASCADE`,
	).Error; err != nil {
		t.Fatalf("truncating: %v", err)
	}

	auditSvc := service.NewAuditService(database)
	if auditOverride != nil {
		auditSvc = auditOverride
	}

	router := newRouter(apiDeps{
		competitors:   service.NewCompetitorService(database),
		auth:          service.NewAuthService(database, testPIN),
		staff:         service.NewStaffService(database),
		events:        service.NewEventService(database),
		audit:         auditSvc,
		schedule:      service.NewScheduleService(database),
		allowedOrigin: "http://localhost:5173",
		ipResolver: func(r *http.Request) string {
			return authmw.GetClientIPWithMode(r, authmw.TrustedProxyCloudflare)
		},
		trustedProxy: authmw.TrustedProxyCloudflare,
	})

	f := &apiFixture{t: t, database: database, router: router, audit: auditSvc}
	// The bulk-import route snapshots competitors into timestamped backup tables.
	// Leaving them behind would pollute the shared scratch database for the
	// snapshot-retention tests in internal/service.
	t.Cleanup(f.dropBackupTables)
	f.admin = f.seedStaff("Ada", "Admin", "admin")
	f.adminTok = f.admin.Token
	return f
}

func (f *apiFixture) dropBackupTables() {
	var tables []string
	if err := f.database.Raw(`
		SELECT table_name FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name ~ '_backup_\d+$'
	`).Scan(&tables).Error; err != nil {
		return
	}
	for _, table := range tables {
		f.database.Exec(`DROP TABLE IF EXISTS ` + pq(table))
	}
}

// pq quotes an identifier read back from the catalog.
func pq(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func (f *apiFixture) seedStaff(first, last, role string) db.StaffToken {
	f.t.Helper()
	token := db.StaffToken{
		ID:        uuid.New().String(),
		Token:     uuid.New().String() + uuid.New().String(),
		FirstName: first,
		LastName:  last,
		Role:      role,
		CreatedAt: time.Now(),
	}
	if err := f.database.Create(&token).Error; err != nil {
		f.t.Fatalf("seeding staff %s %s: %v", first, last, err)
	}
	return token
}

func (f *apiFixture) seedEvent(id string, current bool) db.Event {
	f.t.Helper()
	event := db.Event{ID: id, Name: strings.ToUpper(id), IsCurrent: current, StartDate: time.Now(), EndDate: time.Now()}
	if err := f.database.Create(&event).Error; err != nil {
		f.t.Fatalf("seeding event %s: %v", id, err)
	}
	return event
}

func (f *apiFixture) seedCompetitor(first, last string) db.Competitor {
	f.t.Helper()
	c := db.Competitor{NameFirst: first, NameLast: last}
	if err := f.database.Create(&c).Error; err != nil {
		f.t.Fatalf("seeding competitor: %v", err)
	}
	return c
}

func (f *apiFixture) seedSchedule(competitorID, eventID string) db.CompetitorSchedule {
	f.t.Helper()
	entry := db.CompetitorSchedule{
		CompetitorID: competitorID,
		EventID:      eventID,
		Instrument:   "Piano",
		ScheduleDate: time.Now().Truncate(24 * time.Hour),
		ScheduleTime: "09:00",
		Category:     "Solo",
		Division:     "Junior",
	}
	if err := f.database.Create(&entry).Error; err != nil {
		f.t.Fatalf("seeding schedule: %v", err)
	}
	return entry
}

// do issues a JSON request through the full middleware chain.
func (f *apiFixture) do(method, path string, body any, token string) *httptest.ResponseRecorder {
	f.t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			f.t.Fatalf("marshalling body: %v", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	return f.send(req, token)
}

func (f *apiFixture) send(req *http.Request, token string) *httptest.ResponseRecorder {
	f.t.Helper()
	req.Header.Set("CF-Connecting-IP", testClientIP)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

func (f *apiFixture) auditRows() []db.AuditLog {
	f.t.Helper()
	var rows []db.AuditLog
	if err := f.database.Order("created_at asc").Find(&rows).Error; err != nil {
		f.t.Fatalf("reading audit rows: %v", err)
	}
	return rows
}

func (f *apiFixture) clearAudit() {
	f.t.Helper()
	if err := f.database.Exec(`TRUNCATE audit_logs`).Error; err != nil {
		f.t.Fatalf("truncating audit_logs: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 1. Coverage: every mutating route must write exactly one audit entry.
// ---------------------------------------------------------------------------

// routeCase describes one mutating route and a scenario that exercises it
// successfully. wantAction is the audit action the route is expected to emit;
// an empty wantAction records a route that writes no audit entry at all.
type routeCase struct {
	method     string
	pattern    string
	wantAction string
	run        func(f *apiFixture) *httptest.ResponseRecorder
}

func mutatingRouteCases() []routeCase {
	return []routeCase{
		{
			method: "POST", pattern: "/api/auth/token", wantAction: "staff.token_issued",
			run: func(f *apiFixture) *httptest.ResponseRecorder {
				return f.do("POST", "/api/auth/token", map[string]string{
					"code": testPIN, "firstName": "Grace", "lastName": "Hopper",
				}, "")
			},
		},
		{
			method: "POST", pattern: "/api/competitors", wantAction: "competitor.created",
			run: func(f *apiFixture) *httptest.ResponseRecorder {
				return f.do("POST", "/api/competitors", map[string]any{
					"nameFirst": "New", "nameLast": "Competitor",
				}, f.adminTok)
			},
		},
		{
			method: "PATCH", pattern: "/api/competitors/{id}", wantAction: "competitor.updated",
			run: func(f *apiFixture) *httptest.ResponseRecorder {
				c := f.seedCompetitor("Edit", "Me")
				return f.do("PATCH", "/api/competitors/"+c.ID, map[string]any{
					"nameFirst": "Edit", "nameLast": "Me", "studio": "Studio B",
				}, f.adminTok)
			},
		},
		{
			method: "PATCH", pattern: "/api/competitors/{id}/checkin", wantAction: "competitor.checked_in",
			run: func(f *apiFixture) *httptest.ResponseRecorder {
				f.seedEvent("glr-2026", true)
				c := f.seedCompetitor("Check", "In")
				return f.do("PATCH", "/api/competitors/"+c.ID+"/checkin", nil, f.adminTok)
			},
		},
		{
			method: "PATCH", pattern: "/api/competitors/{id}/contact", wantAction: "competitor.contact_updated",
			run: func(f *apiFixture) *httptest.ResponseRecorder {
				c := f.seedCompetitor("Contact", "Update")
				return f.do("PATCH", "/api/competitors/"+c.ID+"/contact", map[string]any{
					"note": "called parent", "email": "x@example.com",
				}, f.adminTok)
			},
		},
		{
			method: "PATCH", pattern: "/api/competitors/{id}/dob", wantAction: "competitor.dob_updated",
			run: func(f *apiFixture) *httptest.ResponseRecorder {
				c := f.seedCompetitor("Dob", "Update")
				return f.do("PATCH", "/api/competitors/"+c.ID+"/dob", map[string]any{
					"dateOfBirth": "2005-03-15T00:00:00Z",
				}, f.adminTok)
			},
		},
		{
			method: "PATCH", pattern: "/api/competitors/{id}/validate", wantAction: "competitor.validated",
			run: func(f *apiFixture) *httptest.ResponseRecorder {
				c := f.seedCompetitor("Val", "Idate")
				return f.do("PATCH", "/api/competitors/"+c.ID+"/validate", nil, f.adminTok)
			},
		},
		{
			method: "DELETE", pattern: "/api/competitors/{id}", wantAction: "competitor.deleted",
			run: func(f *apiFixture) *httptest.ResponseRecorder {
				c := f.seedCompetitor("Delete", "Me")
				return f.do("DELETE", "/api/competitors/"+c.ID, nil, f.adminTok)
			},
		},
		{
			method: "POST", pattern: "/api/competitors/import", wantAction: "competitor.bulk_import",
			run: func(f *apiFixture) *httptest.ResponseRecorder {
				body, contentType := csvUpload(f.t, "first_name,last_name\nBulk,Imported\n")
				req := httptest.NewRequest("POST", "/api/competitors/import", body)
				req.Header.Set("Content-Type", contentType)
				return f.send(req, f.adminTok)
			},
		},
		{
			method: "POST", pattern: "/api/competitors/{id}/schedule", wantAction: "schedule.created",
			run: func(f *apiFixture) *httptest.ResponseRecorder {
				f.seedEvent("glr-2026", true)
				c := f.seedCompetitor("Sched", "Create")
				return f.do("POST", "/api/competitors/"+c.ID+"/schedule", map[string]any{
					"eventId": "glr-2026", "instrument": "Piano",
					"scheduleDate": "2026-03-14T00:00:00Z", "scheduleTime": "10:00",
					"category": "Solo", "division": "Junior",
				}, f.adminTok)
			},
		},
		{
			method: "POST", pattern: "/api/competitors/{id}/schedule/import", wantAction: "schedule.bulk_import",
			run: func(f *apiFixture) *httptest.ResponseRecorder {
				f.seedEvent("glr-2026", true)
				c := f.seedCompetitor("Sched", "Import")
				return f.do("POST", "/api/competitors/"+c.ID+"/schedule/import", map[string]any{
					"eventId": "glr-2026",
					"entries": []map[string]any{{
						"instrument": "Violin", "scheduleDate": "2026-03-14T00:00:00Z",
						"scheduleTime": "11:00", "category": "Solo", "division": "Senior",
					}},
				}, f.adminTok)
			},
		},
		{
			method: "PATCH", pattern: "/api/schedule/{id}", wantAction: "schedule.updated",
			run: func(f *apiFixture) *httptest.ResponseRecorder {
				f.seedEvent("glr-2026", true)
				c := f.seedCompetitor("Sched", "Update")
				entry := f.seedSchedule(c.ID, "glr-2026")
				return f.do("PATCH", "/api/schedule/"+entry.ID, map[string]any{
					"instrument": "Cello", "scheduleDate": "2026-03-15T00:00:00Z",
					"scheduleTime": "12:00", "category": "Solo", "division": "Senior",
				}, f.adminTok)
			},
		},
		{
			method: "DELETE", pattern: "/api/schedule/{id}", wantAction: "schedule.deleted",
			run: func(f *apiFixture) *httptest.ResponseRecorder {
				f.seedEvent("glr-2026", true)
				c := f.seedCompetitor("Sched", "Delete")
				entry := f.seedSchedule(c.ID, "glr-2026")
				return f.do("DELETE", "/api/schedule/"+entry.ID, nil, f.adminTok)
			},
		},
		{
			method: "POST", pattern: "/api/events", wantAction: "event.created",
			run: func(f *apiFixture) *httptest.ResponseRecorder {
				return f.do("POST", "/api/events", map[string]any{
					"id": "nat-2027", "name": "NAT 2027",
					"startDate": "2027-06-01T00:00:00Z", "endDate": "2027-06-03T00:00:00Z",
				}, f.adminTok)
			},
		},
		{
			method: "PATCH", pattern: "/api/events/{id}/current", wantAction: "event.set_current",
			run: func(f *apiFixture) *httptest.ResponseRecorder {
				f.seedEvent("glr-2026", false)
				return f.do("PATCH", "/api/events/glr-2026/current", nil, f.adminTok)
			},
		},
		{
			method: "PATCH", pattern: "/api/staff/{id}/role", wantAction: "staff.role_updated",
			run: func(f *apiFixture) *httptest.ResponseRecorder {
				target := f.seedStaff("Role", "Target", "registration")
				return f.do("PATCH", "/api/staff/"+target.ID+"/role", map[string]string{"role": "admin"}, f.adminTok)
			},
		},
		{
			method: "DELETE", pattern: "/api/staff/{id}", wantAction: "staff.revoked",
			run: func(f *apiFixture) *httptest.ResponseRecorder {
				target := f.seedStaff("Revoke", "Target", "registration")
				return f.do("DELETE", "/api/staff/"+target.ID, nil, f.adminTok)
			},
		},
	}
}

func csvUpload(t *testing.T, content string) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "import.csv")
	if err != nil {
		t.Fatalf("creating form file: %v", err)
	}
	if _, err := part.Write([]byte(content)); err != nil {
		t.Fatalf("writing csv: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("closing writer: %v", err)
	}
	return &buf, writer.FormDataContentType()
}

// TestEveryMutatingRouteIsCovered walks the real route table and fails if a
// POST/PATCH/DELETE route exists that no audit case exercises. Without this, a
// new mutating endpoint can ship with no accountability trail and nothing
// notices.
func TestEveryMutatingRouteIsCovered(t *testing.T) {
	f := newAPIFixture(t)

	covered := map[string]bool{}
	for _, c := range mutatingRouteCases() {
		covered[c.method+" "+c.pattern] = true
	}

	var missing []string
	err := chi.Walk(f.router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if method != "POST" && method != "PATCH" && method != "DELETE" {
			return nil
		}
		route = strings.TrimSuffix(route, "/")
		if !covered[method+" "+route] {
			missing = append(missing, method+" "+route)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking routes: %v", err)
	}
	if len(missing) > 0 {
		t.Errorf("mutating routes with no audit coverage case: %v", missing)
	}
}

// TestMutatingRoutesWriteExactlyOneAuditEntry is the coverage matrix: each
// mutating route is driven to a successful response and must leave behind
// exactly one audit row carrying the expected action.
func TestMutatingRoutesWriteExactlyOneAuditEntry(t *testing.T) {
	for _, c := range mutatingRouteCases() {
		t.Run(c.method+" "+c.pattern, func(t *testing.T) {
			f := newAPIFixture(t)
			f.clearAudit()

			rec := c.run(f)
			if rec.Code < 200 || rec.Code > 299 {
				t.Fatalf("expected 2xx from %s %s, got %d: %s", c.method, c.pattern, rec.Code, rec.Body.String())
			}

			rows := f.auditRows()
			if c.wantAction == "" {
				if len(rows) != 0 {
					t.Fatalf("expected no audit entry, got %d", len(rows))
				}
				return
			}
			if len(rows) != 1 {
				var actions []string
				for _, r := range rows {
					actions = append(actions, r.Action)
				}
				t.Fatalf("expected exactly 1 audit entry for %s %s, got %d %v",
					c.method, c.pattern, len(rows), actions)
			}
			if rows[0].Action != c.wantAction {
				t.Errorf("action = %q, want %q", rows[0].Action, c.wantAction)
			}
			if rows[0].EntityType == "" || rows[0].EntityID == "" {
				t.Errorf("entity_type/entity_id must be populated, got %q/%q", rows[0].EntityType, rows[0].EntityID)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 2. Field correctness.
// ---------------------------------------------------------------------------

// TestAuditRecordsActorAndClientIP pins the actor identity and the resolved
// client IP: an audit trail that cannot attribute an action to a person and a
// source address is not an audit trail.
func TestAuditRecordsActorAndClientIP(t *testing.T) {
	f := newAPIFixture(t)
	f.seedEvent("glr-2026", true)
	c := f.seedCompetitor("Ada", "Lovelace")
	f.clearAudit()

	if rec := f.do("PATCH", "/api/competitors/"+c.ID+"/checkin", nil, f.adminTok); rec.Code != http.StatusOK {
		t.Fatalf("checkin: %d %s", rec.Code, rec.Body.String())
	}

	rows := f.auditRows()
	if len(rows) != 1 {
		t.Fatalf("expected 1 audit row, got %d", len(rows))
	}
	entry := rows[0]
	if entry.ActorID != f.admin.ID {
		t.Errorf("actor_id = %q, want %q", entry.ActorID, f.admin.ID)
	}
	if entry.ActorName != "Ada Admin" {
		t.Errorf("actor_name = %q, want %q", entry.ActorName, "Ada Admin")
	}
	if entry.IPAddress != testClientIP {
		t.Errorf("ip_address = %q, want the resolved client IP %q", entry.IPAddress, testClientIP)
	}
	if entry.EntityName != "Ada Lovelace" {
		t.Errorf("entity_name = %q, want %q", entry.EntityName, "Ada Lovelace")
	}
}

// TestAuditIPHonoursTrustedProxyMode makes sure the audit IP comes from the
// configured resolver rather than a hard-coded header preference: in direct
// mode a spoofed CF-Connecting-IP must not end up in the audit log.
func TestAuditIPHonoursTrustedProxyMode(t *testing.T) {
	f := newAPIFixture(t)
	// Rebuild the router in direct mode against the same database.
	f.router = newRouter(apiDeps{
		competitors:   service.NewCompetitorService(f.database),
		auth:          service.NewAuthService(f.database, testPIN),
		staff:         service.NewStaffService(f.database),
		events:        service.NewEventService(f.database),
		audit:         service.NewAuditService(f.database),
		schedule:      service.NewScheduleService(f.database),
		allowedOrigin: "http://localhost:5173",
		ipResolver: func(r *http.Request) string {
			return authmw.GetClientIPWithMode(r, authmw.TrustedProxyDirect)
		},
		trustedProxy: authmw.TrustedProxyDirect,
	})
	f.seedEvent("glr-2026", true)
	c := f.seedCompetitor("Ada", "Lovelace")
	f.clearAudit()

	if rec := f.do("PATCH", "/api/competitors/"+c.ID+"/checkin", nil, f.adminTok); rec.Code != http.StatusOK {
		t.Fatalf("checkin: %d %s", rec.Code, rec.Body.String())
	}

	rows := f.auditRows()
	if len(rows) != 1 {
		t.Fatalf("expected 1 audit row, got %d", len(rows))
	}
	if rows[0].IPAddress == testClientIP {
		t.Errorf("direct mode must ignore CF-Connecting-IP, but audit recorded %q", rows[0].IPAddress)
	}
	if rows[0].IPAddress != "192.0.2.1" { // httptest.NewRequest RemoteAddr
		t.Errorf("ip_address = %q, want RemoteAddr 192.0.2.1", rows[0].IPAddress)
	}
}

// TestDeleteCapturesEntityNameBeforeRowDisappears is the classic audit bug: if
// the name is read after the delete the trail records a bare UUID and nobody
// can tell who was removed.
func TestDeleteCapturesEntityNameBeforeRowDisappears(t *testing.T) {
	f := newAPIFixture(t)
	c := f.seedCompetitor("Grace", "Hopper")
	target := f.seedStaff("Alan", "Turing", "registration")
	f.clearAudit()

	if rec := f.do("DELETE", "/api/competitors/"+c.ID, nil, f.adminTok); rec.Code != http.StatusNoContent {
		t.Fatalf("delete competitor: %d %s", rec.Code, rec.Body.String())
	}
	if rec := f.do("DELETE", "/api/staff/"+target.ID, nil, f.adminTok); rec.Code != http.StatusNoContent {
		t.Fatalf("revoke staff: %d %s", rec.Code, rec.Body.String())
	}

	byAction := map[string]db.AuditLog{}
	for _, row := range f.auditRows() {
		byAction[row.Action] = row
	}
	if got := byAction["competitor.deleted"].EntityName; got != "Grace Hopper" {
		t.Errorf("competitor.deleted entity_name = %q, want %q", got, "Grace Hopper")
	}
	if got := byAction["staff.revoked"].EntityName; got != "Alan Turing" {
		t.Errorf("staff.revoked entity_name = %q, want %q", got, "Alan Turing")
	}
}

// TestDeleteOfMissingCompetitorWritesNoAuditEntry: an audit log that records
// deletions which never happened is worse than no log. Deleting an unknown id
// must 404 and leave the trail untouched.
func TestDeleteOfMissingCompetitorWritesNoAuditEntry(t *testing.T) {
	f := newAPIFixture(t)
	f.clearAudit()

	ghost := uuid.New().String()
	rec := f.do("DELETE", "/api/competitors/"+ghost, nil, f.adminTok)
	if rec.Code != http.StatusNotFound {
		t.Errorf("deleting an unknown competitor should 404, got %d", rec.Code)
	}
	if rows := f.auditRows(); len(rows) != 0 {
		t.Errorf("expected no audit entry for a delete that changed nothing, got %d (%q entity_name=%q)",
			len(rows), rows[0].Action, rows[0].EntityName)
	}
}

// TestTokenIssueIsAudited: every successful login mints a fresh, non-expiring
// bearer token. The trail has to show where a staff token came from, attributed
// to the new holder and the IP they logged in from.
func TestTokenIssueIsAudited(t *testing.T) {
	f := newAPIFixture(t)
	f.clearAudit()

	rec := f.do("POST", "/api/auth/token", map[string]string{
		"code": testPIN, "firstName": "Grace", "lastName": "Hopper",
	}, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("login: %d %s", rec.Code, rec.Body.String())
	}

	rows := f.auditRows()
	if len(rows) != 1 {
		t.Fatalf("expected 1 audit entry for a token issue, got %d", len(rows))
	}
	entry := rows[0]
	if entry.Action != "staff.token_issued" {
		t.Errorf("action = %q, want staff.token_issued", entry.Action)
	}
	if entry.ActorName != "Grace Hopper" || entry.EntityName != "Grace Hopper" {
		t.Errorf("actor/entity name = %q/%q, want Grace Hopper", entry.ActorName, entry.EntityName)
	}
	if entry.IPAddress != testClientIP {
		t.Errorf("ip_address = %q, want %q", entry.IPAddress, testClientIP)
	}
	var issued db.StaffToken
	if err := f.database.First(&issued, "id = ?", entry.EntityID).Error; err != nil {
		t.Fatalf("entity_id %q does not point at the created staff token: %v", entry.EntityID, err)
	}
	if issued.FirstName != "Grace" {
		t.Errorf("entity_id points at the wrong token: %+v", issued)
	}
}

// TestFailedLoginIsNotAudited documents FINDING (S3): failed PIN attempts and
// the IP lockout that follows them are recorded only in pin_attempts /
// ip_blocklists, never in the audit log, so a brute-force attempt is invisible
// on the audit page.
func TestFailedLoginIsNotAudited(t *testing.T) {
	f := newAPIFixture(t)
	f.clearAudit()

	rec := f.do("POST", "/api/auth/token", map[string]string{
		"code": "wrong-code", "firstName": "Mal", "lastName": "Lory",
	}, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for a bad code, got %d", rec.Code)
	}
	if rows := f.auditRows(); len(rows) != 0 {
		t.Fatalf("expected the known gap (no audit row), got %d", len(rows))
	}
	var attempts int64
	f.database.Model(&db.PINAttempt{}).Count(&attempts)
	t.Logf("FINDING S3: failed login recorded only in pin_attempts (%d rows), nothing in audit_logs", attempts)
}

// TestCompetitorRenameLosesTheOldName documents FINDING (S3):
// competitor.updated stores the post-update name and no before/after detail, so
// a rename cannot be traced back to who the record used to be.
func TestCompetitorRenameLosesTheOldName(t *testing.T) {
	f := newAPIFixture(t)
	c := f.seedCompetitor("Grace", "Hopper")
	f.clearAudit()

	rec := f.do("PATCH", "/api/competitors/"+c.ID, map[string]any{
		"nameFirst": "Someone", "nameLast": "Else",
	}, f.adminTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}

	rows := f.auditRows()
	if len(rows) != 1 {
		t.Fatalf("expected 1 audit row, got %d", len(rows))
	}
	if rows[0].EntityName != "Someone Else" {
		t.Fatalf("expected the known post-update name, got %q", rows[0].EntityName)
	}
	if strings.Contains(rows[0].DetailRaw, "Hopper") {
		t.Fatalf("detail unexpectedly carries the old name: %q", rows[0].DetailRaw)
	}
	t.Logf("FINDING S3: competitor.updated records only the new name %q; %q is gone from the trail",
		rows[0].EntityName, "Grace Hopper")
}

// TestAuditActionStringsFollowConvention enforces the documented
// `entity.action` shape across every action the API can emit. A filter UI keyed
// on these strings breaks silently when one drifts.
func TestAuditActionStringsFollowConvention(t *testing.T) {
	shape := regexp.MustCompile(`^[a-z]+(_[a-z]+)*\.[a-z]+(_[a-z]+)*$`)
	for _, c := range mutatingRouteCases() {
		if c.wantAction == "" {
			continue
		}
		if !shape.MatchString(c.wantAction) {
			t.Errorf("action %q does not match entity.action convention", c.wantAction)
		}
	}
}

// TestScheduleAuditEntityNameIsNotHumanReadable documents FINDING (S3): the
// schedule handlers put a raw UUID in entity_name, so the audit page renders an
// unreadable id where every other action shows a person's name. Asserted as-is
// so a future fix trips this test rather than passing unnoticed.
func TestScheduleAuditEntityNameIsNotHumanReadable(t *testing.T) {
	f := newAPIFixture(t)
	f.seedEvent("glr-2026", true)
	c := f.seedCompetitor("Grace", "Hopper")
	entry := f.seedSchedule(c.ID, "glr-2026")
	f.clearAudit()

	rec := f.do("PATCH", "/api/schedule/"+entry.ID, map[string]any{
		"instrument": "Cello", "scheduleDate": "2026-03-15T00:00:00Z",
		"scheduleTime": "12:00", "category": "Solo", "division": "Senior",
	}, f.adminTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("update schedule: %d %s", rec.Code, rec.Body.String())
	}

	rows := f.auditRows()
	if len(rows) != 1 {
		t.Fatalf("expected 1 audit row, got %d", len(rows))
	}
	// Current (undesired) behaviour: the competitor UUID, not "Grace Hopper".
	if rows[0].EntityName != c.ID {
		t.Fatalf("expected the known-bad UUID entity_name %q, got %q", c.ID, rows[0].EntityName)
	}
	t.Logf("FINDING S3: schedule.updated entity_name is the competitor UUID %q, not a readable name", rows[0].EntityName)
}

// TestScheduleDeleteAuditRecordsNothingAboutTheDeletedRow documents FINDING
// (S3): the deleted schedule slot is unrecoverable and the audit entry carries
// no competitor, event, instrument or time — the trail cannot answer "what was
// removed?".
func TestScheduleDeleteAuditRecordsNothingAboutTheDeletedRow(t *testing.T) {
	f := newAPIFixture(t)
	f.seedEvent("glr-2026", true)
	c := f.seedCompetitor("Grace", "Hopper")
	entry := f.seedSchedule(c.ID, "glr-2026")
	f.clearAudit()

	if rec := f.do("DELETE", "/api/schedule/"+entry.ID, nil, f.adminTok); rec.Code != http.StatusNoContent {
		t.Fatalf("delete schedule: %d %s", rec.Code, rec.Body.String())
	}
	rows := f.auditRows()
	if len(rows) != 1 {
		t.Fatalf("expected 1 audit row, got %d", len(rows))
	}
	if rows[0].DetailRaw != "{}" {
		t.Fatalf("expected the known-empty detail {}, got %q", rows[0].DetailRaw)
	}
	if rows[0].EntityName != entry.ID {
		t.Fatalf("expected entity_name to be the schedule id %q, got %q", entry.ID, rows[0].EntityName)
	}
	t.Logf("FINDING S3: schedule.deleted records neither the competitor, event, instrument nor slot time")
}

// TestContactUpdateAuditHasNoDetail documents FINDING (S3): note and email are
// staff-editable free text, but the audit entry records no before/after, so a
// note edit is untraceable beyond "someone touched contact info".
func TestContactUpdateAuditHasNoDetail(t *testing.T) {
	f := newAPIFixture(t)
	c := f.seedCompetitor("Grace", "Hopper")
	f.clearAudit()

	rec := f.do("PATCH", "/api/competitors/"+c.ID+"/contact", map[string]any{
		"note": "secret", "email": "new@example.com",
	}, f.adminTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("update contact: %d %s", rec.Code, rec.Body.String())
	}
	rows := f.auditRows()
	if len(rows) != 1 || rows[0].DetailRaw != "{}" {
		t.Fatalf("expected the known-empty detail {}, got %d rows / %q", len(rows), rows[0].DetailRaw)
	}
	t.Logf("FINDING S3: competitor.contact_updated carries no detail; which field changed is unrecorded")
}

// ---------------------------------------------------------------------------
// 3. Detail JSON round-trip through GET /api/audit.
// ---------------------------------------------------------------------------

func (f *apiFixture) listAudit(query string) []map[string]json.RawMessage {
	f.t.Helper()
	rec := f.do("GET", "/api/audit"+query, nil, f.adminTok)
	if rec.Code != http.StatusOK {
		f.t.Fatalf("GET /api/audit%s: %d %s", query, rec.Code, rec.Body.String())
	}
	var out []map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		f.t.Fatalf("decoding audit response %q: %v", rec.Body.String(), err)
	}
	return out
}

// TestAuditDetailIsNotDoubleEncoded: detail must arrive as a JSON object, not a
// JSON string containing escaped JSON, or every consumer has to parse twice.
func TestAuditDetailIsNotDoubleEncoded(t *testing.T) {
	f := newAPIFixture(t)
	f.seedEvent("glr-2026", true)
	c := f.seedCompetitor("Grace", "Hopper")
	f.clearAudit()

	if rec := f.do("PATCH", "/api/competitors/"+c.ID+"/checkin", nil, f.adminTok); rec.Code != http.StatusOK {
		t.Fatalf("checkin: %d", rec.Code)
	}

	entries := f.listAudit("")
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	raw := string(entries[0]["detail"])
	if strings.HasPrefix(raw, `"`) {
		t.Fatalf("detail is double-encoded (a JSON string): %s", raw)
	}
	var detail map[string]any
	if err := json.Unmarshal(entries[0]["detail"], &detail); err != nil {
		t.Fatalf("detail is not a JSON object: %s (%v)", raw, err)
	}
	if detail["eventId"] != "glr-2026" {
		t.Errorf("detail.eventId = %v, want glr-2026", detail["eventId"])
	}
	// DetailRaw must never leak as its own field alongside detail.
	if _, ok := entries[0]["DetailRaw"]; ok {
		t.Error("raw detail string leaked into the response")
	}
}

// TestAuditDetailNilAndCorruptStillProduceValidJSON: a nil detail and a row
// whose stored detail is not valid JSON must both serialise to a usable object
// rather than breaking the whole response.
func TestAuditDetailNilAndCorruptStillProduceValidJSON(t *testing.T) {
	f := newAPIFixture(t)
	c := f.seedCompetitor("Grace", "Hopper")
	f.clearAudit()

	// competitor.validated passes Detail: nil.
	if rec := f.do("PATCH", "/api/competitors/"+c.ID+"/validate", nil, f.adminTok); rec.Code != http.StatusOK {
		t.Fatalf("validate: %d", rec.Code)
	}
	// A row written outside the service (older code, manual SQL) with junk detail.
	corrupt := db.AuditLog{
		ID: uuid.New().String(), ActorID: f.admin.ID, ActorName: "Ada Admin",
		Action: "competitor.legacy", EntityType: "competitor", EntityID: c.ID,
		DetailRaw: "not json at all", CreatedAt: time.Now(),
	}
	if err := f.database.Create(&corrupt).Error; err != nil {
		t.Fatalf("seeding corrupt audit row: %v", err)
	}

	entries := f.listAudit("")
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	for _, entry := range entries {
		raw, ok := entry["detail"]
		if !ok {
			t.Fatal("entry has no detail field")
		}
		var out map[string]any
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Errorf("detail %q is not a valid JSON object: %v", raw, err)
		}
	}
}

// TestAuditDetailWithUnmarshalableValueDoesNotWriteJunk: Log falls back to {}
// when a detail value cannot be marshalled, so the row must still be readable.
func TestAuditDetailWithUnmarshalableValueDoesNotWriteJunk(t *testing.T) {
	f := newAPIFixture(t)
	f.clearAudit()

	f.audit.Log(service.LogEntry{
		ActorID: f.admin.ID, ActorName: "Ada Admin", Action: "competitor.updated",
		EntityType: "competitor", EntityID: "x", EntityName: "x",
		Detail: map[string]any{"bad": make(chan int)}, IP: testClientIP,
	})

	rows := f.auditRows()
	if len(rows) != 1 {
		t.Fatalf("expected the row to be written anyway, got %d", len(rows))
	}
	if !json.Valid([]byte(rows[0].DetailRaw)) {
		t.Fatalf("stored detail is not valid JSON: %q", rows[0].DetailRaw)
	}
	t.Logf("FINDING S4: unmarshalable detail is silently reduced to %q with no error surfaced", rows[0].DetailRaw)
}

// ---------------------------------------------------------------------------
// 4. Audit writes are fire-and-forget.
// ---------------------------------------------------------------------------

// TestAuditWriteFailureDoesNotFailRequest points the AuditService at a closed
// connection so every audit insert errors, then confirms the underlying
// operation still commits and still returns 2xx.
func TestAuditWriteFailureDoesNotFailRequest(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database-backed tests")
	}
	broken, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("opening broken handle: %v", err)
	}
	sqlDB, err := broken.DB()
	if err != nil {
		t.Fatalf("getting sql handle: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("closing handle: %v", err)
	}

	f := newAPIFixtureWith(t, service.NewAuditService(broken))
	f.seedEvent("glr-2026", true)
	c := f.seedCompetitor("Grace", "Hopper")
	f.clearAudit()

	rec := f.do("PATCH", "/api/competitors/"+c.ID+"/checkin", nil, f.adminTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("a failed audit write must not fail the request; got %d %s", rec.Code, rec.Body.String())
	}

	var ce db.CompetitorEvent
	if err := f.database.Where("competitor_id = ?", c.ID).First(&ce).Error; err != nil {
		t.Fatalf("check-in should still have committed: %v", err)
	}
	if !ce.CheckedIn {
		t.Error("competitor should be checked in despite the audit failure")
	}
	if rows := f.auditRows(); len(rows) != 0 {
		t.Errorf("expected the audit write to have failed, got %d rows", len(rows))
	}

	// The create path must behave the same way.
	rec = f.do("POST", "/api/competitors", map[string]any{"nameFirst": "New", "nameLast": "Person"}, f.adminTok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create must succeed despite audit failure; got %d %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// 5. GET /api/audit filters.
// ---------------------------------------------------------------------------

func (f *apiFixture) seedAuditRows(n int, action, actorName string) {
	f.t.Helper()
	rows := make([]db.AuditLog, n)
	base := time.Now().Add(-time.Duration(n) * time.Second)
	for i := range rows {
		rows[i] = db.AuditLog{
			ID: uuid.New().String(), ActorID: f.admin.ID, ActorName: actorName,
			Action: action, EntityType: "competitor", EntityID: fmt.Sprintf("e-%d", i),
			EntityName: fmt.Sprintf("Entity %d", i), DetailRaw: `{"i":` + fmt.Sprint(i) + `}`,
			IPAddress: testClientIP, CreatedAt: base.Add(time.Duration(i) * time.Second),
		}
	}
	if err := f.database.CreateInBatches(&rows, 100).Error; err != nil {
		f.t.Fatalf("seeding audit rows: %v", err)
	}
}

// TestAuditLimitIsCapped: limit is attacker-controlled on a table that grows
// forever, so an uncapped value is an availability problem.
func TestAuditLimitIsCapped(t *testing.T) {
	f := newAPIFixture(t)
	f.clearAudit()
	f.seedAuditRows(620, "competitor.checked_in", "Ada Admin")

	cases := []struct {
		query string
		want  int
	}{
		{"", 100},                  // no limit -> default
		{"?limit=0", 100},          // zero -> default
		{"?limit=-5", 100},         // negative -> default
		{"?limit=notanumber", 100}, // unparseable -> Atoi yields 0 -> default
		{"?limit=10", 10},
		{"?limit=500", 500},    // documented maximum
		{"?limit=501", 100},    // above the cap falls back to the default
		{"?limit=999999", 100}, // huge value must not attempt a full-table read
	}
	for _, c := range cases {
		got := len(f.listAudit(c.query))
		if got != c.want {
			t.Errorf("GET /api/audit%s returned %d rows, want %d", c.query, got, c.want)
		}
	}
}

// TestAuditFiltersRejectInjectionAndBogusValues: the filters are raw query
// strings from an admin UI; they must be parameterised and must degrade to an
// empty list rather than an error or a full dump.
func TestAuditFiltersRejectInjectionAndBogusValues(t *testing.T) {
	f := newAPIFixture(t)
	f.clearAudit()
	f.seedAuditRows(5, "competitor.checked_in", "Ada Admin")
	f.seedAuditRows(3, "staff.revoked", "Bob Registrar")

	if got := len(f.listAudit("?action=competitor.checked_in")); got != 5 {
		t.Errorf("action filter returned %d rows, want 5", got)
	}
	if got := len(f.listAudit("?actor=bob")); got != 3 {
		t.Errorf("actor filter is case-insensitive substring; got %d rows, want 3", got)
	}
	if got := len(f.listAudit("?action=does.not.exist")); got != 0 {
		t.Errorf("unknown action should return no rows, got %d", got)
	}
	if got := len(f.listAudit("?action=&actor=")); got != 8 {
		t.Errorf("empty filters should behave as absent; got %d rows, want 8", got)
	}

	injections := []string{
		"?action=" + urlEscape("' OR '1'='1"),
		"?actor=" + urlEscape("' OR 1=1 --"),
		"?action=" + urlEscape("x'; DROP TABLE audit_logs; --"),
		"?actor=" + urlEscape("%"),
	}
	for _, q := range injections {
		rec := f.do("GET", "/api/audit"+q, nil, f.adminTok)
		if rec.Code != http.StatusOK {
			t.Errorf("GET /api/audit%s returned %d %s", q, rec.Code, rec.Body.String())
		}
	}
	// The table must still be there and still hold every row.
	var count int64
	if err := f.database.Model(&db.AuditLog{}).Count(&count).Error; err != nil {
		t.Fatalf("audit_logs table is gone after injection attempts: %v", err)
	}
	if count != 8 {
		t.Errorf("audit_logs holds %d rows after injection attempts, want 8", count)
	}
	// A bare "%" in the actor filter is a LIKE wildcard, not an escape hatch out
	// of the parameterised query, but it does match everything.
	if got := len(f.listAudit("?actor=" + urlEscape("%"))); got == 8 {
		t.Logf("FINDING S4: actor=%% is passed straight into ILIKE, so it matches all %d rows"+
			" instead of searching for a literal percent sign", got)
	}
}

func urlEscape(s string) string {
	var b strings.Builder
	for _, r := range []byte(s) {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
			b.WriteByte(r)
			continue
		}
		fmt.Fprintf(&b, "%%%02X", r)
	}
	return b.String()
}

// TestAuditListIsAdminOnly: the audit log names staff and competitors, so it
// must never be readable by a registration token.
func TestAuditListIsAdminOnly(t *testing.T) {
	f := newAPIFixture(t)
	reg := f.seedStaff("Bob", "Registrar", "registration")

	rec := f.do("GET", "/api/audit", nil, reg.Token)
	if rec.Code != http.StatusForbidden {
		t.Errorf("registration staff got %d from GET /api/audit, want 403", rec.Code)
	}
}

// TestAuditListReturnsEmptyArrayNotNull: the admin UI maps over the response,
// so a JSON null instead of [] would break the page on a fresh database.
func TestAuditListReturnsEmptyArrayNotNull(t *testing.T) {
	f := newAPIFixture(t)
	f.clearAudit()

	rec := f.do("GET", "/api/audit", nil, f.adminTok)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/audit: %d", rec.Code)
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "[]" {
		t.Errorf("empty audit list serialised as %q, want []", got)
	}
}
