package service_test

// These tests exercise the security-critical AuthService logic against a real
// PostgreSQL database.  They are skipped automatically when the TEST_DATABASE_URL
// environment variable is not set, so the standard unit-test run remains fast and
// dependency-free.  To run them:
//
//   TEST_DATABASE_URL="postgres://..." go test ./internal/service/...

import (
	"os"
	"strings"
	"testing"
	"time"

	"johndpete316/agm-checkin-api/internal/db"
	"johndpete316/agm-checkin-api/internal/service"
	"gorm.io/gorm"
)

// testPIN is deliberately long and clearly non-production so it cannot
// accidentally match the AUTH_PIN of a real deployment.
const testPIN = "test-pin-security-regression-NOT-FOR-PRODUCTION"

// setupAuthSvc connects to the test database (or skips the test) and returns an
// AuthService together with the underlying *gorm.DB for cleanup helpers.
func setupAuthSvc(t *testing.T) (*service.AuthService, *gorm.DB) {
	t.Helper()

	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}

	database := db.Connect(dsn)
	db.AutoMigrate(database)

	t.Cleanup(func() {
		sqlDB, _ := database.DB()
		sqlDB.Close()
	})

	return service.NewAuthService(database, testPIN), database
}

// cleanupIP removes all PINAttempt and IPBlocklist rows for the given IP so that
// tests do not interfere with each other.
func cleanupIP(t *testing.T, database *gorm.DB, ip string) {
	t.Helper()
	database.Where("ip_address = ?", ip).Delete(&db.PINAttempt{})
	database.Where("ip_address = ?", ip).Delete(&db.IPBlocklist{})
}

// cleanupToken removes a StaffToken row by its token value.
func cleanupToken(t *testing.T, database *gorm.DB, token string) {
	t.Helper()
	database.Where("token = ?", token).Delete(&db.StaffToken{})
}

// ---- IsIPBlocked ----------------------------------------------------------------

func TestIsIPBlocked_UnknownIP_ReturnsFalse(t *testing.T) {
	svc, _ := setupAuthSvc(t)

	if svc.IsIPBlocked("192.0.2.1") {
		t.Error("expected IsIPBlocked to return false for an IP not in the blocklist")
	}
}

func TestIsIPBlocked_BlockedIP_ReturnsTrue(t *testing.T) {
	svc, database := setupAuthSvc(t)

	ip := "192.0.2.2"
	t.Cleanup(func() { cleanupIP(t, database, ip) })

	database.Create(&db.IPBlocklist{IPAddress: ip, BlockedAt: time.Now()})

	if !svc.IsIPBlocked(ip) {
		t.Error("expected IsIPBlocked to return true for a blocked IP")
	}
}

// ---- ValidateToken --------------------------------------------------------------

func TestValidateToken_ValidToken_ReturnsStaffAndTrue(t *testing.T) {
	svc, database := setupAuthSvc(t)

	tok := &db.StaffToken{
		ID:        "test-uuid-validate-ok",
		Token:     strings.Repeat("a", 64),
		FirstName: "Test",
		LastName:  "User",
		CreatedAt: time.Now(),
	}
	database.Create(tok)
	t.Cleanup(func() { cleanupToken(t, database, tok.Token) })

	got, ok := svc.ValidateToken(tok.Token)
	if !ok {
		t.Fatal("expected ValidateToken to return true for a valid token")
	}
	if got.FirstName != "Test" || got.LastName != "User" {
		t.Errorf("unexpected staff returned: %+v", got)
	}
}

func TestValidateToken_InvalidToken_ReturnsFalse(t *testing.T) {
	svc, _ := setupAuthSvc(t)

	_, ok := svc.ValidateToken("this-token-does-not-exist-in-the-database")
	if ok {
		t.Error("expected ValidateToken to return false for a non-existent token")
	}
}

func TestValidateToken_EmptyToken_ReturnsFalse(t *testing.T) {
	svc, _ := setupAuthSvc(t)

	_, ok := svc.ValidateToken("")
	if ok {
		t.Error("expected ValidateToken to return false for an empty token")
	}
}

// ---- VerifyPINAndCreateToken ----------------------------------------------------

