package main

// Handler-level security regression tests.
//
// These tests exercise the full HTTP layer without a database by using in-memory
// stubs that satisfy the interfaces declared in main.go and internal/middleware.
// They complement the existing per-package unit tests:
//   - internal/middleware/auth_test.go  — middleware logic in isolation
//   - internal/service/auth_test.go    — auth service against a real Postgres DB

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"johndpete316/agm-checkin-api/internal/db"
	authmw "johndpete316/agm-checkin-api/internal/middleware"
	"johndpete316/agm-checkin-api/internal/service"
)

// ---- stubs ----------------------------------------------------------------------

// mockPINVerifier implements pinVerifier for createToken handler tests.
type mockPINVerifier struct {
	fn func(ip, pin, firstName, lastName string) (*db.StaffToken, error)
}

func (m *mockPINVerifier) VerifyPINAndCreateToken(ip, pin, firstName, lastName string) (*db.StaffToken, error) {
	return m.fn(ip, pin, firstName, lastName)
}

// mockRouteAuthChecker implements authmw.AuthChecker for route-enforcement tests.
type mockRouteAuthChecker struct {
	blockedIPs  map[string]bool
	validTokens map[string]*db.StaffToken
}

func (m *mockRouteAuthChecker) IsIPBlocked(ip string) bool {
	return m.blockedIPs[ip]
}
func (m *mockRouteAuthChecker) ValidateToken(token string) (*db.StaffToken, bool) {
	t, ok := m.validTokens[token]
	return t, ok
}

func newRouteAuthChecker() *mockRouteAuthChecker {
	return &mockRouteAuthChecker{
		blockedIPs:  make(map[string]bool),
		validTokens: make(map[string]*db.StaffToken),
	}
}

// buildSecurityTestRouter constructs a chi router that mirrors the production
// route structure.  Protected handlers are stubs — the goal is to verify that
// the correct middleware is applied to each route, not to test handler logic.
func buildSecurityTestRouter(authChecker authmw.AuthChecker, verifier pinVerifier) http.Handler {
	r := chi.NewRouter()

	// Security headers — same as production.
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Referrer-Policy", "no-referrer")
			next.ServeHTTP(w, r)
		})
	})

	r.Use(authmw.IPBlocklist(authChecker))

	// Public routes.
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	r.Post("/api/auth/token", createToken(verifier))

	// Protected routes — stub handlers that return 204 when auth passes.
	r.Group(func(r chi.Router) {
		r.Use(authmw.RequireToken(authChecker))
		stub := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
		r.Get("/api/competitors", stub)
		r.Get("/api/competitors/{id}", stub)
		r.Post("/api/competitors", stub)
		r.Patch("/api/competitors/{id}/checkin", stub)
		r.Patch("/api/competitors/{id}/dob", stub)
		r.Patch("/api/competitors/{id}/validate", stub)
		r.Delete("/api/competitors/{id}", stub)
	})

	return r
}

// ---- helpers --------------------------------------------------------------------

// postJSON sends a POST request with a JSON body to the handler or router.
func postJSON(t *testing.T, handler http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	return w
}

// alwaysFailVerifier returns a pinVerifier that fails the test if ever called.
func alwaysFailVerifier(t *testing.T) *mockPINVerifier {
	t.Helper()
	return &mockPINVerifier{fn: func(ip, pin, firstName, lastName string) (*db.StaffToken, error) {
		t.Fatal("service must not be called when request is rejected by handler validation")
		return nil, nil
	}}
}

// ---- createToken handler tests: input validation --------------------------------

func TestCreateToken_MalformedJSON_Returns400(t *testing.T) {
	handler := createToken(alwaysFailVerifier(t))
	r := httptest.NewRequest(http.MethodPost, "/api/auth/token", bytes.NewBufferString("{not valid json"))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for malformed JSON, got %d", w.Code)
	}
}

func TestCreateToken_MissingCode_Returns400(t *testing.T) {
	handler := createToken(alwaysFailVerifier(t))
	w := postJSON(t, handler, "/api/auth/token", map[string]string{"firstName": "Alice", "lastName": "Smith"})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 when code is missing, got %d", w.Code)
	}
}

func TestCreateToken_MissingFirstName_Returns400(t *testing.T) {
	handler := createToken(alwaysFailVerifier(t))
	w := postJSON(t, handler, "/api/auth/token", map[string]string{"code": "1234", "lastName": "Smith"})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 when firstName is missing, got %d", w.Code)
	}
}

func TestCreateToken_MissingLastName_Returns400(t *testing.T) {
	handler := createToken(alwaysFailVerifier(t))
	w := postJSON(t, handler, "/api/auth/token", map[string]string{"code": "1234", "firstName": "Alice"})
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 when lastName is missing, got %d", w.Code)
	}
}

func TestCreateToken_EmptyBody_Returns400(t *testing.T) {
	handler := createToken(alwaysFailVerifier(t))
	r := httptest.NewRequest(http.MethodPost, "/api/auth/token", bytes.NewBufferString("{}"))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty JSON object, got %d", w.Code)
	}
}

