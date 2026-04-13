package service

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"johndpete316/agm-checkin-api/internal/db"
)

const (
	maxPINAttempts = 3
	// pinAttemptWindow is the rolling window within which failed PIN attempts are
	// counted against the per-IP limit.  Attempts older than this are ignored,
	// which means a blocked IP automatically becomes unblocked after the window
	// expires — preventing the permanent-block DoS described in Finding 4.
	pinAttemptWindow = 15 * time.Minute
)

var (
	ErrIPBlocked       = errors.New("ip address is blocked")
	ErrInvalidPIN      = errors.New("invalid pin")
	ErrTooManyAttempts = errors.New("too many failed attempts, ip has been blocked")
)

type AuthService struct {
	db  *gorm.DB
	pin string
}

func NewAuthService(database *gorm.DB, pin string) *AuthService {
	return &AuthService{db: database, pin: pin}
}

func (s *AuthService) IsIPBlocked(ip string) bool {
	var count int64
	s.db.Model(&db.IPBlocklist{}).Where("ip_address = ?", ip).Count(&count)
	return count > 0
}

// UnblockIP removes the given IP from the blocklist and deletes its recorded
// PIN attempts so it may immediately attempt login again.
// This is the admin-facing recovery path for Finding 4 (permanent-block DoS).
func (s *AuthService) UnblockIP(ip string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("ip_address = ?", ip).Delete(&db.IPBlocklist{}).Error; err != nil {
			return err
		}
		return tx.Where("ip_address = ?", ip).Delete(&db.PINAttempt{}).Error
	})
}

// VerifyPINAndCreateToken validates the PIN against the configured value,
// enforces per-IP attempt limits atomically, and on success creates a persistent
// staff token. Returns ErrIPBlocked, ErrInvalidPIN, or ErrTooManyAttempts on failure.
func (s *AuthService) VerifyPINAndCreateToken(ip, pin, firstName, lastName string) (*db.StaffToken, error) {
	// Fast-path check before entering the transaction.
	if s.IsIPBlocked(ip) {
		return nil, ErrIPBlocked
	}

	var staffToken *db.StaffToken
	var authErr error

	txErr := s.db.Transaction(func(tx *gorm.DB) error {
		// Acquire a per-IP advisory lock for the duration of this transaction.
		// This serializes concurrent login attempts from the same IP, eliminating
		// the race condition where parallel requests all read the same attempt count
		// and bypass the lockout threshold.
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", ip).Error; err != nil {
			return err
		}

		// Only count attempts within the rolling window (Finding 4: time-bounded
		// lockout so an IP does not stay blocked forever due to old attempts).
		windowStart := time.Now().Add(-pinAttemptWindow)
		var attemptCount int64
		if err := tx.Model(&db.PINAttempt{}).
			Where("ip_address = ? AND attempted_at > ?", ip, windowStart).
			Count(&attemptCount).Error; err != nil {
			return err
		}

		// Guard: shouldn't reach here if already at limit, but enforce defensively.
		if attemptCount >= maxPINAttempts {
			blockIPTx(tx, ip)
			authErr = ErrIPBlocked
			return nil
		}

		if subtle.ConstantTimeCompare([]byte(pin), []byte(s.pin)) != 1 {
			if err := tx.Create(&db.PINAttempt{
				IPAddress:   ip,
				AttemptedAt: time.Now(),
			}).Error; err != nil {
				return err
			}

			if attemptCount+1 >= maxPINAttempts {
				blockIPTx(tx, ip)
				authErr = ErrTooManyAttempts
				return nil
			}

			authErr = ErrInvalidPIN
			return nil
		}

		tokenBytes := make([]byte, 32)
		if _, err := rand.Read(tokenBytes); err != nil {
			return err
		}

		t := &db.StaffToken{
			ID:        uuid.New().String(),
			Token:     hex.EncodeToString(tokenBytes),
			FirstName: firstName,
			LastName:  lastName,
			CreatedAt: time.Now(),
		}

		if err := tx.Create(t).Error; err != nil {
			return err
		}

		staffToken = t
		return nil
	})

	if txErr != nil {
		return nil, txErr
	}
	if authErr != nil {
		return nil, authErr
	}
	return staffToken, nil
}

// emptyToken is a fixed-length placeholder used for constant-time comparisons
// when no token record is found in the database, preventing timing oracles
// that could distinguish non-existent tokens from invalid ones.
const emptyToken = "0000000000000000000000000000000000000000000000000000000000000000"

func (s *AuthService) ValidateToken(token string) (*db.StaffToken, bool) {
	var staffToken db.StaffToken
	if err := s.db.Where("token = ?", token).First(&staffToken).Error; err != nil {
		// Always compare against a dummy so both the DB-miss and DB-hit paths
		// spend the same time in constant-time comparison, preventing a timing
		// side-channel between non-existent and invalid tokens.
		subtle.ConstantTimeCompare([]byte(emptyToken), []byte(token))
		return nil, false
	}
	if subtle.ConstantTimeCompare([]byte(staffToken.Token), []byte(token)) != 1 {
		return nil, false
	}
	return &staffToken, true
}

// blockIPTx adds the given IP to the blocklist within an existing transaction.
// It is idempotent — if the IP is already blocked, it does nothing.
func blockIPTx(tx *gorm.DB, ip string) {
	tx.Where(db.IPBlocklist{IPAddress: ip}).
		Attrs(db.IPBlocklist{BlockedAt: time.Now()}).
		FirstOrCreate(&db.IPBlocklist{})
}