func TestVerifyPINAndCreateToken_CorrectPIN_ReturnsToken(t *testing.T) {
	svc, database := setupAuthSvc(t)

	ip := "192.0.2.10"
	t.Cleanup(func() { cleanupIP(t, database, ip) })

	tok, err := svc.VerifyPINAndCreateToken(ip, testPIN, "Alice", "Jones")
	if err != nil {
		t.Fatalf("expected success with correct PIN, got error: %v", err)
	}
	if tok == nil || tok.Token == "" {
		t.Fatal("expected a non-empty token on success")
	}
	t.Cleanup(func() { cleanupToken(t, database, tok.Token) })

	if tok.FirstName != "Alice" || tok.LastName != "Jones" {
		t.Errorf("token carries wrong staff name: %+v", tok)
	}
}

func TestVerifyPINAndCreateToken_WrongPIN_ReturnsErrInvalidPIN(t *testing.T) {
	svc, database := setupAuthSvc(t)

	ip := "192.0.2.11"
	t.Cleanup(func() { cleanupIP(t, database, ip) })

	_, err := svc.VerifyPINAndCreateToken(ip, "wrong-pin", "Bob", "Smith")
	if err == nil {
		t.Fatal("expected an error for a wrong PIN, got nil")
	}
	// First bad attempt: should be ErrInvalidPIN, not ErrTooManyAttempts yet.
	if err != service.ErrInvalidPIN {
		t.Errorf("expected ErrInvalidPIN on first bad attempt, got %v", err)
	}
}

func TestVerifyPINAndCreateToken_ThreeWrongPINs_BlocksIP(t *testing.T) {
	svc, database := setupAuthSvc(t)

	ip := "192.0.2.12"
	t.Cleanup(func() { cleanupIP(t, database, ip) })

	// First two failures → ErrInvalidPIN
	for i := 0; i < 2; i++ {
		_, err := svc.VerifyPINAndCreateToken(ip, "wrong-pin", "Bob", "Smith")
		if err != service.ErrInvalidPIN {
			t.Fatalf("attempt %d: expected ErrInvalidPIN, got %v", i+1, err)
		}
	}

	// Third failure must trip the lockout.
	_, err := svc.VerifyPINAndCreateToken(ip, "wrong-pin", "Bob", "Smith")
	if err != service.ErrTooManyAttempts {
		t.Errorf("expected ErrTooManyAttempts on third bad attempt, got %v", err)
	}

	// After lockout the IP must be blocked in the database.
	if !svc.IsIPBlocked(ip) {
		t.Error("IP must be in the blocklist after three failed attempts")
	}
}

func TestVerifyPINAndCreateToken_BlockedIP_ReturnsErrIPBlocked(t *testing.T) {
	svc, database := setupAuthSvc(t)

	ip := "192.0.2.13"
	t.Cleanup(func() { cleanupIP(t, database, ip) })

	// Pre-block the IP directly.
	database.Create(&db.IPBlocklist{IPAddress: ip, BlockedAt: time.Now()})

	// Even with the correct PIN, a blocked IP must be rejected immediately.
	_, err := svc.VerifyPINAndCreateToken(ip, testPIN, "Carol", "White")
	if err != service.ErrIPBlocked {
		t.Errorf("expected ErrIPBlocked for a pre-blocked IP, got %v", err)
	}
}

func TestVerifyPINAndCreateToken_BlockedIPWithCorrectPIN_DoesNotCreateToken(t *testing.T) {
	svc, database := setupAuthSvc(t)

	ip := "192.0.2.14"
	t.Cleanup(func() { cleanupIP(t, database, ip) })

	database.Create(&db.IPBlocklist{IPAddress: ip, BlockedAt: time.Now()})

	tok, _ := svc.VerifyPINAndCreateToken(ip, testPIN, "Eve", "Black")
	if tok != nil {
		t.Errorf("blocked IP must not receive a token, got: %+v", tok)
		cleanupToken(t, database, tok.Token)
	}
}

func TestVerifyPINAndCreateToken_CorrectPINDoesNotIncrementAttemptCount(t *testing.T) {
	svc, database := setupAuthSvc(t)

	ip := "192.0.2.15"
	t.Cleanup(func() { cleanupIP(t, database, ip) })

	tok, err := svc.VerifyPINAndCreateToken(ip, testPIN, "Dave", "Brown")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	t.Cleanup(func() { cleanupToken(t, database, tok.Token) })

	var count int64
	database.Model(&db.PINAttempt{}).Where("ip_address = ?", ip).Count(&count)
	if count != 0 {
		t.Errorf("successful login must not record a PINAttempt, found %d", count)
	}
}
