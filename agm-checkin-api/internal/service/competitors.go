package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"johndpete316/agm-checkin-api/internal/db"
)

// ImportRow represents one row from a normalized import CSV.
type ImportRow struct {
	NameFirst          string
	NameLast           string
	Studio             string
	Teacher            string
	Email              string
	ShirtSize          string
	DateOfBirth        *time.Time // nil if unknown
	RequiresValidation bool
	Validated          bool
	Events             []string // event IDs sorted oldest→newest, e.g. ["nat-2024","glr-2026"]
}

// ImportResult summarises what was inserted during a BulkImport call.
type ImportResult struct {
	CompetitorsCreated int            `json:"competitorsCreated"`
	CompetitorsMatched int            `json:"competitorsMatched"`
	FieldsUpdated      int            `json:"fieldsUpdated"`
	EventsCreated      int            `json:"eventsCreated"`
	EventEntriesAdded  int            `json:"eventEntriesAdded"`
	FieldConflicts     []FieldConflict `json:"fieldConflicts,omitempty"`
	Errors             []string       `json:"errors,omitempty"`
}

// FieldConflict is returned when an import row has a value for a field that differs from the
// existing record. Both sides are non-blank. The caller (UI) can resolve it by choosing which
// value to keep. ExistingValue and ImportValue are always plain strings; dates use YYYY-MM-DD.
type FieldConflict struct {
	CompetitorID  string `json:"competitorId"`
	Name          string `json:"name"`
	Field         string `json:"field"`         // JSON field name: "email", "studio", "teacher", "shirtSize", "dateOfBirth"
	ExistingValue string `json:"existingValue"`
	ImportValue   string `json:"importValue"`
}

// eventChronology ranks every known event by start date, oldest first. Reading
// this from the events table rather than a hardcoded list is the point: adding
// an event is a data change, not a code change. Stub events imported without
// dates carry the zero time and therefore rank oldest, so a dated event always
// wins — which is what we want, since only the two current events have real
// dates on file.
func (s *CompetitorService) eventChronology() (map[string]int, error) {
	var events []db.Event
	if err := s.db.Order("start_date ASC, id ASC").Find(&events).Error; err != nil {
		return nil, fmt.Errorf("loading event chronology: %w", err)
	}
	rank := make(map[string]int, len(events))
	for i, e := range events {
		rank[e.ID] = i
	}
	return rank, nil
}

// mostRecentEvent returns the latest of the given event IDs. Events absent from
// rank are unknown to the database and ignored.
func mostRecentEvent(events []string, rank map[string]int) string {
	best := ""
	bestRank := 0
	for _, e := range events {
		r, known := rank[e]
		if !known {
			continue
		}
		if best == "" || r > bestRank {
			bestRank = r
			best = e
		}
	}
	return best
}

// nameKey returns a normalised lookup key for a competitor name.
func nameKey(first, last string) string {
	return strings.ToLower(strings.TrimSpace(first)) + "|" + strings.ToLower(strings.TrimSpace(last))
}

