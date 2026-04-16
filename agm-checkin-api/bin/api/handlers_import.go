package main

import (
	"encoding/csv"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	authmw "johndpete316/agm-checkin-api/internal/middleware"
	"johndpete316/agm-checkin-api/internal/service"
)

func bulkImportCompetitors(svc *service.CompetitorService, audit *service.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart form"})
			return
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "missing file field"})
			return
		}
		defer file.Close()

		rows, parseErrors := parseImportCSV(file)
		if len(parseErrors) > 0 && len(rows) == 0 {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": parseErrors[0]})
			return
		}

		result, err := svc.BulkImport(rows)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		result.Errors = append(result.Errors, parseErrors...)

		actorID, actorName := actorFrom(r)
		audit.Log(service.LogEntry{
			ActorID:    actorID,
			ActorName:  actorName,
			Action:     "competitor.bulk_import",
			EntityType: "competitor",
			EntityID:   "bulk",
			EntityName: "bulk import",
			Detail: map[string]any{
				"competitorsCreated": result.CompetitorsCreated,
				"eventsCreated":      result.EventsCreated,
				"eventEntriesAdded":  result.EventEntriesAdded,
			},
			IP: authmw.ClientIP(r),
		})

		respondJSON(w, http.StatusOK, result)
	}
}

func parseImportCSV(r io.Reader) ([]service.ImportRow, []string) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true

	headers, err := cr.Read()
	if err != nil {
		return nil, []string{"could not read CSV header: " + err.Error()}
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

	var rows []service.ImportRow
	var errs []string
	lineNum := 1

	for {
		row, err := cr.Read()
		if err == io.EOF {
			break
		}
		lineNum++
		if err != nil {
			errs = append(errs, "line "+strconv.Itoa(lineNum)+": "+err.Error())
			continue
		}

		first := col(row, "first_name")
		last := col(row, "last_name")
		if first == "" || last == "" {
			continue
		}

		var dob *time.Time
		if dobStr := col(row, "date_of_birth"); dobStr != "" {
			t, err := time.Parse("2006-01-02", dobStr)
			if err != nil {
				errs = append(errs, "line "+strconv.Itoa(lineNum)+": invalid date_of_birth "+dobStr)
			} else {
				dob = &t
			}
		}

		requiresValidation, _ := strconv.ParseBool(col(row, "requires_validation"))
		validated, _ := strconv.ParseBool(col(row, "validated"))

		var events []string
		if evStr := col(row, "events"); evStr != "" {
			for _, e := range strings.Split(evStr, "|") {
				if e = strings.TrimSpace(e); e != "" {
					events = append(events, e)
				}
			}
		}

		rows = append(rows, service.ImportRow{
			NameFirst:          first,
			NameLast:           last,
			Studio:             col(row, "studio"),
			Teacher:            col(row, "teacher"),
			Email:              col(row, "email"),
			ShirtSize:          col(row, "shirt_size"),
			DateOfBirth:        dob,
			RequiresValidation: requiresValidation,
			Validated:          validated,
			Events:             events,
		})
	}

	return rows, errs
}
