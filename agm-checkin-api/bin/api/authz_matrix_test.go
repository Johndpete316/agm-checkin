package main

import (
	"net/http"
	"testing"

	authmw "johndpete316/agm-checkin-api/internal/middleware"
)

// authLevel is the access class a route is expected to enforce.
type authLevel int

const (
	// levelPublic: reachable with no Authorization header at all.
	levelPublic authLevel = iota
	// levelToken: any valid staff token, regardless of role.
	levelToken
	// levelAdmin: only tokens whose role is "admin".
	levelAdmin
)

// route is one entry in the API's route table. body is sent for methods that
// take one so handlers reach their real logic rather than failing to decode.
type route struct {
	method string
	path   string
	level  authLevel
	body   any
}

// apiRoutes mirrors the route table registered in newRouter. Every route
// registered there must appear here — the matrix test is the guard that new
// routes get their auth class asserted rather than silently inheriting one.
//
// Path params use IDs that do not exist, so an authorised call lands on a
// 4xx/5xx from the handler rather than mutating fixture state. The matrix only
// asserts the auth outcome (401/403 vs anything else), never the handler's own
// status, so a not-found result is a pass.
func apiRoutes() []route {
	const missing = "00000000-0000-0000-0000-0000000000ff"

	return []route{
		{http.MethodGet, "/health", levelPublic, nil},
		{http.MethodPost, "/api/auth/token", levelPublic, map[string]string{
			"code": testPIN, "firstName": "Matrix", "lastName": "Probe",
		}},

		{http.MethodGet, "/api/auth/me", levelToken, nil},
		{http.MethodGet, "/api/competitors", levelToken, nil},
		{http.MethodGet, "/api/competitors/" + missing, levelToken, nil},
		{http.MethodPost, "/api/competitors", levelToken, map[string]string{
			"nameFirst": "Matrix", "nameLast": "Probe",
		}},
		{http.MethodPatch, "/api/competitors/" + missing + "/checkin", levelToken, nil},
		{http.MethodPatch, "/api/competitors/" + missing + "/contact", levelToken, map[string]string{"note": "n"}},
		{http.MethodPatch, "/api/competitors/" + missing + "/dob", levelToken, map[string]string{
			"dateOfBirth": "2005-03-15T00:00:00Z",
		}},
		{http.MethodPatch, "/api/competitors/" + missing + "/validate", levelToken, nil},
		{http.MethodDelete, "/api/competitors/" + missing, levelToken, nil},
		{http.MethodGet, "/api/competitors/" + missing + "/events", levelToken, nil},
		{http.MethodGet, "/api/competitors/" + missing + "/schedule", levelToken, nil},
		{http.MethodGet, "/api/events", levelToken, nil},
		{http.MethodGet, "/api/events/current", levelToken, nil},

		{http.MethodPatch, "/api/competitors/" + missing, levelAdmin, map[string]string{"nameFirst": "X"}},
		{http.MethodPost, "/api/events", levelAdmin, map[string]string{
			"id": "matrix-probe", "name": "Matrix Probe",
		}},
		{http.MethodPatch, "/api/events/matrix-missing/current", levelAdmin, nil},
		{http.MethodGet, "/api/staff", levelAdmin, nil},
		{http.MethodPatch, "/api/staff/" + missing + "/role", levelAdmin, map[string]string{"role": "admin"}},
		{http.MethodDelete, "/api/staff/" + missing, levelAdmin, nil},
		{http.MethodGet, "/api/audit", levelAdmin, nil},
		{http.MethodPost, "/api/competitors/import", levelAdmin, nil},
		{http.MethodPost, "/api/competitors/" + missing + "/schedule/import", levelAdmin, nil},
		{http.MethodPost, "/api/competitors/" + missing + "/schedule", levelAdmin, map[string]string{"title": "t"}},
		{http.MethodPatch, "/api/schedule/" + missing, levelAdmin, map[string]string{"title": "t"}},
		{http.MethodDelete, "/api/schedule/" + missing, levelAdmin, nil},
	}
}

