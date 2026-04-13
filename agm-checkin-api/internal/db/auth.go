package db

import "time"

type IPBlocklist struct {
	ID        uint      `gorm:"primaryKey;autoIncrement"`
	IPAddress string    `gorm:"uniqueIndex;not null"`
	BlockedAt time.Time `gorm:"not null"`
}

type PINAttempt struct {
	ID          uint      `gorm:"primaryKey;autoIncrement"`
	IPAddress   string    `gorm:"index;not null"`
	AttemptedAt time.Time `gorm:"not null"`
}

type StaffToken struct {
	ID        string     `gorm:"primaryKey;type:uuid" json:"id"`
	Token     string     `gorm:"uniqueIndex;not null" json:"-"` // never serialised to JSON; only returned at creation time
	FirstName string     `gorm:"not null" json:"firstName"`
	LastName  string     `gorm:"not null" json:"lastName"`
	Role      string     `gorm:"not null;default:'registration'" json:"role"` // "registration" or "admin"
	CreatedAt time.Time  `json:"createdAt"`
	ExpiresAt *time.Time `gorm:"index" json:"expiresAt"` // nil = no expiry (legacy tokens). See Finding 6.
}

// StaffView is the safe public representation of a StaffToken.
// It omits the raw bearer token so it can be freely returned from list and
// "me" endpoints without leaking credentials.
type StaffView struct {
	ID        string     `json:"id"`
	FirstName string     `json:"firstName"`
	LastName  string     `json:"lastName"`
	Role      string     `json:"role"`
	CreatedAt time.Time  `json:"createdAt"`
	ExpiresAt *time.Time `json:"expiresAt"`
}

// ToView converts a StaffToken to a StaffView (no token field).
func (s *StaffToken) ToView() StaffView {
	return StaffView{
		ID:        s.ID,
		FirstName: s.FirstName,
		LastName:  s.LastName,
		Role:      s.Role,
		CreatedAt: s.CreatedAt,
		ExpiresAt: s.ExpiresAt,
	}
}