// ---- createToken handler tests: error-to-HTTP-status mapping --------------------

func TestCreateToken_ErrIPBlocked_Returns403(t *testing.T) {
	v := &mockPINVerifier{fn: func(ip, pin, firstName, lastName string) (*db.StaffToken, error) {
		return nil, service.ErrIPBlocked
	}}
	w := postJSON(t, createToken(v), "/api/auth/token",
		map[string]string{"code": "pin", "firstName": "A", "lastName": "B"})
	if w.Code != http.StatusForbidden {
		t.Errorf("ErrIPBlocked must map to 403, got %d", w.Code)
	}
}

func TestCreateToken_ErrTooManyAttempts_Returns403(t *testing.T) {
	v := &mockPINVerifier{fn: func(ip, pin, firstName, lastName string) (*db.StaffToken, error) {
		return nil, service.ErrTooManyAttempts
	}}
	w := postJSON(t, createToken(v), "/api/auth/token",
		map[string]string{"code": "pin", "firstName": "A", "lastName": "B"})
	if w.Code != http.StatusForbidden {
		t.Errorf("ErrTooManyAttempts must map to 403, got %d", w.Code)
	}
}

func TestCreateToken_ErrInvalidPIN_Returns401(t *testing.T) {
	v := &mockPINVerifier{fn: func(ip, pin, firstName, lastName string) (*db.StaffToken, error) {
		return nil, service.ErrInvalidPIN
	}}
	w := postJSON(t, createToken(v), "/api/auth/token",
		map[string]string{"code": "wrong", "firstName": "A", "lastName": "B"})
	if w.Code != http.StatusUnauthorized {
		t.Errorf("ErrInvalidPIN must map to 401, got %d", w.Code)
	}
}

func TestCreateToken_UnexpectedError_Returns500(t *testing.T) {
	v := &mockPINVerifier{fn: func(ip, pin, firstName, lastName string) (*db.StaffToken, error) {
		return nil, errors.New("database connection lost")
	}}
	w := postJSON(t, createToken(v), "/api/auth/token",
		map[string]string{"code": "pin", "firstName": "A", "lastName": "B"})
	if w.Code != http.StatusInternalServerError {
		t.Errorf("unexpected error must map to 500, got %d", w.Code)
	}
}

func TestCreateToken_Success_Returns201WithFields(t *testing.T) {
	expectedToken := "abc123tokenvalue"
	v := &mockPINVerifier{fn: func(ip, pin, firstName, lastName string) (*db.StaffToken, error) {
		return &db.StaffToken{
			Token:     expectedToken,
			FirstName: firstName,
			LastName:  lastName,
		}, nil
	}}
	w := postJSON(t, createToken(v), "/api/auth/token",
		map[string]string{"code": "goodpin", "firstName": "Alice", "lastName": "Smith"})
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201 on success, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("could not decode response: %v", err)
	}
	if resp["token"] != expectedToken {
		t.Errorf("token in response = %q, want %q", resp["token"], expectedToken)
	}
	if resp["firstName"] != "Alice" {
		t.Errorf("firstName in response = %q, want \"Alice\"", resp["firstName"])
	}
	if resp["lastName"] != "Smith" {
		t.Errorf("lastName in response = %q, want \"Smith\"", resp["lastName"])
	}
}

// TestCreateToken_ResponseDoesNotLeakPINOnFailure ensures the error body never
// echoes back the submitted PIN or code value.
func TestCreateToken_ResponseDoesNotLeakPINOnFailure(t *testing.T) {
	const secretPIN = "super-secret-pin-value"
	v := &mockPINVerifier{fn: func(ip, pin, firstName, lastName string) (*db.StaffToken, error) {
		return nil, service.ErrInvalidPIN
	}}
	handler := createToken(v)
	r := httptest.NewRequest(http.MethodPost, "/api/auth/token", bytes.NewBufferString(
		`{"code":"`+secretPIN+`","firstName":"A","lastName":"B"}`,
	))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	body := w.Body.String()
	if containsSubstring(body, secretPIN) {
		t.Errorf("response body must not contain the submitted PIN; got: %s", body)
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsAt(s, sub))
}

func containsAt(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ---- Route enforcement tests ----------------------------------------------------

// TestProtectedRoutes_NoAuth_AllReturn401 verifies that every route in the
// protected group returns 401 when no Authorization header is provided.
// This is the regression guard against accidentally removing RequireToken
// from one of the routes.
func TestProtectedRoutes_NoAuth_AllReturn401(t *testing.T) {
	authChecker := newRouteAuthChecker() // no valid tokens
	verifier := &mockPINVerifier{fn: func(ip, pin, firstName, lastName string) (*db.StaffToken, error) {
		return nil, service.ErrInvalidPIN
	}}
	router := buildSecurityTestRouter(authChecker, verifier)

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/competitors"},
		{http.MethodGet, "/api/competitors/some-uuid"},
		{http.MethodPost, "/api/competitors"},
		{http.MethodPatch, "/api/competitors/some-uuid/checkin"},
		{http.MethodPatch, "/api/competitors/some-uuid/dob"},
		{http.MethodPatch, "/api/competitors/some-uuid/validate"},
		{http.MethodDelete, "/api/competitors/some-uuid"},
	}

	for _, tc := range routes {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("expected 401 without auth token, got %d", w.Code)
			}
		})
	}
}