// TestRouteIdentityMatrix walks every route against every identity class and
// asserts the auth outcome. Handler-level statuses are deliberately ignored:
// the only thing under test is whether the request was authenticated and
// authorised, not what the handler then did with it.
func TestRouteIdentityMatrix(t *testing.T) {
	database, router := newAuthFixture(t, authmw.TrustedProxyDirect)

	adminToken := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	regToken := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	revokedToken := "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"

	mintToken(t, database, "Ada", "Admin", "admin", adminToken)
	mintToken(t, database, "Rex", "Reg", "registration", regToken)
	revoked := mintToken(t, database, "Rev", "Oked", "registration", revokedToken)
	if err := database.Delete(&revoked).Error; err != nil {
		t.Fatalf("revoking token: %v", err)
	}

	identities := []struct {
		name string
		// headers to send; nil means send no Authorization header at all.
		headers map[string]string
		// authenticated is true only for identities that map to a live token.
		authenticated bool
		admin         bool
	}{
		{name: "no-header", headers: nil},
		{name: "empty-header", headers: map[string]string{"Authorization": ""}},
		{name: "bare-Bearer", headers: map[string]string{"Authorization": "Bearer"}},
		{name: "Bearer-empty-token", headers: map[string]string{"Authorization": "Bearer "}},
		{name: "lowercase-bearer", headers: map[string]string{"Authorization": "bearer " + adminToken}},
		{name: "basic-auth", headers: map[string]string{"Authorization": "Basic YWRtaW46YWRtaW4="}},
		{name: "token-without-scheme", headers: map[string]string{"Authorization": adminToken}},
		{name: "unknown-token", headers: bearer("deadbeef")},
		{name: "revoked-token", headers: bearer(revokedToken)},
		{name: "registration-token", headers: bearer(regToken), authenticated: true},
		{name: "admin-token", headers: bearer(adminToken), authenticated: true, admin: true},
	}

	for _, rt := range apiRoutes() {
		for _, id := range identities {
			name := rt.method + "_" + rt.path + "_" + id.name
			t.Run(name, func(t *testing.T) {
				rec := request(t, router, rt.method, rt.path, "203.0.113.9:5000", id.headers, rt.body)
				got := rec.Code

				switch rt.level {
				case levelPublic:
					if got == http.StatusUnauthorized || got == http.StatusForbidden {
						t.Errorf("public route %s %s returned %d for %s; want it reachable",
							rt.method, rt.path, got, id.name)
					}
				case levelToken:
					if id.authenticated {
						if got == http.StatusUnauthorized || got == http.StatusForbidden {
							t.Errorf("token route %s %s rejected %s with %d; want it allowed through auth",
								rt.method, rt.path, id.name, got)
						}
					} else if got != http.StatusUnauthorized {
						t.Errorf("token route %s %s returned %d for %s; want 401",
							rt.method, rt.path, got, id.name)
					}
				case levelAdmin:
					switch {
					case id.admin:
						if got == http.StatusUnauthorized || got == http.StatusForbidden {
							t.Errorf("admin route %s %s rejected admin with %d",
								rt.method, rt.path, got)
						}
					case id.authenticated:
						if got != http.StatusForbidden {
							t.Errorf("admin route %s %s returned %d for a registration token; want 403",
								rt.method, rt.path, got)
						}
					default:
						if got != http.StatusUnauthorized {
							t.Errorf("admin route %s %s returned %d for %s; want 401",
								rt.method, rt.path, got, id.name)
						}
					}
				}
			})
		}
	}
}

// TestMalformedAuthorizationHeadersAreRejected pins the exact behaviour of the
// Bearer prefix check, which is a plain HasPrefix and therefore case-sensitive
// and whitespace-sensitive. These are the shapes a sloppy client is most likely
// to send; every one of them must be a 401, never a partial success.
func TestMalformedAuthorizationHeadersAreRejected(t *testing.T) {
	database, router := newAuthFixture(t, authmw.TrustedProxyDirect)

	raw := "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	mintToken(t, database, "Ada", "Admin", "admin", raw)

	cases := []struct {
		name   string
		header string
	}{
		{"lowercase scheme", "bearer " + raw},
		{"uppercase scheme", "BEARER " + raw},
		{"mixed case scheme", "BeArEr " + raw},
		{"no scheme", raw},
		{"basic scheme", "Basic " + raw},
		{"double space", "Bearer  " + raw},
		{"leading space", " Bearer " + raw},
		{"trailing whitespace on token", "Bearer " + raw + " "},
		{"token with newline", "Bearer " + raw + "\n"},
		{"scheme only", "Bearer"},
		{"scheme and space only", "Bearer "},
		{"empty", ""},
		{"null byte suffix", "Bearer " + raw + "\x00"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			headers := map[string]string{}
			// http.Header.Set panics on nothing but does normalise; send raw via
			// the recorder request so control characters survive.
			headers["Authorization"] = tc.header
			rec := request(t, router, http.MethodGet, "/api/auth/me", "198.51.100.4:1111", headers, nil)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("Authorization %q returned %d; want 401", tc.header, rec.Code)
			}
		})
	}

	// Control: the exact header shape must work, otherwise the negatives above
	// would pass for the wrong reason.
	rec := request(t, router, http.MethodGet, "/api/auth/me", "198.51.100.4:1111", bearer(raw), nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("well-formed Bearer header returned %d; want 200", rec.Code)
	}
}
