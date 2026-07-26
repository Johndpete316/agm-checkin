package main

import (
	"net/http"
	"testing"

	authmw "johndpete316/agm-checkin-api/internal/middleware"
)

// TestBlocklistThresholdIsExactlyThree pins the lockout threshold. The first two
// bad PINs must be recoverable (401) and the third must both fail and block, so
// a staff member gets two retries and no more.
func TestBlocklistThresholdIsExactlyThree(t *testing.T) {
	database, router := newAuthFixture(t, authmw.TrustedProxyDirect)
	const addr = "203.0.113.10:4000"
	const ip = "203.0.113.10"

	for attempt := 1; attempt <= 2; attempt++ {
		code, _ := login(t, router, addr, "wrong", nil)
		if code != http.StatusUnauthorized {
			t.Fatalf("attempt %d returned %d; want 401", attempt, code)
		}
		if got := blockedIPs(t, database); len(got) != 0 {
			t.Fatalf("attempt %d blocked %v; want no block before the third failure", attempt, got)
		}
	}

	code, _ := login(t, router, addr, "wrong", nil)
	if code != http.StatusForbidden {
		t.Fatalf("third failed attempt returned %d; want 403", code)
	}
	if got := blockedIPs(t, database); len(got) != 1 || got[0] != ip {
		t.Fatalf("blocklist is %v; want exactly [%s]", got, ip)
	}

	// A correct PIN after the block must not let the caller back in.
	if code, _ := login(t, router, addr, testPIN, nil); code != http.StatusForbidden {
		t.Errorf("correct PIN from a blocked IP returned %d; want 403", code)
	}
}

// TestBlockPersistsAcrossRouterInstances proves the block lives in the database
// rather than in per-process memory, so a restart or a second replica does not
// clear it.
func TestBlockPersistsAcrossRouterInstances(t *testing.T) {
	database, router := newAuthFixture(t, authmw.TrustedProxyDirect)
	const addr = "203.0.113.11:4000"

	for i := 0; i < 3; i++ {
		login(t, router, addr, "wrong", nil)
	}
	if got := blockedIPs(t, database); len(got) != 1 {
		t.Fatalf("blocklist is %v; want one entry", got)
	}

	fresh := newRouter(routerConfig{
		database:      database,
		pin:           testPIN,
		trustedProxy:  authmw.TrustedProxyDirect,
		allowedOrigin: "http://localhost:5173",
	})
	if code, _ := login(t, fresh, addr, testPIN, nil); code != http.StatusForbidden {
		t.Errorf("a fresh router let a blocked IP log in with %d; want 403", code)
	}
}

// TestBlockedIPIsRefusedOnEveryRoute covers the global placement of the
// IPBlocklist middleware: a blocked IP must not reach any endpoint, including
// /health and including routes it holds a valid token for.
func TestBlockedIPIsRefusedOnEveryRoute(t *testing.T) {
	database, router := newAuthFixture(t, authmw.TrustedProxyDirect)
	const addr = "203.0.113.12:4000"

	adminToken := "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	mintToken(t, database, "Ada", "Admin", "admin", adminToken)

	// Sanity: before the block, both a public and a protected route work.
	if rec := request(t, router, http.MethodGet, "/health", addr, nil, nil); rec.Code != http.StatusOK {
		t.Fatalf("/health before block returned %d; want 200", rec.Code)
	}
	if rec := request(t, router, http.MethodGet, "/api/staff", addr, bearer(adminToken), nil); rec.Code != http.StatusOK {
		t.Fatalf("/api/staff before block returned %d; want 200", rec.Code)
	}

	for i := 0; i < 3; i++ {
		login(t, router, addr, "wrong", nil)
	}

	for _, rt := range apiRoutes() {
		t.Run(rt.method+"_"+rt.path, func(t *testing.T) {
			rec := request(t, router, rt.method, rt.path, addr, bearer(adminToken), rt.body)
			if rec.Code != http.StatusForbidden {
				t.Errorf("%s %s returned %d for a blocked IP; want 403", rt.method, rt.path, rec.Code)
			}
		})
	}
}

// TestSuccessfulLoginResetsFailedAttemptCounter is the behaviour a lockout
// counter is expected to have: proving you know the PIN clears the strikes
// against you. Without a reset the attempt rows are permanent, so a staff
// member who mistypes twice on day one is one typo away — forever — from
// permanently blocking the IP the whole registration desk shares.
func TestSuccessfulLoginResetsFailedAttemptCounter(t *testing.T) {
	database, router := newAuthFixture(t, authmw.TrustedProxyDirect)
	const addr = "203.0.113.13:4000"
	const ip = "203.0.113.13"

	for i := 0; i < 2; i++ {
		if code, _ := login(t, router, addr, "wrong", nil); code != http.StatusUnauthorized {
			t.Fatalf("warm-up failure %d returned %d; want 401", i+1, code)
		}
	}

	if code, token := login(t, router, addr, testPIN, nil); code != http.StatusCreated || token == "" {
		t.Fatalf("correct PIN after two failures returned %d; want 201 with a token", code)
	}

	if n := attemptCount(t, database, ip); n != 0 {
		t.Errorf("after a successful login the IP still has %d recorded failures; want 0", n)
	}

	// The consequence of not resetting: a single later typo blocks the desk.
	code, _ := login(t, router, addr, "wrong", nil)
	if code == http.StatusForbidden {
		t.Errorf("one failure after a successful login blocked the IP (403); "+
			"want 401 with the counter restarted, blocklist=%v", blockedIPs(t, database))
	}
}

// TestFailedAttemptsAreNotPrunedAfterBlock records that pin_attempts rows are
// written on every failure and never removed. Nothing reads them once the IP is
// blocked, so the table only grows — and an attacker who can vary the client IP
// (see the spoofing tests) can grow it without bound.
func TestFailedAttemptsAreNotPrunedAfterBlock(t *testing.T) {
	database, router := newAuthFixture(t, authmw.TrustedProxyDirect)
	const addr = "203.0.113.14:4000"
	const ip = "203.0.113.14"

	for i := 0; i < 3; i++ {
		login(t, router, addr, "wrong", nil)
	}
	if n := attemptCount(t, database, ip); n != 3 {
		t.Fatalf("recorded %d attempts; want 3", n)
	}

	// Further attempts short-circuit on the blocklist and add no rows, so the
	// table growth is bounded per IP — but only per IP.
	for i := 0; i < 5; i++ {
		login(t, router, addr, "wrong", nil)
	}
	if n := attemptCount(t, database, ip); n != 3 {
		t.Errorf("recorded %d attempts after post-block tries; want 3 (blocked IPs must short-circuit)", n)
	}
}
