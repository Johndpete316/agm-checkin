package service

import (
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"johndpete316/agm-checkin-api/internal/db"
)

// CompetitorWithCheckIn is the standard list/detail response — the competitor record
// plus their check-in record for the current event (nil if not yet checked in).
type CompetitorWithCheckIn struct {
	db.Competitor
	CurrentCheckIn *db.CompetitorEvent `json:"currentCheckIn"`
}

// CompetitorEventWithEvent is used for the per-competitor history endpoint.
type CompetitorEventWithEvent struct {
	db.CompetitorEvent
	Event db.Event `json:"event"`
}

type CompetitorService struct {
	db *gorm.DB
}

func NewCompetitorService(database *gorm.DB) *CompetitorService {
	return &CompetitorService{db: database}
}

// currentEventID returns the ID of the current event, or "" if none is set.
func (s *CompetitorService) currentEventID() string {
	var event db.Event
	if err := s.db.Where("is_current = true").First(&event).Error; err != nil {
		return ""
	}
	return event.ID
}

func (s *CompetitorService) GetAll(search string, adminView bool) ([]CompetitorWithCheckIn, error) {
	eventID := s.currentEventID()

	query := s.db.Model(&db.Competitor{})

	// Registration users only see competitors registered for the current event.
	if !adminView {
		if eventID == "" {
			return []CompetitorWithCheckIn{}, nil
		}
		query = query.Where("last_registered_event = ?", eventID)
	}

	if search != "" {
		like := "%" + search + "%"
		query = query.Where(
			"name_first ILIKE ? OR name_last ILIKE ? OR CONCAT(name_first, ' ', name_last) ILIKE ?",
			like, like, like,
		)
	}

	var competitors []db.Competitor
	if err := query.Find(&competitors).Error; err != nil {
		return nil, err
	}

	if len(competitors) == 0 {
		return []CompetitorWithCheckIn{}, nil
	}

	// Attach current-event check-in records.
	checkInMap := map[string]*db.CompetitorEvent{}
	if eventID != "" {
		ids := make([]string, len(competitors))
		for i, c := range competitors {
			ids[i] = c.ID
		}
		var checkIns []db.CompetitorEvent
		s.db.Where("competitor_id IN ? AND event_id = ?", ids, eventID).Find(&checkIns)
		for i := range checkIns {
			ce := checkIns[i]
			checkInMap[ce.CompetitorID] = &ce
		}
	}

	result := make([]CompetitorWithCheckIn, len(competitors))
	for i, c := range competitors {
		result[i] = CompetitorWithCheckIn{
			Competitor:     c,
			CurrentCheckIn: checkInMap[c.ID],
		}
	}
	return result, nil
}

func (s *CompetitorService) GetByID(id string) (*CompetitorWithCheckIn, error) {
	var competitor db.Competitor
	if err := s.db.First(&competitor, "id = ?", id).Error; err != nil {
		return nil, err
	}

	result := &CompetitorWithCheckIn{Competitor: competitor}

	eventID := s.currentEventID()
	if eventID != "" {
		var ce db.CompetitorEvent
		if err := s.db.Where("competitor_id = ? AND event_id = ?", id, eventID).First(&ce).Error; err == nil {
			result.CurrentCheckIn = &ce
		}
	}
	return result, nil
}

func (s *CompetitorService) Create(competitor *db.Competitor) error {
	return s.db.Create(competitor).Error
}

func (s *CompetitorService) CheckIn(id string, staffName string) (*CompetitorWithCheckIn, error) {
	var competitor db.Competitor
	if err := s.db.First(&competitor, "id = ?", id).Error; err != nil {
		return nil, err
	}

	eventID := s.currentEventID()
	if eventID == "" {
		return nil, ErrNoCurrentEvent
	}

	now := time.Now()
	ce := db.CompetitorEvent{
		CompetitorID:    id,
		EventID:         eventID,
		CheckedIn:       true,
		CheckInDatetime: &now,
		CheckedInBy:     staffName,
	}

	if err := s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "competitor_id"}, {Name: "event_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"checked_in", "check_in_datetime", "checked_in_by"}),
	}).Create(&ce).Error; err != nil {
		return nil, err
	}

	// Re-fetch to get the ID if it was an update (upsert may not populate ID on conflict path).
	s.db.Where("competitor_id = ? AND event_id = ?", id, eventID).First(&ce)

	return &CompetitorWithCheckIn{Competitor: competitor, CurrentCheckIn: &ce}, nil
}

func (s *CompetitorService) UpdateDOB(id string, dob time.Time) (*db.Competitor, error) {
	var competitor db.Competitor
	if err := s.db.First(&competitor, "id = ?", id).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&competitor).Update("date_of_birth", dob).Error; err != nil {
		return nil, err
	}
	return &competitor, nil
}

func (s *CompetitorService) Validate(id string) (*db.Competitor, error) {
	var competitor db.Competitor
	if err := s.db.First(&competitor, "id = ?", id).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&competitor).Update("validated", true).Error; err != nil {
		return nil, err
	}
	competitor.Validated = true
	return &competitor, nil
}

func (s *CompetitorService) Update(id string, input db.Competitor) (*db.Competitor, error) {
	var competitor db.Competitor
	if err := s.db.First(&competitor, "id = ?", id).Error; err != nil {
		return nil, err
	}
	input.ID = competitor.ID
	if err := s.db.Save(&input).Error; err != nil {
		return nil, err
	}
	return &input, nil
}

func (s *CompetitorService) Delete(id string) error {
	return s.db.Delete(&db.Competitor{}, "id = ?", id).Error
}

func (s *CompetitorService) GetEventHistory(competitorID string) ([]CompetitorEventWithEvent, error) {
	var entries []db.CompetitorEvent
	if err := s.db.Where("competitor_id = ?", competitorID).Find(&entries).Error; err != nil {
		return nil, err
	}

	if len(entries) == 0 {
		return []CompetitorEventWithEvent{}, nil
	}

	eventIDs := make([]string, len(entries))
	for i, e := range entries {
		eventIDs[i] = e.EventID
	}

	var events []db.Event
	s.db.Where("id IN ?", eventIDs).Find(&events)

	eventMap := make(map[string]db.Event)
	for _, e := range events {
		eventMap[e.ID] = e
	}

	// Sort by event date descending via the event map.
	result := make([]CompetitorEventWithEvent, len(entries))
	for i, entry := range entries {
		result[i] = CompetitorEventWithEvent{
			CompetitorEvent: entry,
			Event:           eventMap[entry.EventID],
		}
	}
	return result, nil
}

// ErrNotFound is returned when a competitor record does not exist.
var ErrNotFound = errors.New("competitor not found")
