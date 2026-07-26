package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"johndpete316/agm-checkin-api/internal/db"
	authmw "johndpete316/agm-checkin-api/internal/middleware"
)

// testPIN is the access code every fixture router is built with.
const testPIN = "1234"

// httpTestDSN resolves once per process to a database dedicated to this package.
//
// It cannot share TEST_DATABASE_URL with internal/service: the migration tests
// there run `DROP SCHEMA public CASCADE`, and `go test ./...` builds packages in
// parallel, so a shared database means those drops land underneath these tests.
// Deriving a sibling database keeps both suites green under plain `go test ./...`
// with no -p 1 required.
var (
	httpTestDSNOnce sync.Once
	httpTestDSN     string
	httpTestDSNErr  error
)

func testDSN(t *testing.T) string {
	t.Helper()

	base := os.Getenv("TEST_DATABASE_URL")
	if base == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping database-backed tests")
	}

	httpTestDSNOnce.Do(func() {
		httpTestDSN, httpTestDSNErr = provisionSiblingDatabase(base, "_httpapi")
	})
	if httpTestDSNErr != nil {
		t.Fatalf("provisioning the http test database: %v", httpTestDSNErr)
	}
	return httpTestDSN
}

// provisionSiblingDatabase creates "<dbname><suffix>" next to the database in
// dsn and returns a DSN pointing at it. An existing database is reused.
func provisionSiblingDatabase(dsn, suffix string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("parsing TEST_DATABASE_URL: %w", err)
	}
	name := strings.TrimPrefix(parsed.Path, "/")
	if name == "" {
		return "", fmt.Errorf("TEST_DATABASE_URL has no database name")
	}
	sibling := name + suffix

	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return "", fmt.Errorf("connecting to %s: %w", name, err)
	}
	defer func() {
		if sqlDB, err := admin.DB(); err == nil {
			sqlDB.Close()
		}
	}()

	// CREATE DATABASE cannot be parameterised, and the name is derived from the
	// operator's own DSN rather than from any request input.
	if err := admin.Exec(`CREATE DATABASE "` + sibling + `"`).Error; err != nil {
		// 42P04 is duplicate_database, which is the steady state after the
		// first run and not an error.
		if !strings.Contains(err.Error(), "already exists") {
			return "", fmt.Errorf("creating %s: %w", sibling, err)
		}
	}

	parsed.Path = "/" + sibling
	return parsed.String(), nil
}

// newAuthFixture returns a clean database plus the real chi router, built through
// the same newRouter used by main so the middleware chain under test is the one
// that ships. Set TEST_DATABASE_URL to a scratch database — never point it at a
// database you care about, since each test truncates.
func newAuthFixture(t *testing.T, mode authmw.TrustedProxy) (*gorm.DB, *chi.Mux) {
	t.Helper()

	dsn := testDSN(t)

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
		          audit_logs, ip_blocklists, pin_attempts, staff_tokens
		 RESTART IDENTITY CASCADE`,
	).Error; err != nil {
		t.Fatalf("truncating: %v", err)
	}

	router := newRouter(routerConfig{
		database:      database,
		pin:           testPIN,
		trustedProxy:  mode,
		allowedOrigin: "http://localhost:5173",
	})
	return database, router
}

// mintToken creates a staff token directly in the database with the given role,
// bypassing the login flow so role setup does not depend on the code under test.
func mintToken(t *testing.T, database *gorm.DB, first, last, role, raw string) db.StaffToken {
	t.Helper()
	tok := db.StaffToken{
		ID:        uuidLike(raw),
		Token:     raw,
		FirstName: first,
		LastName:  last,
		Role:      role,
		CreatedAt: time.Now(),
	}
	if err := database.Create(&tok).Error; err != nil {
		t.Fatalf("minting %s token: %v", role, err)
	}
	return tok
}

// uuidLike derives a stable UUID-shaped string from a token so tests can predict
// staff IDs without threading them through every helper.
func uuidLike(seed string) string {
	h := "00000000000000000000000000000000" + seed
	h = h[len(h)-32:]
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

// request issues a request against the router. remoteAddr and headers are applied
// verbatim so client-IP resolution can be exercised.
func request(t *testing.T, router *chi.Mux, method, path, remoteAddr string, headers map[string]string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshalling body: %v", err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}

	req := httptest.NewRequest(method, path, reader)
	if remoteAddr != "" {
		req.RemoteAddr = remoteAddr
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// bearer builds an Authorization header map for a raw token.
func bearer(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

// login drives the real POST /api/auth/token endpoint and returns the status
// code plus the token string (empty when the call did not succeed).
func login(t *testing.T, router *chi.Mux, remoteAddr, pin string, headers map[string]string) (int, string) {
	t.Helper()
	rec := request(t, router, http.MethodPost, "/api/auth/token", remoteAddr, headers, map[string]string{
		"code":      pin,
		"firstName": "Test",
		"lastName":  "Staff",
	})
	if rec.Code != http.StatusCreated {
		return rec.Code, ""
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decoding token response: %v", err)
	}
	return rec.Code, out.Token
}

// blockedIPs returns every IP currently on the blocklist.
func blockedIPs(t *testing.T, database *gorm.DB) []string {
	t.Helper()
	var rows []db.IPBlocklist
	if err := database.Find(&rows).Error; err != nil {
		t.Fatalf("reading blocklist: %v", err)
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.IPAddress
	}
	return out
}

// attemptCount returns how many failed-PIN rows are recorded for an IP.
func attemptCount(t *testing.T, database *gorm.DB, ip string) int64 {
	t.Helper()
	var n int64
	if err := database.Model(&db.PINAttempt{}).Where("ip_address = ?", ip).Count(&n).Error; err != nil {
		t.Fatalf("counting attempts: %v", err)
	}
	return n
}
