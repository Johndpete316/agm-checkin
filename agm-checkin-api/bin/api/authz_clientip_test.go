package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	authmw "johndpete316/agm-checkin-api/internal/middleware"
)

// TestGetClientIPWithMode pins the parsing rules for both trust modes. These are
// unit-level: no database, no router, just the resolver.
func TestGetClientIPWithMode(t *testing.T) {
	cases := []struct {
		name       string
		mode       authmw.TrustedProxy
		remoteAddr string
		headers    map[string]string
		want       string
	}{
		{
			name:       "cloudflare prefers CF-Connecting-IP",
			mode:       authmw.TrustedProxyCloudflare,
			remoteAddr: "10.0.0.1:1234",
			headers:    map[string]string{"CF-Connecting-IP": "203.0.113.5", "X-Forwarded-For": "198.51.100.7"},
			want:       "203.0.113.5",
		},
		{
			// X-Forwarded-For is no longer a fallback: cloudflared always sets
			// CF-Connecting-IP, so a request reaching this branch did not come
			// through the tunnel and its headers cannot be trusted.
			name:       "cloudflare ignores multi-hop XFF and uses the real peer",
			mode:       authmw.TrustedProxyCloudflare,
			remoteAddr: "10.0.0.1:1234",
			headers:    map[string]string{"X-Forwarded-For": "198.51.100.7, 10.0.0.9, 172.16.0.3"},
			want:       "10.0.0.1",
		},
		{
			name:       "cloudflare ignores a single-hop XFF",
			mode:       authmw.TrustedProxyCloudflare,
			remoteAddr: "10.0.0.1:1234",
			headers:    map[string]string{"X-Forwarded-For": "198.51.100.7"},
			want:       "10.0.0.1",
		},
		{
			// Cloudflare only ever sends a literal IP, so an unparseable value
			// is not from Cloudflare and must not reach the IP columns.
			name:       "cloudflare rejects a non-IP CF header",
			mode:       authmw.TrustedProxyCloudflare,
			remoteAddr: "10.0.0.1:1234",
			headers:    map[string]string{"CF-Connecting-IP": "not-an-ip"},
			want:       "10.0.0.1",
		},
		{
			name:       "cloudflare rejects a CF header carrying a port",
			mode:       authmw.TrustedProxyCloudflare,
			remoteAddr: "10.0.0.1:1234",
			headers:    map[string]string{"CF-Connecting-IP": "203.0.113.5:443"},
			want:       "10.0.0.1",
		},
		{
			name:       "cloudflare trims whitespace in CF header",
			mode:       authmw.TrustedProxyCloudflare,
			remoteAddr: "10.0.0.1:1234",
			headers:    map[string]string{"CF-Connecting-IP": "  203.0.113.5  "},
			want:       "203.0.113.5",
		},
		{
			name:       "cloudflare with no headers uses RemoteAddr host",
			mode:       authmw.TrustedProxyCloudflare,
			remoteAddr: "192.0.2.44:9999",
			want:       "192.0.2.44",
		},
		{
			name:       "direct ignores CF-Connecting-IP",
			mode:       authmw.TrustedProxyDirect,
			remoteAddr: "192.0.2.44:9999",
			headers:    map[string]string{"CF-Connecting-IP": "203.0.113.5"},
			want:       "192.0.2.44",
		},
		{
			name:       "direct ignores X-Forwarded-For",
			mode:       authmw.TrustedProxyDirect,
			remoteAddr: "192.0.2.44:9999",
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.5"},
			want:       "192.0.2.44",
		},
		{
			name:       "IPv6 literal RemoteAddr loses its brackets and port",
			mode:       authmw.TrustedProxyDirect,
			remoteAddr: "[2001:db8::1]:443",
			want:       "2001:db8::1",
		},
		{
			name:       "IPv6 loopback RemoteAddr",
			mode:       authmw.TrustedProxyDirect,
			remoteAddr: "[::1]:53124",
			want:       "::1",
		},
		{
			name:       "RemoteAddr without a port is returned verbatim",
			mode:       authmw.TrustedProxyDirect,
			remoteAddr: "192.0.2.44",
			want:       "192.0.2.44",
		},
		{
			name:       "IPv6 in CF-Connecting-IP passes through unbracketed",
			mode:       authmw.TrustedProxyCloudflare,
			remoteAddr: "10.0.0.1:1234",
			headers:    map[string]string{"CF-Connecting-IP": "2001:db8::99"},
			want:       "2001:db8::99",
		},
		{
			name:       "empty CF header falls through to the real peer, not XFF",
			mode:       authmw.TrustedProxyCloudflare,
			remoteAddr: "10.0.0.1:1234",
			headers:    map[string]string{"CF-Connecting-IP": "", "X-Forwarded-For": "198.51.100.7"},
			want:       "10.0.0.1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/health", nil)
			req.RemoteAddr = tc.remoteAddr
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			if got := authmw.GetClientIPWithMode(req, tc.mode); got != tc.want {
				t.Errorf("GetClientIPWithMode(%q, %q) = %q; want %q",
					tc.remoteAddr, tc.mode, got, tc.want)
			}
		})
	}
}

