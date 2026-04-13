package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"johndpete316/agm-checkin-api/internal/db"
	mw "johndpete316/agm-checkin-api/internal/middleware"
)

// mockAuthChecker is a simple in-memory stub that satisfies middleware.AuthChecker.
// No database is required for these tests.
type mockAuthChecker struct {
	blockedIPs  map[string]bool
	validTokens map[string]*db.StaffToken
}

func (m *mockAuthChecker) IsIPBlocked(ip string) bool {
	return m.blockedIPs[ip]
}

func (m *mockAuthChecker) ValidateToken(token string) (*db.StaffToken, bool) {
	t, ok := m.validTokens[token]
	return t, ok
}

func newMock() *mockAuthChecker {
	return &mockAuthChecker{
		blockedIPs:  make(map[string]bool),
		validTokens: make(map[string]*db.StaffToken),
	}
}

// ---- GetClientIP ----------------------------------------------------------------

func TestGetClientIP_CloudflareHeader_TakesPriority(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("CF-Connecting-IP", "1.2.3.4")
	r.Header.Set("X-Forwarded-For", "5.6.7.8")
	r.RemoteAddr = "9.10.11.12:9999"

	if got := mw.GetClientIP(r); got != "1.2.3.4" {
		t.Errorf("expected CF-Connecting-IP to take priority, got %q", got)
	}
}

func TestGetClientIP_XForwardedFor_UsedWhenNoCFHeader(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Forwarded-For", "5.6.7.8")
	r.RemoteAddr = "9.10.11.12:9999"

	if got := mw.GetClientIP(r); got != "5.6.7.8" {
		t.Errorf("expected X-Forwarded-For, got %q", got)
	}
}

func TestGetClientIP_XForwardedFor_TakesLeftmostAddress(t *testing.T) {
	// X-Forwarded-For may contain a chain of IPs added by each proxy.
	// The leftmost entry is the original client; only that should be trusted.
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Forwarded-For", "5.6.7.8, 10.0.0.1, 10.0.0.2")

	if got := mw.GetClientIP(r); got != "5.6.7.8" {
		t.Errorf("expected leftmost IP from X-Forwarded-For, got %q", got)
	}
}

func TestGetClientIP_RemoteAddr_FallbackWhenNoHeaders(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "9.10.11.12:9999"

	if got := mw.GetClientIP(r); got != "9.10.11.12" {
		t.Errorf("expected host portion of RemoteAddr, got %q", got)
	}
}

func TestGetClientIP_RemoteAddr_StripsPort(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "203.0.113.5:54321"

	got := mw.GetClientIP(r)
	if got != "203.0.113.5" {
		t.Errorf("expected IP without port, got %q", got)
	}
}

// ---- IPBlocklist middleware ------------------------------------------------------

func TestIPBlocklistMiddleware_BlockedIP_Returns403(t *testing.T) {
	mock := newMock()
	mock.blockedIPs["1.2.3.4"] = true

	handler := mw.IPBlocklist(mock)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("CF-Connecting-IP", "1.2.3.4")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for blocked IP, got %d", w.Code)
	}
}

func TestIPBlocklistMiddleware_BlockedIP_DoesNotCallNextHandler(t *testing.T) {
	mock := newMock()
	mock.blockedIPs["1.2.3.4"] = true

	nextCalled := false
	handler := mw.IPBlocklist(mock)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("CF-Connecting-IP", "1.2.3.4")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if nextCalled {
		t.Error("next handler must not be called for a blocked IP")
	}
}

func TestIPBlocklistMiddleware_UnblockedIP_PassesThrough(t *testing.T) {
	mock := newMock()

	handler := mw.IPBlocklist(mock)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("CF-Connecting-IP", "5.6.7.8")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for unblocked IP, got %d", w.Code)
	}
}

func TestIPBlocklistMiddleware_BlockedIP_IsolatedFromOthers(t *testing.T) {
	// Blocking one IP must not affect other IPs.
	mock := newMock()
	mock.blockedIPs["1.2.3.4"] = true

	handler := mw.IPBlocklist(mock)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("CF-Connecting-IP", "9.9.9.9")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("blocking 1.2.3.4 should not block 9.9.9.9, got %d", w.Code)
	}
}

// ---- RequireToken middleware -----------------------------------------------------

func TestRequireToken_NoAuthorizationHeader_Returns401(t *testing.T) {
	mock := newMock()
	handler := mw.RequireToken(mock)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/api/competitors", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with no Authorization header, got %d", w.Code)
	}
}

func TestRequireToken_NonBearerScheme_Returns401(t *testing.T) {
	mock := newMock()
	handler := mw.RequireToken(mock)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/api/competitors", nil)
	r.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for non-Bearer auth scheme, got %d", w.Code)
	}
}

func TestRequireToken_EmptyBearerValue_Returns401(t *testing.T) {
	mock := newMock()
	handler := mw.RequireToken(mock)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/api/competitors", nil)
	r.Header.Set("Authorization", "Bearer ")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for empty bearer token, got %d", w.Code)
	}
}

func TestRequireToken_InvalidToken_Returns401(t *testing.T) {
	mock := newMock()
	handler := mw.RequireToken(mock)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/api/competitors", nil)
	r.Header.Set("Authorization", "Bearer not-a-real-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid token, got %d", w.Code)
	}
}

func TestRequireToken_ValidToken_Returns200(t *testing.T) {
	mock := newMock()
	mock.validTokens["valid-token-abc"] = &db.StaffToken{
		ID:        "uuid-1",
		Token:     "valid-token-abc",
		FirstName: "Jane",
		LastName:  "Smith",
	}

	handler := mw.RequireToken(mock)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/api/competitors", nil)
	r.Header.Set("Authorization", "Bearer valid-token-abc")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for valid token, got %d", w.Code)
	}
}

func TestRequireToken_ValidToken_InjectsStaffIntoContext(t *testing.T) {
	mock := newMock()
	expected := &db.StaffToken{
		ID:        "uuid-1",
		Token:     "valid-token-abc",
		FirstName: "Jane",
		LastName:  "Smith",
	}
	mock.validTokens["valid-token-abc"] = expected

	var gotStaff *db.StaffToken
	handler := mw.RequireToken(mock)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotStaff = mw.StaffFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/api/competitors", nil)
	r.Header.Set("Authorization", "Bearer valid-token-abc")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if gotStaff == nil {
		t.Fatal("expected staff token in context, got nil")
	}
	if gotStaff.FirstName != "Jane" || gotStaff.LastName != "Smith" {
		t.Errorf("unexpected staff in context: %+v", gotStaff)
	}
}

func TestRequireToken_ValidToken_DoesNotCallNextWithoutToken(t *testing.T) {
	// A request with NO token must not reach the protected handler.
	mock := newMock()
	nextCalled := false
	handler := mw.RequireToken(mock)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/api/competitors", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if nextCalled {
		t.Error("next handler must not be called without a valid token")
	}
}

// ---- StaffFromContext -----------------------------------------------------------

func TestStaffFromContext_ReturnsNilOnEmptyContext(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := mw.StaffFromContext(r.Context()); got != nil {
		t.Errorf("expected nil for empty context, got %+v", got)
	}
}