// BulkImport creates Postgres snapshot backup tables, then processes all rows from the CSV.
// Rows whose (first_name, last_name) match an existing competitor are merged rather than
// duplicated: a missing DOB is filled in automatically, a conflicting DOB is returned in
// DOBConflicts for the caller to resolve via the UI. Rows with no existing match create new
// competitor records. Stub event records are auto-created for any event ID not yet in the DB.
func (s *CompetitorService) BulkImport(rows []ImportRow) (*ImportResult, error) {
	result := &ImportResult{}
	importedAt := time.Now()

	// --- 1. Backup existing tables so the import can be rolled back if needed. ---
	ts := importedAt.Unix()
	backupSQL := fmt.Sprintf(`
		CREATE TABLE competitors_backup_%d AS SELECT * FROM competitors;
		CREATE TABLE competitor_events_backup_%d AS SELECT * FROM competitor_events;
	`, ts, ts)
	if err := s.db.Exec(backupSQL).Error; err != nil {
		return nil, fmt.Errorf("creating backup tables: %w", err)
	}

	// --- 2. Load all existing competitors and build a name → []Competitor lookup map. ---
	var allExisting []db.Competitor
	if err := s.db.Find(&allExisting).Error; err != nil {
		return nil, fmt.Errorf("loading existing competitors: %w", err)
	}
	existingByName := make(map[string][]db.Competitor, len(allExisting))
	for _, c := range allExisting {
		k := nameKey(c.NameFirst, c.NameLast)
		existingByName[k] = append(existingByName[k], c)
	}

	// --- 3. Collect all referenced event IDs and auto-create stubs for missing ones. ---
	eventSet := map[string]bool{}
	for _, row := range rows {
		for _, eid := range row.Events {
			eventSet[eid] = true
		}
	}
	for eid := range eventSet {
		var existing db.Event
		if err := s.db.First(&existing, "id = ?", eid).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, fmt.Errorf("checking event %s: %w", eid, err)
			}
			// Event doesn't exist — create a stub. Dates are left as zero; admin fills them in later.
			name := eventDisplayName(eid)
			stub := db.Event{ID: eid, Name: name}
			if err := s.db.Create(&stub).Error; err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("create event %s: %v", eid, err))
				continue
			}
			result.EventsCreated++
		}
	}

	// Loaded after stub creation so freshly created events are ranked too.
	eventRank, err := s.eventChronology()
	if err != nil {
		return nil, err
	}

	// --- 4. Classify each row: match existing competitor or create new. ---
	// resolvedIDs maps import-row index → the competitor ID to use for event linking.
	resolvedIDs := make([]string, len(rows))
	var toCreate []db.Competitor
	var toCreateIdx []int // maps toCreate[i] back to rows index

	for i, row := range rows {
		k := nameKey(row.NameFirst, row.NameLast)
		matches := existingByName[k]

		if len(matches) > 1 {
			// Ambiguous — multiple existing competitors share this name. Skip to avoid
			// incorrectly updating the wrong record.
			result.Errors = append(result.Errors, fmt.Sprintf(
				"%s %s: skipped — %d competitors with this name exist; resolve manually",
				row.NameFirst, row.NameLast, len(matches),
			))
			continue
		}

		if len(matches) == 1 {
			// Matched — merge all mergeable fields.
			existing := matches[0]
			result.CompetitorsMatched++
			resolvedIDs[i] = existing.ID

			fullName := existing.NameFirst + " " + existing.NameLast
			autoFill := map[string]any{} // fields to auto-update (existing blank, import has value)

			// String fields: blank→fill auto; both set and differ→conflict.
			type stringField struct {
				jsonKey  string
				dbCol    string
				existing string
				incoming string
			}
			stringFields := []stringField{
				{"email", "email", existing.Email, row.Email},
				{"studio", "studio", existing.Studio, row.Studio},
				{"teacher", "teacher", existing.Teacher, row.Teacher},
			}
			for _, f := range stringFields {
				if f.incoming == "" {
					continue // import has nothing — leave existing alone
				}
				if f.existing == "" {
					autoFill[f.dbCol] = f.incoming
				} else if f.existing != f.incoming {
					result.FieldConflicts = append(result.FieldConflicts, FieldConflict{
						CompetitorID:  existing.ID,
						Name:          fullName,
						Field:         f.jsonKey,
						ExistingValue: f.existing,
						ImportValue:   f.incoming,
					})
				}
			}

			// Shirt size is always overwritten when the import has a value —
			// it's event-specific and the latest registration is authoritative.
			if row.ShirtSize != "" {
				autoFill["shirt_size"] = row.ShirtSize
			}

			// Update lastRegisteredEvent when incoming events include something newer.
			if incoming := mostRecentEvent(row.Events, eventRank); incoming != "" &&
				eventRank[incoming] > eventRank[existing.LastRegisteredEvent] {
				autoFill["last_registered_event"] = incoming
			}

			// Date of birth: zero→fill auto; both set and differ→conflict.
			if row.DateOfBirth != nil {
				importDOB := row.DateOfBirth.UTC().Truncate(24 * time.Hour)
				if existing.DateOfBirth.IsZero() {
					autoFill["date_of_birth"] = importDOB
				} else {
					existingDOB := existing.DateOfBirth.UTC().Truncate(24 * time.Hour)
					if !existingDOB.Equal(importDOB) {
						result.FieldConflicts = append(result.FieldConflicts, FieldConflict{
							CompetitorID:  existing.ID,
							Name:          fullName,
							Field:         "dateOfBirth",
							ExistingValue: existingDOB.Format("2006-01-02"),
							ImportValue:   importDOB.Format("2006-01-02"),
						})
					}
				}
			}

			if len(autoFill) > 0 {
				if err := s.db.Model(&existing).Updates(autoFill).Error; err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf(
						"%s: failed to auto-fill fields: %v", fullName, err,
					))
				} else {
					result.FieldsUpdated += len(autoFill)
				}
			}
			continue
		}

		// No match — queue for creation.
		dob := time.Time{}
		if row.DateOfBirth != nil {
			dob = *row.DateOfBirth
		}

		// A CSV can assert a competitor was verified, but not that we hold the
		// date it was verified against — without a DOB the claim is unusable, so
		// they get checked at the desk instead.
		var verifiedAt *time.Time
		verifiedBy := ""
		if row.Validated && !dob.IsZero() {
			verifiedAt = &importedAt
			verifiedBy = "historical import"
		}

		toCreate = append(toCreate, db.Competitor{
			NameFirst:           strings.TrimSpace(row.NameFirst),
			NameLast:            strings.TrimSpace(row.NameLast),
			Studio:              row.Studio,
			Teacher:             row.Teacher,
			Email:               row.Email,
			ShirtSize:           row.ShirtSize,
			DateOfBirth:         dob,
			DobVerifiedAt:       verifiedAt,
			DobVerifiedBy:       verifiedBy,
			RequiresValidation:  row.RequiresValidation,
			Validated:           row.Validated,
			LastRegisteredEvent: mostRecentEvent(row.Events, eventRank),
		})
		toCreateIdx = append(toCreateIdx, i)
	}

	// --- 5. Bulk-insert new competitors. ---
	if len(toCreate) > 0 {
		if err := s.db.Create(&toCreate).Error; err != nil {
			return nil, fmt.Errorf("bulk inserting competitors: %w", err)
		}
		result.CompetitorsCreated = len(toCreate)
		// Back-fill resolvedIDs for newly created records.
		for j, rowIdx := range toCreateIdx {
			resolvedIDs[rowIdx] = toCreate[j].ID
		}
	}

	// --- 6. Bulk-insert CompetitorEvent rows (one per competitor × event). ---
	var ces []db.CompetitorEvent
	for i, row := range rows {
		if resolvedIDs[i] == "" {
			continue // row was skipped (ambiguous match or creation error)
		}
		for _, eid := range row.Events {
			if _, exists := eventSet[eid]; !exists {
				continue // event creation failed; skip
			}
			ces = append(ces, db.CompetitorEvent{
				CompetitorID: resolvedIDs[i],
				EventID:      eid,
				CheckedIn:    false,
			})
		}
	}

	if len(ces) > 0 {
		// ON CONFLICT DO NOTHING — safe to re-run if partially completed.
		ceResult := s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&ces)
		if ceResult.Error != nil {
			return nil, fmt.Errorf("bulk inserting competitor events: %w", ceResult.Error)
		}
		result.EventEntriesAdded = int(ceResult.RowsAffected)
	}

	return result, nil
}

