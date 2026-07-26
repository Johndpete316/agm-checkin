package service

import (
	"errors"

	"gorm.io/gorm"

	"johndpete316/agm-checkin-api/internal/db"
)

var (
	ErrEventNotFound  = errors.New("event not found")
	ErrNoCurrentEvent = errors.New("no current event is set")
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
