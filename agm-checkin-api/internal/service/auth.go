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

const maxPINAttempts = 3

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

// VerifyPINAndCreateToken validates the PIN against the configured value,
// enforces per-IP attempt limits, and on success creates a persistent staff token.
// Returns ErrIPBlocked, ErrInvalidPIN, or ErrTooManyAttempts on failure.
func (s *AuthService) VerifyPINAndCreateToken(ip, pin, firstName, lastName string) (*db.StaffToken, error) {
	if s.IsIPBlocked(ip) {
		return nil, ErrIPBlocked
	}

	var attemptCount int64
	s.db.Model(&db.PINAttempt{}).Where("ip_address = ?", ip).Count(&attemptCount)

	// Guard: shouldn't reach here if already at limit, but enforce defensively
	if attemptCount >= maxPINAttempts {
		s.blockIP(ip)
		return nil, ErrIPBlocked
	}

	pinMatch := subtle.ConstantTimeCompare([]byte(pin), []byte(s.pin)) == 1

	if !pinMatch {
		s.db.Create(&db.PINAttempt{
			IPAddress:   ip,
			AttemptedAt: time.Now(),
		})

		if attemptCount+1 >= maxPINAttempts {
			s.blockIP(ip)
			return nil, ErrTooManyAttempts
		}

		return nil, ErrInvalidPIN
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, err
	}

	staffToken := &db.StaffToken{
		ID:        uuid.New().String(),
		Token:     hex.EncodeToString(tokenBytes),
		FirstName: firstName,
		LastName:  lastName,
		CreatedAt: time.Now(),
	}

	if err := s.db.Create(staffToken).Error; err != nil {
		return nil, err
	}

	return staffToken, nil
}

func (s *AuthService) ValidateToken(token string) (*db.StaffToken, bool) {
	var staffToken db.StaffToken
	if err := s.db.Where("token = ?", token).First(&staffToken).Error; err != nil {
		return nil, false
	}
	return &staffToken, true
}

func (s *AuthService) blockIP(ip string) {
	// Upsert: if the IP is already there (race condition), ignore the conflict
	s.db.Where(db.IPBlocklist{IPAddress: ip}).
		Attrs(db.IPBlocklist{BlockedAt: time.Now()}).
		FirstOrCreate(&db.IPBlocklist{})
}
