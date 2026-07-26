package main

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	authmw "johndpete316/agm-checkin-api/internal/middleware"
)

// TestIssuedTokensAreHighEntropyAndUnique checks the shape of what the login
// endpoint hands out: 32 random bytes hex-encoded to 64 characters, distinct on
// every call. A short or repeating token would make the bearer credential
// guessable, and there is no expiry to limit the window.
func TestIssuedTokensAreHighEntropyAndUnique(t *testing.T) {
	_, router := newAuthFixture(t, authmw.TrustedProxyDirect)

	const issues = 20
	seen := map[string]bool{}
	for i := 0; i < issues; i++ {
		// Vary the peer so the three-strike counter is irrelevant here.
		code, token := login(t, router, "192.0.2."+itoa(i+1)+":4000", testPIN, nil)
		if code != http.StatusCreated {
			t.Fatalf("login %d returned %d; want 201", i, code)
		}
		if len(token) != 64 {
			t.Errorf("token %q is %d chars; want 64 (32 random bytes hex-encoded)", token, len(token))
		}
		if _, err := hex.DecodeString(token); err != nil {
			t.Errorf("token %q is not hex: %v", token, err)
		}
		if seen[token] {
			t.Fatalf("token %q was issued twice", token)
		}
		seen[token] = true
	}
	if len(seen) != issues {
		t.Errorf("got %d distinct tokens from %d logins", len(seen), issues)
	}
}

// TestTokenIsNeverEchoedBackAfterCreation confirms the raw bearer token is
// returned only by the login response and never leaks from the staff-listing or
// "me" endpoints, which admins and the frontend poll routinely.
func TestTokenIsNeverEchoedBackAfterCreation(t *testing.T) {
	database, router := newAuthFixture(t, authmw.TrustedProxyDirect)

	adminToken := "1111111111111111111111111111111111111111111111111111111111111111"
	regToken := "2222222222222222222222222222222222222222222222222222222222222222"
	mintToken(t, database, "Ada", "Admin", "admin", adminToken)
	reg := mintToken(t, database, "Rex", "Reg", "registration", regToken)

	for _, probe := range []struct {
		name, path string
		headers    map[string]string
	}{
		{"me", "/api/auth/me", bearer(regToken)},
		{"staff list", "/api/staff", bearer(adminToken)},
	} {
		rec := request(t, router, http.MethodGet, probe.path, "192.0.2.9:4000", probe.headers, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s returned %d; want 200", probe.name, rec.Code)
		}
		body := rec.Body.String()
		for _, secret := range []string{adminToken, regToken} {
			if strings.Contains(body, secret) {
				t.Errorf("%s response leaked a raw bearer token", probe.name)
			}
		}
	}

	// The role-update response returns the model directly rather than a view;
	// make sure that path does not leak either.
	rec := request(t, router, http.MethodPatch, "/api/staff/"+reg.ID+"/role", "192.0.2.9:4000",
		bearer(adminToken), map[string]string{"role": "admin"})
	if rec.Code != http.StatusOK {
		t.Fatalf("role update returned %d; want 200", rec.Code)
	}
	if strings.Contains(rec.Body.String(), regToken) {
		t.Errorf("role-update response leaked the target's raw bearer token")
	}
}