// eventDisplayName converts an event slug into a human-readable name for stub events.
func eventDisplayName(id string) string {
	parts := strings.SplitN(id, "-", 2)
	if len(parts) != 2 {
		return id
	}
	prefix := strings.ToUpper(parts[0])
	year := parts[1]
	switch prefix {
	case "GLR":
		return "GLR " + year
	case "NAT":
		return "Nationals " + year
	default:
		return prefix + " " + year
	}
}

// CompetitorWithCheckIn is the standard list/detail response — the competitor record
// plus their check-in record for the current event (nil if not yet checked in).
type CompetitorWithCheckIn struct {
	db.Competitor
	CurrentCheckIn *db.CompetitorEvent `json:"currentCheckIn"`

	// MostRecentEvent is derived from competitor_events on every read rather
	// than stored, so it cannot drift from the attendance rows the way
	// LastRegisteredEvent did.
	MostRecentEvent string `json:"mostRecentEvent"`
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
	// Registration is an attendance row, not a column on the competitor: a
	// single text field cannot express being registered for two events at once,
	// which is what forced a full re-import to open every new event.
	if !adminView {
		if eventID == "" {
			return []CompetitorWithCheckIn{}, nil
		}
		query = query.Where(
			"EXISTS (SELECT 1 FROM competitor_events ce WHERE ce.competitor_id = competitors.id AND ce.event_id = ?)",
			eventID,
		)
	}

	if search != "" {
		like := "%" + search + "%"
		query = query.Where(
			"name_first ILIKE ? OR name_last ILIKE ? OR CONCAT(name_first, ' ', name_last) ILIKE ?",
			like, like, like,
		).Order(clause.Expr{
			SQL:  "CASE WHEN name_last ILIKE ? THEN 0 WHEN name_first ILIKE ? THEN 1 ELSE 2 END, name_last, name_first",
			Vars: []interface{}{like, like},
		})
	}

	var competitors []db.Competitor
	if err := query.Find(&competitors).Error; err != nil {
		return nil, err
	}

	if len(competitors) == 0 {
		return []CompetitorWithCheckIn{}, nil
	}

	ids := make([]string, len(competitors))
	for i, c := range competitors {
		ids[i] = c.ID
	}

	// Attach current-event check-in records.
	checkInMap := map[string]*db.CompetitorEvent{}
	if eventID != "" {
		var checkIns []db.CompetitorEvent
		if err := s.db.Where("competitor_id IN ? AND event_id = ?", ids, eventID).Find(&checkIns).Error; err != nil {
			return nil, err
		}
		for i := range checkIns {
			ce := checkIns[i]
			checkInMap[ce.CompetitorID] = &ce
		}
	}

	recentMap, err := s.mostRecentEvents(ids)
	if err != nil {
		return nil, err
	}

	result := make([]CompetitorWithCheckIn, len(competitors))
	for i, c := range competitors {
		result[i] = CompetitorWithCheckIn{
			Competitor:      c,
			CurrentCheckIn:  checkInMap[c.ID],
			MostRecentEvent: recentMap[c.ID],
		}
	}
	return result, nil
}