// TestClientIPHeaderJunkIsRejected covers the header-validation fix. Anything
// that is not a literal IP did not come from Cloudflare, and must not be
// written into ip_blocklists, pin_attempts, or audit_logs.ip_address — all of
// which are unbounded text columns.
func TestClientIPHeaderJunkIsRejected(t *testing.T) {
	junk := []string{
		"not-an-ip",
		"'; DROP TABLE staff_tokens; --",
		strings.Repeat("A", 4096),
		"127.0.0.1 evil",
		"127.0.0.1, 10.0.0.1",
		"999.999.999.999",
		"[2001:db8::1]",
	}
	for _, v := range junk {
		t.Run(v[:min(len(v), 24)], func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/health", nil)
			req.RemoteAddr = "10.0.0.1:1234"
			req.Header.Set("CF-Connecting-IP", v)
			if got := authmw.GetClientIPWithMode(req, authmw.TrustedProxyCloudflare); got != "10.0.0.1" {
				t.Errorf("CF-Connecting-IP %q resolved to %q; want the real peer 10.0.0.1", v, got)
			}
		})
	}
}

// TestDirectModeBlocklistCannotBeEvaded is the control for the spoofing tests
// below: with TRUSTED_PROXY=direct the headers are inert, so a blocked IP stays
// blocked no matter what it claims to be.
func TestDirectModeBlocklistCannotBeEvaded(t *testing.T) {
	_, router := newAuthFixture(t, authmw.TrustedProxyDirect)
	const addr = "203.0.113.20:4000"

	for i := 0; i < 3; i++ {
		login(t, router, addr, "wrong", nil)
	}

	spoofs := []map[string]string{
		{"CF-Connecting-IP": "198.51.100.1"},
		{"X-Forwarded-For": "198.51.100.1"},
		{"CF-Connecting-IP": "198.51.100.1", "X-Forwarded-For": "198.51.100.2"},
	}
	for _, h := range spoofs {
		rec := request(t, router, http.MethodGet, "/health", addr, h, nil)
		if rec.Code != http.StatusForbidden {
			t.Errorf("direct mode let a blocked IP through with headers %v (status %d); want 403", h, rec.Code)
		}
	}
}

// TestCloudflareModeXFFNoLongerSpoofable covers the half of the spoofing
// problem that is fixed. X-Forwarded-For is inert in cloudflare mode, so a
// blocked peer cannot shed its block by inventing a forwarding chain.
func TestCloudflareModeXFFNoLongerSpoofable(t *testing.T) {
	database, router := newAuthFixture(t, authmw.TrustedProxyCloudflare)
	const addr = "203.0.113.21:4000"
	const ip = "203.0.113.21"

	for i := 0; i < 3; i++ {
		login(t, router, addr, "wrong", nil)
	}
	if got := blockedIPs(t, database); len(got) != 1 || got[0] != ip {
		t.Fatalf("blocklist is %v; want [%s]", got, ip)
	}

	for _, h := range []map[string]string{
		{"X-Forwarded-For": "198.51.100.78"},
		{"X-Forwarded-For": "198.51.100.78, 10.0.0.1"},
		{"CF-Connecting-IP": "not-an-ip"},
		{"CF-Connecting-IP": "", "X-Forwarded-For": "198.51.100.79"},
	} {
		rec := request(t, router, http.MethodGet, "/health", addr, h, nil)
		if rec.Code != http.StatusForbidden {
			t.Errorf("blocked peer %s reached /health with headers %v (status %d); want 403",
				addr, h, rec.Code)
		}
	}
}

