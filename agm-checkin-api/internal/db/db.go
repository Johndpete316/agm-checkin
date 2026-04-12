package db

import (
	"log"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type Competitor struct {
	ID                  string    `gorm:"primaryKey;type:uuid" json:"id"`
	NameFirst           string    `json:"nameFirst"`
	NameLast            string    `json:"nameLast"`
	DateOfBirth         time.Time `json:"dateOfBirth"`
	RequiresValidation  bool      `json:"requiresValidation"`
	Validated           bool      `json:"validated"`
	ShirtSize           string    `json:"shirtSize"`
	Email               string    `json:"email"`
	Teacher             string    `json:"teacher"`
	Studio              string    `json:"studio"`
	LastRegisteredEvent string    `json:"lastRegisteredEvent"`
}

// Event represents a competition event (e.g. "glr-2026").
type Event struct {
	ID        string    `gorm:"primaryKey" json:"id"` // human-readable slug, e.g. "glr-2026"
	Name      string    `gorm:"not null" json:"name"`
	StartDate time.Time `json:"startDate"`
	EndDate   time.Time `json:"endDate"`
	IsCurrent bool      `gorm:"not null;default:false" json:"isCurrent"`
}

// CompetitorEvent records a competitor's participation in a specific event.
// The unique index on (competitor_id, event_id) ensures one row per competitor per event.
type CompetitorEvent struct {
	ID              string     `gorm:"primaryKey;type:uuid" json:"id"`
	CompetitorID    string     `gorm:"not null;uniqueIndex:idx_competitor_event" json:"competitorId"`
	EventID         string     `gorm:"not null;uniqueIndex:idx_competitor_event" json:"eventId"`
	CheckedIn       bool       `gorm:"not null;default:false" json:"checkedIn"`
	CheckInDatetime *time.Time `json:"checkInDatetime"` // null for historical imports
	CheckedInBy     string     `json:"checkedInBy"`     // empty for historical imports
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

func (ce *CompetitorEvent) BeforeCreate(tx *gorm.DB) error {
	ce.ID = uuid.New().String()
	return nil
}

// AuditLog records every state-changing operation with who did it, what changed, and from where.
// DetailRaw stores action-specific JSON (e.g. new role, event ID) — excluded from JSON output;
// callers should embed it as json.RawMessage in a view struct.
type AuditLog struct {
	ID         string    `gorm:"primaryKey;type:uuid" json:"id"`
	ActorID    string    `gorm:"index;not null" json:"actorId"`
	ActorName  string    `gorm:"not null" json:"actorName"`
	Action     string    `gorm:"not null;index" json:"action"`
	EntityType string    `gorm:"not null;index" json:"entityType"`
	EntityID   string    `gorm:"not null;index" json:"entityId"`
	EntityName string    `json:"entityName"`
	DetailRaw  string    `gorm:"column:detail;not null;default:'{}'" json:"-"`
	IPAddress  string    `json:"ipAddress"`
	CreatedAt  time.Time `gorm:"index" json:"createdAt"`
}

func AutoMigrate(database *gorm.DB) {
	database.AutoMigrate(
		&Competitor{},
		&Event{},
		&CompetitorEvent{},
		&AuditLog{},
		&IPBlocklist{},
		&PINAttempt{},
		&StaffToken{},
	)
}
