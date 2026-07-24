// import-schedule reads the student-list-master.csv and inserts competitor_schedules
// rows directly into the database. Competitor records are matched by full name.
//
// Usage:
//
//	DATABASE_URL=... go run ./bin/import-schedule --event nat-2026 student-list-master.csv
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm/clause"

	"johndpete316/agm-checkin-api/internal/db"
)

func main() {
	eventID := flag.String("event", "", "event ID to associate schedule entries with (e.g. nat-2026)")
	flag.Parse()

	if *eventID == "" {
		fmt.Fprintln(os.Stderr, "usage: import-schedule --event <event-id> <csv-file>")
		os.Exit(1)
	}
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: import-schedule --event <event-id> <csv-file>")
		os.Exit(1)
	}

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	database := db.Connect(dsn)
	db.AutoMigrate(database)

	f, err := os.Open(flag.Arg(0))
	if err != nil {
		log.Fatalf("opening CSV: %v", err)
	}
	defer f.Close()

	// Load all competitors into a name→ID map for matching.
	var allCompetitors []db.Competitor
	if err := database.Find(&allCompetitors).Error; err != nil {
		log.Fatalf("loading competitors: %v", err)
	}
	byName := make(map[string]string, len(allCompetitors)) // "first|last" → ID
	for _, c := range allCompetitors {
		key := nameKey(c.NameFirst, c.NameLast)
		byName[key] = c.ID
	}

	entries, skipped, errs := parseScheduleCSV(f, *eventID, byName)

	for _, e := range errs {
		fmt.Fprintln(os.Stderr, "WARN:", e)
	}
	fmt.Printf("Matched %d entries, skipped %d rows\n", len(entries), skipped)

	if len(entries) == 0 {
		fmt.Println("Nothing to insert.")
		return
	}

	// Delete any existing schedule entries for this event before inserting.
	if err := database.Delete(&db.CompetitorSchedule{}, "event_id = ?", *eventID).Error; err != nil {
		log.Fatalf("clearing existing schedule entries: %v", err)
	}

	result := database.Clauses(clause.OnConflict{DoNothing: true}).Create(&entries)
	if result.Error != nil {
		log.Fatalf("inserting schedule entries: %v", result.Error)
	}
	fmt.Printf("Inserted %d schedule entries for event %q\n", result.RowsAffected, *eventID)
}

func parseScheduleCSV(r io.Reader, eventID string, byName map[string]string) ([]db.CompetitorSchedule, int, []string) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true

	headers, err := cr.Read()
	if err != nil {
		return nil, 0, []string{"could not read CSV header: " + err.Error()}
	}

	cols := map[string]int{}
	for i, h := range headers {
		cols[strings.TrimSpace(strings.ToLower(h))] = i
	}

	col := func(row []string, name string) string {
		idx, ok := cols[name]
		if !ok || idx >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[idx])
	}

	var entries []db.CompetitorSchedule
	var errs []string
	skipped := 0
	lineNum := 1

	for {
		row, err := cr.Read()
		if err == io.EOF {
			break
		}
		lineNum++
		if err != nil {
			errs = append(errs, fmt.Sprintf("line %d: %v", lineNum, err))
			skipped++
			continue
		}

		name := col(row, "name")
		category := col(row, "category")
		instrument := col(row, "instrument")

		// Skip placeholder/template rows.
		if name == "" || strings.HasPrefix(name, "NULL") ||
			strings.EqualFold(category, "LOADING") || strings.EqualFold(category, "Loading") ||
			strings.EqualFold(instrument, "Loading") {
			skipped++
			continue
		}

		// Resolve the name(s) — duet/trio entries use " & " to join participants.
		var competitorIDs []string
		for _, participant := range strings.Split(name, " & ") {
			id, ok := lookupCompetitor(strings.TrimSpace(participant), byName)
			if ok {
				competitorIDs = append(competitorIDs, id)
			} else {
				errs = append(errs, fmt.Sprintf("line %d: no competitor match for %q (from %q)", lineNum, strings.TrimSpace(participant), name))
			}
		}
		if len(competitorIDs) == 0 {
			skipped++
			continue
		}

		schedDate, err := parseDate(col(row, "day"))
		if err != nil {
			errs = append(errs, fmt.Sprintf("line %d: invalid day %q: %v", lineNum, col(row, "day"), err))
			skipped++
			continue
		}

		timeStr := col(row, "time")
		sortOrder, err := timeToMinutes(timeStr)
		if err != nil {
			errs = append(errs, fmt.Sprintf("line %d: invalid time %q: %v", lineNum, timeStr, err))
			skipped++
			continue
		}

		for _, cid := range competitorIDs {
			entries = append(entries, db.CompetitorSchedule{
				CompetitorID: cid,
				EventID:      eventID,
				Instrument:   instrument,
				ScheduleDate: schedDate,
				ScheduleTime: timeStr,
				Room:         col(row, "room"),
				Category:     category,
				Division:     col(row, "division"),
				SortOrder:    sortOrder,
			})
		}
	}

	return entries, skipped, errs
}

// lookupCompetitor splits a full name on the last space and looks up in the map.
// Returns the competitor UUID and true on success.
func lookupCompetitor(fullName string, byName map[string]string) (string, bool) {
	fullName = strings.TrimSpace(fullName)
	idx := strings.LastIndex(fullName, " ")
	if idx < 0 {
		return "", false
	}
	first := fullName[:idx]
	last := fullName[idx+1:]
	id, ok := byName[nameKey(first, last)]
	return id, ok
}

func nameKey(first, last string) string {
	return strings.ToLower(strings.TrimSpace(first)) + "|" + strings.ToLower(strings.TrimSpace(last))
}

// parseDate parses "M/D/YYYY" into a time.Time at midnight UTC.
func parseDate(s string) (time.Time, error) {
	t, err := time.Parse("1/2/2006", s)
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), nil
}

// timeToMinutes converts "9:00 AM" / "2:45 PM" to minutes since midnight for sort ordering.
func timeToMinutes(s string) (int, error) {
	s = strings.TrimSpace(s)
	parts := strings.Fields(s)
	if len(parts) != 2 {
		return 0, fmt.Errorf("expected \"H:MM AM/PM\", got %q", s)
	}
	hhmm := strings.SplitN(parts[0], ":", 2)
	if len(hhmm) != 2 {
		return 0, fmt.Errorf("expected H:MM, got %q", parts[0])
	}
	h, err := strconv.Atoi(hhmm[0])
	if err != nil {
		return 0, err
	}
	m, err := strconv.Atoi(hhmm[1])
	if err != nil {
		return 0, err
	}
	ampm := strings.ToUpper(parts[1])
	if ampm == "PM" && h != 12 {
		h += 12
	} else if ampm == "AM" && h == 12 {
		h = 0
	}
	return h*60 + m, nil
}