// TestRevocationTakesEffectImmediately is the property the Manage Users page
// depends on: once an admin revokes a token, the very next request carrying it
// must fail. There is no cache or session layer, but this pins that.
func TestRevocationTakesEffectImmediately(t *testing.T) {
	database, router := newAuthFixture(t, authmw.TrustedProxyDirect)

	adminToken := "3333333333333333333333333333333333333333333333333333333333333333"
	regToken := "4444444444444444444444444444444444444444444444444444444444444444"
	mintToken(t, database, "Ada", "Admin", "admin", adminToken)
	reg := mintToken(t, database, "Rex", "Reg", "registration", regToken)

	const addr = "192.0.2.30:4000"

	// Before revocation the token works.
	if rec := request(t, router, http.MethodGet, "/api/auth/me", addr, bearer(regToken), nil); rec.Code != http.StatusOK {
		t.Fatalf("/api/auth/me before revocation returned %d; want 200", rec.Code)
	}
	if rec := request(t, router, http.MethodGet, "/api/competitors", addr, bearer(regToken), nil); rec.Code != http.StatusOK {
		t.Fatalf("/api/competitors before revocation returned %d; want 200", rec.Code)
	}

	rec := request(t, router, http.MethodDelete, "/api/staff/"+reg.ID, addr, bearer(adminToken), nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("revoke returned %d; want 204", rec.Code)
	}

	for _, path := range []string{"/api/auth/me", "/api/competitors", "/api/events"} {
		rec := request(t, router, http.MethodGet, path, addr, bearer(regToken), nil)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s with a revoked token returned %d; want 401", path, rec.Code)
		}
	}
}

// TestRoleChangeTakesEffectImmediately covers the other half of Manage Users:
// a demotion must close admin routes on the next request, without the demoted
// user needing to log in again.
func TestRoleChangeTakesEffectImmediately(t *testing.T) {
	database, router := newAuthFixture(t, authmw.TrustedProxyDirect)

	adminToken := "5555555555555555555555555555555555555555555555555555555555555555"
	targetToken := "6666666666666666666666666666666666666666666666666666666666666666"
	mintToken(t, database, "Ada", "Admin", "admin", adminToken)
	target := mintToken(t, database, "Tom", "Target", "registration", targetToken)

	const addr = "192.0.2.31:4000"

	if rec := request(t, router, http.MethodGet, "/api/staff", addr, bearer(targetToken), nil); rec.Code != http.StatusForbidden {
		t.Fatalf("registration token on /api/staff returned %d; want 403", rec.Code)
	}

	// Promote.
	rec := request(t, router, http.MethodPatch, "/api/staff/"+target.ID+"/role", addr,
		bearer(adminToken), map[string]string{"role": "admin"})
	if rec.Code != http.StatusOK {
		t.Fatalf("promotion returned %d; want 200", rec.Code)
	}
	if rec := request(t, router, http.MethodGet, "/api/staff", addr, bearer(targetToken), nil); rec.Code != http.StatusOK {
		t.Errorf("promoted token on /api/staff returned %d; want 200 on the next request", rec.Code)
	}

	// Demote.
	rec = request(t, router, http.MethodPatch, "/api/staff/"+target.ID+"/role", addr,
		bearer(adminToken), map[string]string{"role": "registration"})
	if rec.Code != http.StatusOK {
		t.Fatalf("demotion returned %d; want 200", rec.Code)
	}
	if rec := request(t, router, http.MethodGet, "/api/staff", addr, bearer(targetToken), nil); rec.Code != http.StatusForbidden {
		t.Errorf("demoted token on /api/staff returned %d; want 403 on the next request", rec.Code)
	}
}