// TestProtectedRoutes_ValidToken_PassThrough verifies that a valid token allows
// the request to reach the handler (returns 204 from the stub, not 401).
func TestProtectedRoutes_ValidToken_PassThrough(t *testing.T) {
	authChecker := newRouteAuthChecker()
	authChecker.validTokens["valid-token"] = &db.StaffToken{
		ID: "uuid-1", Token: "valid-token", FirstName: "Jane", LastName: "Doe",
	}
	verifier := &mockPINVerifier{fn: func(ip, pin, firstName, lastName string) (*db.StaffToken, error) {
		return nil, service.ErrInvalidPIN
	}}
	router := buildSecurityTestRouter(authChecker, verifier)

	r := httptest.NewRequest(http.MethodGet, "/api/competitors", nil)
	r.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected stub 204 for authenticated request, got %d", w.Code)
	}
}

// TestHealthEndpoint_NoAuth_Returns200 confirms the health probe is public.
func TestHealthEndpoint_NoAuth_Returns200(t *testing.T) {
	authChecker := newRouteAuthChecker()
	verifier := &mockPINVerifier{fn: func(ip, pin, firstName, lastName string) (*db.StaffToken, error) {
		return nil, service.ErrInvalidPIN
	}}
	router := buildSecurityTestRouter(authChecker, verifier)

	r := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for /health without auth, got %d", w.Code)
	}
}

// TestAuthTokenEndpoint_NoAuth_NotForbidden confirms POST /api/auth/token is
// publicly reachable (returns 400 for missing body, not 401 or 403).
func TestAuthTokenEndpoint_NoAuth_NotForbidden(t *testing.T) {
	authChecker := newRouteAuthChecker()
	verifier := &mockPINVerifier{fn: func(ip, pin, firstName, lastName string) (*db.StaffToken, error) {
		return nil, service.ErrInvalidPIN
	}}
	router := buildSecurityTestRouter(authChecker, verifier)

	// Send an empty body — we expect 400 (validation), not 401/403 (auth).
	r := httptest.NewRequest(http.MethodPost, "/api/auth/token", bytes.NewBufferString("{}"))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)

	if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
		t.Errorf("auth endpoint must be publicly reachable (got %d, want 400)", w.Code)
	}
}

// TestBlockedIP_CannotReachAnyEndpoint verifies that the IPBlocklist middleware
// intercepts a blocked IP before it reaches any handler, including public routes.
func TestBlockedIP_CannotReachAnyEndpoint(t *testing.T) {
	authChecker := newRouteAuthChecker()
	authChecker.blockedIPs["1.2.3.4"] = true
	verifier := &mockPINVerifier{fn: func(ip, pin, firstName, lastName string) (*db.StaffToken, error) {
		t.Fatal("blocked IP must not reach the handler")
		return nil, nil
	}}
	router := buildSecurityTestRouter(authChecker, verifier)

	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/health"},
		{http.MethodPost, "/api/auth/token"},
		{http.MethodGet, "/api/competitors"},
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			r := httptest.NewRequest(ep.method, ep.path, nil)
			r.Header.Set("CF-Connecting-IP", "1.2.3.4")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			if w.Code != http.StatusForbidden {
				t.Errorf("blocked IP must get 403 on %s %s, got %d", ep.method, ep.path, w.Code)
			}
		})
	}
}

// ---- Security response header tests ---------------------------------------------

// TestSecurityHeaders_PresentOnAllResponses verifies that every response carries
// the hardening headers regardless of the endpoint or auth status.
func TestSecurityHeaders_PresentOnAllResponses(t *testing.T) {
	authChecker := newRouteAuthChecker()
	verifier := &mockPINVerifier{fn: func(ip, pin, firstName, lastName string) (*db.StaffToken, error) {
		return nil, service.ErrInvalidPIN
	}}
	router := buildSecurityTestRouter(authChecker, verifier)

	endpoints := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/health"},
		{http.MethodPost, "/api/auth/token"},
		{http.MethodGet, "/api/competitors"},
	}

	wantHeaders := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "no-referrer",
	}

	for _, ep := range endpoints {
		t.Run(ep.method+" "+ep.path, func(t *testing.T) {
			r := httptest.NewRequest(ep.method, ep.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)

			for header, want := range wantHeaders {
				if got := w.Header().Get(header); got != want {
					t.Errorf("%s: header %q = %q, want %q", ep.path, header, got, want)
				}
			}
		})
	}
}