// TestCloudflareModeCFHeaderIsStillSpoofable is a characterization test for an
// OPEN finding, not an assertion that the behaviour is correct.
//
// In cloudflare mode the client IP is taken from CF-Connecting-IP with no check
// that the request actually arrived from Cloudflare. Anyone who can reach the
// listener directly — bypassing the tunnel — sets the header themselves, and
// the three-strike blocklist stops applying to them: rotate the claimed IP and
// the PIN can be brute-forced indefinitely.
//
// Closing this needs the peer address checked against Cloudflare's published
// IP ranges (or an equivalent shared secret), which is deployment
// configuration this branch cannot verify. Until then the only control is
// network-level: the listener must be unreachable except through the tunnel.
//
// If this test starts failing, the hole has been closed — delete it.
func TestCloudflareModeCFHeaderIsStillSpoofable(t *testing.T) {
	database, router := newAuthFixture(t, authmw.TrustedProxyCloudflare)
	const addr = "203.0.113.22:4000"

	const guesses = 25
	for i := 0; i < guesses; i++ {
		code, _ := login(t, router, addr, "wrong", map[string]string{"CF-Connecting-IP": spoofedIP(i)})
		if code != http.StatusUnauthorized {
			t.Fatalf("guess %d returned %d; want 401 — expected the attacker to be unthrottled", i, code)
		}
	}

	code, token := login(t, router, addr, testPIN, map[string]string{"CF-Connecting-IP": spoofedIP(guesses)})
	if code != http.StatusCreated || token == "" {
		t.Fatalf("login after %d spoofed failures returned %d; expected 201 — "+
			"if this now fails, CF-Connecting-IP spoofing has been fixed and this test should be deleted",
			guesses, code)
	}

	// The real TCP peer was never blocked despite 25 failures from it.
	for _, ip := range blockedIPs(t, database) {
		if ip == "203.0.113.22" {
			t.Fatalf("the real peer was blocked; the spoofing hole appears closed — update this test")
		}
	}
}

// TestCloudflareModeCanBlockAnInnocentIP is a characterization test for the
// denial-of-service direction of the same OPEN finding. The attacker never has
// to guess the PIN — they only have to name a victim. Three spoofed requests
// permanently block whatever IP they claim, and because the blocklist
// middleware is global the victim then loses every route, including /health and
// including routes they hold a valid admin token for.
//
// If this test starts failing, the hole has been closed — delete it.
func TestCloudflareModeCanBlockAnInnocentIP(t *testing.T) {
	database, router := newAuthFixture(t, authmw.TrustedProxyCloudflare)
	const attacker = "203.0.113.23:4000"
	const victim = "198.51.100.200"

	for i := 0; i < 3; i++ {
		login(t, router, attacker, "wrong", map[string]string{"CF-Connecting-IP": victim})
	}

	poisoned := false
	for _, ip := range blockedIPs(t, database) {
		if ip == victim {
			poisoned = true
		}
	}
	if !poisoned {
		t.Fatalf("attacker at %s could not block %s; the spoofing hole appears closed — update this test",
			attacker, victim)
	}

	victimToken := "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"
	mintToken(t, database, "Vic", "Tim", "admin", victimToken)

	// The victim is locked out of the authenticated API...
	rec := request(t, router, http.MethodGet, "/api/staff", victim+":6000", map[string]string{
		"CF-Connecting-IP": victim,
		"Authorization":    "Bearer " + victimToken,
	}, nil)
	if rec.Code != http.StatusForbidden {
		t.Errorf("victim holding a valid admin token got %d on /api/staff; expected 403", rec.Code)
	}

	// ...and out of the unauthenticated liveness probe, because IPBlocklist is
	// registered above /health.
	rec = request(t, router, http.MethodGet, "/health", victim+":6000",
		map[string]string{"CF-Connecting-IP": victim}, nil)
	if rec.Code != http.StatusForbidden {
		t.Errorf("/health returned %d for the spoof-blocked victim; expected 403 "+
			"(the blocklist gates the liveness probe too)", rec.Code)
	}
}

// spoofedIP produces a distinct claimed client IP per iteration.
func spoofedIP(i int) string {
	return "198.51.100." + itoa(i%250+1)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [8]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}
