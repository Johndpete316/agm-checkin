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
	NameLast            string    `gorm:"index" json:"nameLast"`
	DateOfBirth         time.Time `json:"dateOfBirth"`
	ShirtSize           string    `json:"shirtSize"`
	Email               string    `json:"email"`
	Teacher             string    `json:"teacher"`
	Studio              string    `json:"studio"`
	LastRegisteredEvent string    `gorm:"index" json:"lastRegisteredEvent"`
	Note                string    `json:"note"`

	// DobVerifiedAt is the single source of truth for identity verification:
	// nil means the date of birth has not been confirmed against ID yet.
	// Verification is permanent, so this is never cleared by the check-in flow.
	DobVerifiedAt *time.Time `json:"dobVerifiedAt"`
	DobVerifiedBy string     `gorm:"not null;default:''" json:"dobVerifiedBy"`

	// Superseded by DobVerifiedAt. Still written so this phase stays revertible;
	// migration 004 drops them.
	RequiresValidation bool `json:"requiresValidation"`
	Validated          bool `json:"validated"`
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
// Foreign keys to competitors and events are owned by migration 001, not by AutoMigrate.
type CompetitorEvent struct {
	ID              string     `gorm:"primaryKey;type:uuid" json:"id"`
	CompetitorID    string     `gorm:"type:uuid;not null;uniqueIndex:idx_competitor_event" json:"competitorId"`
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

// CompetitorSchedule records a single scheduled slot for a competitor at a specific event.
// One competitor may have multiple rows (e.g. Sight Reading + Test List on different days).
// SortOrder is pre-computed at import time (minutes since midnight) and used for ORDER BY.
// Foreign keys to competitors and events are owned by migration 001, not by AutoMigrate.
type CompetitorSchedule struct {
	ID           string    `gorm:"primaryKey;type:uuid" json:"id"`
	CompetitorID string    `gorm:"type:uuid;not null;index:idx_cs_competitor_event" json:"competitorId"`
	EventID      string    `gorm:"not null;index:idx_cs_competitor_event" json:"eventId"`
	Instrument   string    `gorm:"not null" json:"instrument"`
	ScheduleDate time.Time `gorm:"not null;type:date" json:"scheduleDate"`
	ScheduleTime string    `gorm:"column:schedule_time;not null" json:"scheduleTime"`
	Room         string    `json:"room"`
	Category     string    `gorm:"not null" json:"category"`
	Division     string    `gorm:"not null" json:"division"`
	SortOrder    int       `gorm:"not null;default:0" json:"sortOrder"`
}

func (cs *CompetitorSchedule) BeforeCreate(tx *gorm.DB) error {
	cs.ID = uuid.New().String()
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
		&CompetitorSchedule{},
		&AuditLog{},
		&IPBlocklist{},
		&PINAttempt{},
		&StaffToken{},
	)
}
