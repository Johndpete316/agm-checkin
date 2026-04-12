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
	ID        string    `gorm:"primaryKey;type:uuid" json:"id"`
	Token     string    `gorm:"uniqueIndex;not null" json:"token"`
	FirstName string    `gorm:"not null" json:"firstName"`
	LastName  string    `gorm:"not null" json:"lastName"`
	Role      string    `gorm:"not null;default:'registration'" json:"role"` // "registration" or "admin"
	CreatedAt time.Time `json:"createdAt"`
}
