package db

import (
	"log"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Competitor struct {
	ID                  string     `gorm:"primaryKey;type:uuid" json:"id"`
	NameFirst           string     `json:"nameFirst"`
	NameLast            string     `json:"nameLast"`
	DateOfBirth         time.Time  `json:"dateOfBirth"`
	RequiresValidation  bool       `json:"requiresValidation"`
	Validated           bool       `json:"validated"`
	IsCheckedIn         bool       `json:"isCheckedIn"`
	CheckInDateTime     *time.Time `json:"checkInDateTime"`
	ShirtSize           string     `json:"shirtSize"`
	Email               string     `json:"email"`
	Teacher             string     `json:"teacher"`
	Studio              string     `json:"studio"`
	LastRegisteredEvent string     `json:"lastRegisteredEvent"`
}

func (c *Competitor) BeforeCreate(tx *gorm.DB) error {
	c.ID = uuid.New().String()
	return nil
}

func Connect(dsn string) *gorm.DB {
	database, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("failed to connect to database:", err)
	}
	return database
}

func AutoMigrate(database *gorm.DB) {
	database.AutoMigrate(
		&Competitor{},
		&IPBlocklist{},
		&PINAttempt{},
		&StaffToken{},
	)
}
