package service

import (
	"errors"
	"fmt"

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

// scheduleUpdatableColumns is the set of columns a caller may change through
// Update. id, competitor_id and event_id are deliberately absent: an entry's
// ownership is fixed at creation, so a caller can never re-point a row at a
// different competitor or event by naming the column.
var scheduleUpdatableColumns = map[string]bool{
	"instrument":    true,
	"schedule_date": true,
	"schedule_time": true,
	"room":          true,
	"category":      true,
	"division":      true,
	"sort_order":    true,
}

// Update applies only the columns present in updates, leaving every other
// column as it was. Callers must pass column names, not struct fields, and any
// key outside scheduleUpdatableColumns is rejected rather than silently
// dropped so a typo in a handler surfaces as an error instead of a no-op.
//
// This is a partial update on purpose. Writing the whole row from a decoded
// request body turns every PATCH into a full replace: a request that names one
// field zeroes out the ones it omits, which silently destroys the rest of a
// competitor's slot.
func (s *ScheduleService) Update(id string, updates map[string]any) (*db.CompetitorSchedule, error) {
	for column := range updates {
		if !scheduleUpdatableColumns[column] {
			return nil, fmt.Errorf("%w: %s", ErrScheduleColumnNotUpdatable, column)
		}
	}

	var entry db.CompetitorSchedule
	if err := s.db.First(&entry, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrScheduleNotFound
		}
		return nil, err
	}

	if len(updates) > 0 {
		if err := s.db.Model(&db.CompetitorSchedule{}).
			Where("id = ?", entry.ID).
			Updates(updates).Error; err != nil {
			return nil, err
		}
		if err := s.db.First(&entry, "id = ?", id).Error; err != nil {
			return nil, err
		}
	}
	return &entry, nil
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

// scheduleInsertBatchSize caps how many rows go into a single INSERT.
// CompetitorSchedule has ten columns and the Postgres extended protocol allows
// 65535 bind parameters per statement, so an unbatched insert of a few thousand
// rows fails outright. Since the delete below has already removed the previous
// schedule by then, that failure used to leave the competitor with nothing.
const scheduleInsertBatchSize = 500

// BulkUpsert replaces a competitor's schedule for one event: it deletes every
// existing entry for that (competitor_id, event_id) pair, then inserts the new
// batch. Re-running it is therefore safe and does not duplicate rows.
//
// The whole replacement runs in one transaction. Delete-then-insert without one
// is destructive on any insert failure — the delete commits, the insert does
// not, and the competitor's schedule is gone with only a 500 to show for it.
func (s *ScheduleService) BulkUpsert(competitorID, eventID string, entries []db.CompetitorSchedule) (int, error) {
	for i := range entries {
		entries[i].CompetitorID = competitorID
		entries[i].EventID = eventID
	}

	var inserted int64
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&db.CompetitorSchedule{},
			"competitor_id = ? AND event_id = ?", competitorID, eventID,
		).Error; err != nil {
			return err
		}

		if len(entries) == 0 {
			return nil
		}

		result := tx.Clauses(clause.OnConflict{DoNothing: true}).
			CreateInBatches(&entries, scheduleInsertBatchSize)
		if result.Error != nil {
			return result.Error
		}
		inserted = result.RowsAffected
		return nil
	})
	if err != nil {
		return 0, err
	}
	return int(inserted), nil
}

var (
	ErrScheduleNotFound           = errors.New("schedule entry not found")
	ErrScheduleColumnNotUpdatable = errors.New("schedule column is not updatable")
)