// mostRecentEvents resolves each competitor's latest event by start date. Stub
// events with no date on file carry the zero time and so lose to any dated
// event, which matches how the roster is actually ordered.
func (s *CompetitorService) mostRecentEvents(competitorIDs []string) (map[string]string, error) {
	if len(competitorIDs) == 0 {
		return map[string]string{}, nil
	}
	var rows []struct {
		CompetitorID string
		EventID      string
	}
	if err := s.db.Raw(`
		SELECT DISTINCT ON (ce.competitor_id) ce.competitor_id, ce.event_id
		FROM competitor_events ce
		JOIN events e ON e.id = ce.event_id
		WHERE ce.competitor_id IN ?
		ORDER BY ce.competitor_id, e.start_date DESC, ce.event_id DESC
	`, competitorIDs).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("resolving most recent events: %w", err)
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.CompetitorID] = r.EventID
	}
	return out, nil
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

	recentMap, err := s.mostRecentEvents([]string{competitor.ID})
	if err != nil {
		return nil, err
	}
	result.MostRecentEvent = recentMap[competitor.ID]
	return result, nil
}

func (s *CompetitorService) Create(competitor *db.Competitor, staffName string) error {
	applyDobVerification(competitor, nil, staffName)
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(competitor).Error; err != nil {
			return err
		}
		return registerForEvent(tx, competitor.ID, competitor.LastRegisteredEvent)
	})
}

// registerForEvent puts a competitor on an event roster if they are not on it
// already. Attendance rows are what make someone visible to registration staff,
// so adding or editing a competitor has to write one — setting the event field
// alone would leave them registered on paper and invisible in the app.
func registerForEvent(tx *gorm.DB, competitorID, eventID string) error {
	if eventID == "" {
		return nil
	}
	var known int64
	if err := tx.Model(&db.Event{}).Where("id = ?", eventID).Count(&known).Error; err != nil {
		return fmt.Errorf("checking event %s: %w", eventID, err)
	}
	if known == 0 {
		return fmt.Errorf("%w: %s", ErrUnknownEvent, eventID)
	}
	ce := db.CompetitorEvent{CompetitorID: competitorID, EventID: eventID}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&ce).Error
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

	// Both writes in one transaction: a check-in recorded without its matching
	// registration update is the drift this conversion exists to remove
	// (DB_REVIEW finding 3).
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "competitor_id"}, {Name: "event_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"checked_in", "check_in_datetime", "checked_in_by"}),
		}).Create(&ce).Error; err != nil {
			return err
		}

		// Reload the stored row. On the conflict path BeforeCreate has already
		// overwritten ce.ID with a freshly generated UUID that was never persisted,
		// and GORM would otherwise use that stale primary key as a query condition.
		ce.ID = ""
		if err := tx.Where("competitor_id = ? AND event_id = ?", id, eventID).First(&ce).Error; err != nil {
			return fmt.Errorf("reloading check-in record: %w", err)
		}

		// Nothing reads last_registered_event any more, but it is kept current so
		// this phase can be rolled back without stranding anyone checked in during
		// the window. Migration 004 drops the column.
		if competitor.LastRegisteredEvent != eventID {
			if err := tx.Model(&competitor).Update("last_registered_event", eventID).Error; err != nil {
				return fmt.Errorf("updating last registered event: %w", err)
			}
			competitor.LastRegisteredEvent = eventID
		}
		return nil
	}); err != nil {
		return nil, err
	}

	recentMap, err := s.mostRecentEvents([]string{id})
	if err != nil {
		return nil, err
	}

	return &CompetitorWithCheckIn{
		Competitor:      competitor,
		CurrentCheckIn:  &ce,
		MostRecentEvent: recentMap[id],
	}, nil
}