// TestLoginNeverAssignsAdminRole pins that role is never client-controlled: the
// login endpoint takes a name and a PIN, and whatever name is supplied the new
// token comes back as "registration". Impersonating an existing admin's name
// must not inherit their role.
func TestLoginNeverAssignsAdminRole(t *testing.T) {
	database, router := newAuthFixture(t, authmw.TrustedProxyDirect)
	mintToken(t, database, "Test", "Staff", "admin",
		"7777777777777777777777777777777777777777777777777777777777777777")

	// login() sends firstName "Test", lastName "Staff" — the same name as the
	// admin minted above.
	code, token := login(t, router, "192.0.2.40:4000", testPIN, nil)
	if code != http.StatusCreated {
		t.Fatalf("login returned %d; want 201", code)
	}

	rec := request(t, router, http.MethodGet, "/api/auth/me", "192.0.2.40:4000", bearer(token), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("/api/auth/me returned %d; want 200", rec.Code)
	}
	var me struct {
		Role string `json:"role"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
		t.Fatalf("decoding me: %v", err)
	}
	if me.Role != "registration" {
		t.Errorf("a fresh login using an existing admin's name got role %q; want registration", me.Role)
	}

	if rec := request(t, router, http.MethodGet, "/api/staff", "192.0.2.40:4000", bearer(token), nil); rec.Code != http.StatusForbidden {
		t.Errorf("name-squatted token reached /api/staff with %d; want 403", rec.Code)
	}
}

// TestTokenValidationTimingIsNotAnObviousOracle compares the cost of validating
// an unknown token against a known one. The comparison itself is
// subtle.ConstantTimeCompare, but the lookup in front of it is an indexed
// database query, so this measures the whole path rather than the compare. The
// threshold is deliberately loose: the test is here to catch an order-of-
// magnitude difference, not to certify constant time.
func TestTokenValidationTimingIsNotAnObviousOracle(t *testing.T) {
	database, router := newAuthFixture(t, authmw.TrustedProxyDirect)

	known := "8888888888888888888888888888888888888888888888888888888888888888"
	unknown := "9999999999999999999999999999999999999999999999999999999999999999"
	mintToken(t, database, "Ada", "Admin", "admin", known)

	const iterations = 200
	measure := func(token string) time.Duration {
		// Warm the connection and query plan first.
		for i := 0; i < 20; i++ {
			request(t, router, http.MethodGet, "/api/auth/me", "192.0.2.50:4000", bearer(token), nil)
		}
		start := time.Now()
		for i := 0; i < iterations; i++ {
			request(t, router, http.MethodGet, "/api/auth/me", "192.0.2.50:4000", bearer(token), nil)
		}
		return time.Since(start) / iterations
	}

	knownAvg := measure(known)
	unknownAvg := measure(unknown)
	t.Logf("mean validation latency: known=%v unknown=%v", knownAvg, unknownAvg)

	ratio := float64(knownAvg) / float64(unknownAvg)
	if ratio < 1 {
		ratio = 1 / ratio
	}
	if ratio > 10 {
		t.Errorf("known and unknown token validation differ by %.1fx (known=%v unknown=%v); "+
			"that is an exploitable oracle for token existence", ratio, knownAvg, unknownAvg)
	}
}

// TestDatabaseOutageFailsClosedWithForbidden is a characterization test for an
// OPEN finding, not an assertion that the behaviour is correct.
//
// AuthService.IsIPBlocked returns true on any database error, and the global
// IPBlocklist middleware turns that into a blanket 403. Failing closed on the
// authenticated API is defensible; doing it on /health is not, because the
// probe then answers "403 access denied" for a database outage. An orchestrator
// reading 403 as a live-but-refusing service will keep the pod in rotation
// instead of restarting or draining it, and the status gives an operator no
// hint that the database is the problem. A 503 from /health would say both.
//
// If this test starts failing, the behaviour has been corrected — delete it.
func TestDatabaseOutageFailsClosedWithForbidden(t *testing.T) {
	dsn := testDSN(t)

	broken, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	sqlDB, err := broken.DB()
	if err != nil {
		t.Fatalf("getting sql handle: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("closing pool: %v", err)
	}

	router := newRouter(routerConfig{
		database:      broken,
		pin:           testPIN,
		trustedProxy:  authmw.TrustedProxyDirect,
		allowedOrigin: "http://localhost:5173",
	})

	rec := request(t, router, http.MethodGet, "/health", "192.0.2.60:4000", nil, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("/health returned %d with the database unreachable; expected 403 — "+
			"if this is now a 5xx the finding has been fixed and this test should be deleted", rec.Code)
	}
	t.Logf("/health with an unreachable database: %d %s", rec.Code, rec.Body.String())
}
