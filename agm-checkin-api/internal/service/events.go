package service

import (
	"errors"
	"fmt"

	"gorm.io/gorm"

	"johndpete316/agm-checkin-api/internal/db"
)

var (
	ErrEventNotFound  = errors.New("event not found")
	ErrNoCurrentEvent = errors.New("no current event is set")
	ErrSameEvent      = errors.New("source and target events are the same")
)

type EventService struct {
	db *gorm.DB
}

func NewEventService(database *gorm.DB) *EventService {
	return &EventService{db: database}
}

func (s *EventService) List() ([]db.Event, error) {
	var events []db.Event
	if err := s.db.Order("start_date desc").Find(&events).Error; err != nil {
		return nil, err
	}
	return events, nil
}

func (s *EventService) Create(event *db.Event) error {
	return s.db.Create(event).Error
}

func (s *EventService) GetCurrent() (*db.Event, error) {
	var event db.Event
	if err := s.db.Where("is_current = true").First(&event).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNoCurrentEvent
		}
		return nil, err
	}
	return &event, nil
}

// RosterCopy reports what a carry-forward did, or what it would do when DryRun.
type RosterCopy struct {
	SourceEventID   string `json:"sourceEventId"`
	TargetEventID   string `json:"targetEventId"`
	SourceRoster    int64  `json:"sourceRoster"`
	AlreadyOnTarget int64  `json:"alreadyOnTarget"`
	Copied          int64  `json:"copied"`
	DryRun          bool   `json:"dryRun"`
}

// CopyRoster adds everyone on the source event's roster to the target event,
// skipping competitors already on it. Copied rows are never checked in — this
// seeds who is expected, not who has arrived. Existing check-ins on the target
// are left alone, so the operation is safe to repeat mid-event.
func (s *EventService) CopyRoster(targetID, sourceID string, dryRun bool) (*RosterCopy, error) {
	if targetID == sourceID {
		return nil, ErrSameEvent
	}

	result := RosterCopy{SourceEventID: sourceID, TargetEventID: targetID, DryRun: dryRun}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var known int64
		if err := tx.Model(&db.Event{}).Where("id IN ?", []string{targetID, sourceID}).Count(&known).Error; err != nil {
			return err
		}
		if known < 2 {
			return ErrEventNotFound
		}

		if err := tx.Model(&db.CompetitorEvent{}).Where("event_id = ?", sourceID).
			Count(&result.SourceRoster).Error; err != nil {
			return err
		}
		if err := tx.Model(&db.CompetitorEvent{}).
			Where("event_id = ?", sourceID).
			Where("EXISTS (SELECT 1 FROM competitor_events t WHERE t.competitor_id = competitor_events.competitor_id AND t.event_id = ?)", targetID).
			Count(&result.AlreadyOnTarget).Error; err != nil {
			return err
		}

		if dryRun {
			result.Copied = result.SourceRoster - result.AlreadyOnTarget
			return nil
		}

		insert := tx.Exec(`
			INSERT INTO competitor_events (id, competitor_id, event_id, checked_in)
			SELECT gen_random_uuid(), ce.competitor_id, ?, false
			FROM competitor_events ce
			WHERE ce.event_id = ?
			ON CONFLICT (competitor_id, event_id) DO NOTHING
		`, targetID, sourceID)
		if insert.Error != nil {
			return fmt.Errorf("copying roster from %s to %s: %w", sourceID, targetID, insert.Error)
		}
		result.Copied = insert.RowsAffected
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// SetCurrent clears any existing current event and marks the given event as current.
func (s *EventService) SetCurrent(id string) (*db.Event, error) {
	var event db.Event
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&db.Event{}).Where("is_current = true").Update("is_current", false).Error; err != nil {
			return err
		}
		result := tx.Model(&db.Event{}).Where("id = ?", id).Update("is_current", true)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrEventNotFound
		}
		return tx.First(&event, "id = ?", id).Error
	})
	if err != nil {
		return nil, err
	}
	return &event, nil
}
