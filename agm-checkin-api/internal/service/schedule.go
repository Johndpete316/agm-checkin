package service

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"johndpete316/agm-checkin-api/internal/db"
)

type ScheduleService struct {
	db *gorm.DB
}

func NewScheduleService(database *gorm.DB) *ScheduleService {
	return &ScheduleService{db: database}
}

// GetByCompetitorEvent returns all schedule entries for a competitor at a specific event,
// ordered by date then time (via sort_order).
func (s *ScheduleService) GetByCompetitorEvent(competitorID, eventID string) ([]db.CompetitorSchedule, error) {
	var entries []db.CompetitorSchedule
	if err := s.db.
		Where("competitor_id = ? AND event_id = ?", competitorID, eventID).
		Order("schedule_date ASC, sort_order ASC").
		Find(&entries).Error; err != nil {
		return nil, err
	}
	return entries, nil
}

func (s *ScheduleService) GetByID(id string) (*db.CompetitorSchedule, error) {
	var entry db.CompetitorSchedule
	if err := s.db.First(&entry, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrScheduleNotFound
		}
		return nil, err
	}
	return &entry, nil
}

func (s *ScheduleService) Create(entry *db.CompetitorSchedule) error {
	return s.db.Create(entry).Error
}

func (s *ScheduleService) Update(id string, input db.CompetitorSchedule) (*db.CompetitorSchedule, error) {
	var entry db.CompetitorSchedule
	if err := s.db.First(&entry, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrScheduleNotFound
		}
		return nil, err
	}
	input.ID = entry.ID
	input.CompetitorID = entry.CompetitorID
	input.EventID = entry.EventID
	if err := s.db.Save(&input).Error; err != nil {
		return nil, err
	}
	return &input, nil
}

func (s *ScheduleService) Delete(id string) error {
	result := s.db.Delete(&db.CompetitorSchedule{}, "id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrScheduleNotFound
	}
	return nil
}

// BulkUpsert inserts or replaces a batch of schedule entries for a competitor+event.
// It deletes all existing entries for that (competitor_id, event_id) pair first,
// then inserts the new batch — making it safe to re-run the import tool.
func (s *ScheduleService) BulkUpsert(competitorID, eventID string, entries []db.CompetitorSchedule) (int, error) {
	if err := s.db.Delete(&db.CompetitorSchedule{},
		"competitor_id = ? AND event_id = ?", competitorID, eventID,
	).Error; err != nil {
		return 0, err
	}

	if len(entries) == 0 {
		return 0, nil
	}

	for i := range entries {
		entries[i].CompetitorID = competitorID
		entries[i].EventID = eventID
	}

	result := s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&entries)
	if result.Error != nil {
		return 0, result.Error
	}
	return int(result.RowsAffected), nil
}

var ErrScheduleNotFound = errors.New("schedule entry not found")
