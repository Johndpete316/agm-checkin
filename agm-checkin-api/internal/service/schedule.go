package service

import (
	"errors"
	"time"

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

// EventScheduleEntry is a schedule row flattened with the competitor's name, so
// the schedule can be browsed event-wide without an N+1 lookup per row.
type EventScheduleEntry struct {
	ID           string    `json:"id"`
	CompetitorID string    `json:"competitorId"`
	EventID      string    `json:"eventId"`
	NameFirst    string    `json:"nameFirst"`
	NameLast     string    `json:"nameLast"`
	Instrument   string    `json:"instrument"`
	ScheduleDate time.Time `json:"scheduleDate"`
	ScheduleTime string    `json:"scheduleTime"`
	Room         string    `json:"room"`
	Category     string    `json:"category"`
	Division     string    `json:"division"`
	PageNumber   string    `json:"pageNumber"`
	SortOrder    int       `json:"sortOrder"`
}

// GetByEvent returns every schedule entry for an event in chronological order.
// Grouping (by room, day, instrument, category) is left to the caller — the
// whole event is a few thousand rows at most, so it ships in one response.
func (s *ScheduleService) GetByEvent(eventID string) ([]EventScheduleEntry, error) {
	entries := []EventScheduleEntry{}
	if err := s.db.
		Table("competitor_schedules AS cs").
		Select(`cs.id, cs.competitor_id, cs.event_id, cs.instrument,
			cs.schedule_date, cs.schedule_time, cs.room, cs.category,
			cs.division, cs.page_number, cs.sort_order,
			c.name_first, c.name_last`).
		Joins("JOIN competitors c ON c.id = cs.competitor_id").
		Where("cs.event_id = ?", eventID).
		Order("cs.schedule_date ASC, cs.sort_order ASC, cs.room ASC, c.name_last ASC").
		Scan(&entries).Error; err != nil {
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