func (s *CompetitorService) UpdateDOB(id string, dob time.Time) (*db.Competitor, error) {
	var competitor db.Competitor
	if err := s.db.First(&competitor, "id = ?", id).Error; err != nil {
		return nil, err
	}
	if err := s.db.Model(&competitor).Update("date_of_birth", dob).Error; err != nil {
		return nil, err
	}
	competitor.DateOfBirth = dob
	return &competitor, nil
}

// Validate records that staff confirmed this competitor's date of birth against
// ID. It is idempotent: re-verifying an already-verified competitor keeps the
// original timestamp and staff name rather than overwriting the provenance.
func (s *CompetitorService) Validate(id, staffName string) (*db.Competitor, error) {
	var competitor db.Competitor
	if err := s.db.First(&competitor, "id = ?", id).Error; err != nil {
		return nil, err
	}
	if competitor.DobVerifiedAt != nil {
		return &competitor, nil
	}
	// Postgres timestamptz holds microseconds; truncate so the value we return
	// matches the value that was actually stored.
	now := time.Now().Truncate(time.Microsecond)
	if err := s.db.Model(&competitor).Updates(map[string]any{
		"dob_verified_at": now,
		"dob_verified_by": staffName,
		"validated":       true,
	}).Error; err != nil {
		return nil, err
	}
	competitor.DobVerifiedAt = &now
	competitor.DobVerifiedBy = staffName
	competitor.Validated = true
	return &competitor, nil
}

func (s *CompetitorService) UpdateContact(id string, note *string, email *string) (*db.Competitor, error) {
	var competitor db.Competitor
	if err := s.db.First(&competitor, "id = ?", id).Error; err != nil {
		return nil, err
	}
	updates := map[string]any{}
	if note != nil {
		updates["note"] = *note
	}
	if email != nil {
		updates["email"] = *email
	}
	if len(updates) == 0 {
		return &competitor, nil
	}
	if err := s.db.Model(&competitor).Updates(updates).Error; err != nil {
		return nil, err
	}
	if note != nil {
		competitor.Note = *note
	}
	if email != nil {
		competitor.Email = *email
	}
	return &competitor, nil
}

func (s *CompetitorService) Update(id string, input db.Competitor, staffName string) (*db.Competitor, error) {
	var competitor db.Competitor
	if err := s.db.First(&competitor, "id = ?", id).Error; err != nil {
		return nil, err
	}
	input.ID = competitor.ID
	applyDobVerification(&input, &competitor, staffName)
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&input).Error; err != nil {
			return err
		}
		return registerForEvent(tx, input.ID, input.LastRegisteredEvent)
	}); err != nil {
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
	if err := s.db.Where("id IN ?", eventIDs).Find(&events).Error; err != nil {
		return nil, err
	}

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

// ErrUnknownEvent is returned when a competitor is assigned to an event ID that
// does not exist, which would otherwise surface as a foreign key violation.
var ErrUnknownEvent = errors.New("unknown event")

// applyDobVerification resolves the verification state the client asked for
// against what is already stored. The client only gets to say whether the
// competitor is verified; when and by whom are always decided here, so a caller
// cannot forge provenance. An existing verification is never re-stamped.
func applyDobVerification(input *db.Competitor, existing *db.Competitor, staffName string) {
	var stored *db.Competitor
	if existing != nil {
		stored = existing
	}
	switch {
	case input.DobVerifiedAt == nil:
		input.DobVerifiedBy = ""
	case stored != nil && stored.DobVerifiedAt != nil:
		input.DobVerifiedAt = stored.DobVerifiedAt
		input.DobVerifiedBy = stored.DobVerifiedBy
	default:
		// Postgres timestamptz holds microseconds; truncate so the value we
		// return matches the value that was actually stored.
		now := time.Now().Truncate(time.Microsecond)
		input.DobVerifiedAt = &now
		input.DobVerifiedBy = staffName
	}
	input.Validated = input.DobVerifiedAt != nil
	input.RequiresValidation = input.DobVerifiedAt == nil
}
