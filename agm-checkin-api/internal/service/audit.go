package service

import (
	"encoding/json"
	"log"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"johndpete316/agm-checkin-api/internal/db"
)

type AuditService struct {
	db *gorm.DB
}

func NewAuditService(database *gorm.DB) *AuditService {
	return &AuditService{db: database}
}

// LogEntry carries all the context needed to write an audit record.
type LogEntry struct {
	ActorID    string
	ActorName  string
	Action     string         // e.g. "competitor.checked_in"
	EntityType string         // "competitor" | "staff_token" | "event"
	EntityID   string
	EntityName string         // human-readable snapshot of the entity
	Detail     map[string]any // action-specific context, stored as JSON
	IP         string
}

// Log writes an audit record. Errors are logged to stdout and never surfaced to the caller
// so that a logging failure never breaks the primary operation.
func (s *AuditService) Log(entry LogEntry) {
	detail := "{}"
	if entry.Detail != nil {
		if b, err := json.Marshal(entry.Detail); err == nil {
			detail = string(b)
		}
	}
	record := db.AuditLog{
		ID:         uuid.New().String(),
		ActorID:    entry.ActorID,
		ActorName:  entry.ActorName,
		Action:     entry.Action,
		EntityType: entry.EntityType,
		EntityID:   entry.EntityID,
		EntityName: entry.EntityName,
		DetailRaw:  detail,
		IPAddress:  entry.IP,
		CreatedAt:  time.Now(),
	}
	if err := s.db.Create(&record).Error; err != nil {
		log.Printf("audit log write failed (action=%s entity=%s): %v", entry.Action, entry.EntityID, err)
	}
}

// AuditLogView is the API response shape — embeds AuditLog with DetailRaw re-exposed
// as a proper JSON value rather than a raw string.
type AuditLogView struct {
	db.AuditLog
	Detail json.RawMessage `json:"detail"`
}

func toView(record db.AuditLog) AuditLogView {
	detail := json.RawMessage(record.DetailRaw)
	if !json.Valid(detail) {
		detail = json.RawMessage("{}")
	}
	return AuditLogView{AuditLog: record, Detail: detail}
}

// List returns audit log entries, most recent first.
// action and actorName are optional filters; limit defaults to 100 (max 500).
func (s *AuditService) List(action, actorName string, limit int) ([]AuditLogView, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := s.db.Order("created_at desc").Limit(limit)
	if action != "" {
		query = query.Where("action = ?", action)
	}
	if actorName != "" {
		query = query.Where("actor_name ILIKE ?", "%"+actorName+"%")
	}
	var records []db.AuditLog
	if err := query.Find(&records).Error; err != nil {
		return nil, err
	}
	views := make([]AuditLogView, len(records))
	for i, r := range records {
		views[i] = toView(r)
	}
	return views, nil
}
