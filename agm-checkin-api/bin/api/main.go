package main

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"johndpete316/agm-checkin-api/internal/db"
	authmw "johndpete316/agm-checkin-api/internal/middleware"
	"johndpete316/agm-checkin-api/internal/service"
)

// pinVerifier is the subset of service.AuthService used by the createToken handler.
// Defined as an interface to allow testing without a live database.
type pinVerifier interface {
	VerifyPINAndCreateToken(ip, pin, firstName, lastName string) (*db.StaffToken, error)
}

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	pin := os.Getenv("AUTH_PIN")
	if pin == "" {
		log.Fatal("AUTH_PIN environment variable is required")
	}

	database := db.Connect(dsn)
	db.AutoMigrate(database)

	competitorSvc := service.NewCompetitorService(database)
	authSvc := service.NewAuthService(database, pin)
	staffSvc := service.NewStaffService(database)
	eventSvc := service.NewEventService(database)
	auditSvc := service.NewAuditService(database)

	r := chi.NewRouter()
	r.Use(chimw.Logger)

	// Security headers applied to every response.
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Referrer-Policy", "no-referrer")
			next.ServeHTTP(w, r)
		})
	})

	allowedOrigin := os.Getenv("ALLOWED_ORIGIN")
	if allowedOrigin == "" {
		allowedOrigin = "*"
	}
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{allowedOrigin},
		AllowedMethods: []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type", "Accept", "Authorization"},
		MaxAge:         300,
	}))
	r.Use(chimw.Recoverer)

	r.Use(authmw.IPBlocklist(authSvc))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Post("/api/auth/token", createToken(authSvc))

	r.Group(func(r chi.Router) {
		r.Use(authmw.RequireToken(authSvc))

		r.Get("/api/auth/me", func(w http.ResponseWriter, r *http.Request) {
			respondJSON(w, http.StatusOK, authmw.StaffFromContext(r.Context()))
		})

		r.Get("/api/competitors", listCompetitors(competitorSvc))
		r.Get("/api/competitors/{id}", getCompetitor(competitorSvc))
		r.Post("/api/competitors", createCompetitor(competitorSvc, auditSvc))
		r.Patch("/api/competitors/{id}/checkin", checkInCompetitor(competitorSvc, auditSvc))
		r.Patch("/api/competitors/{id}/dob", updateDOB(competitorSvc, auditSvc))
		r.Patch("/api/competitors/{id}/validate", validateCompetitor(competitorSvc, auditSvc))
		r.Delete("/api/competitors/{id}", deleteCompetitor(competitorSvc, auditSvc))
		r.Get("/api/competitors/{id}/events", getCompetitorEvents(competitorSvc))

		r.Get("/api/events", listEvents(eventSvc))
		r.Get("/api/events/current", getCurrentEvent(eventSvc))

		r.Group(func(r chi.Router) {
			r.Use(authmw.RequireAdmin)
			r.Patch("/api/competitors/{id}", updateCompetitor(competitorSvc, auditSvc))

			r.Post("/api/events", createEvent(eventSvc, auditSvc))
			r.Patch("/api/events/{id}/current", setCurrentEvent(eventSvc, auditSvc))

			r.Get("/api/staff", listStaff(staffSvc))
			r.Patch("/api/staff/{id}/role", updateStaffRole(staffSvc, auditSvc))
			r.Delete("/api/staff/{id}", revokeStaff(staffSvc, auditSvc))

			r.Get("/api/audit", listAudit(auditSvc))

			r.Post("/api/competitors/import", bulkImportCompetitors(competitorSvc, auditSvc))
		})
	})

	log.Println("Listening on :8080")
	http.ListenAndServe(":8080", r)
}

// actorFrom extracts actor ID and display name from the request context.
func actorFrom(r *http.Request) (id, name string) {
	if staff := authmw.StaffFromContext(r.Context()); staff != nil {
		return staff.ID, staff.FirstName + " " + staff.LastName
	}
	return "", "unknown"
}

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func createToken(authSvc pinVerifier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Code      string `json:"code"`
			FirstName string `json:"firstName"`
			LastName  string `json:"lastName"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if req.Code == "" || req.FirstName == "" || req.LastName == "" {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "code, firstName, and lastName are required"})
			return
		}

		ip := authmw.GetClientIP(r)
		token, err := authSvc.VerifyPINAndCreateToken(ip, req.Code, req.FirstName, req.LastName)
		if err != nil {
			switch {
			case errors.Is(err, service.ErrIPBlocked):
				respondJSON(w, http.StatusForbidden, map[string]string{"error": "access denied"})
			case errors.Is(err, service.ErrTooManyAttempts):
				respondJSON(w, http.StatusForbidden, map[string]string{"error": "too many failed attempts"})
			case errors.Is(err, service.ErrInvalidPIN):
				respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			default:
				respondJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
			}
			return
		}

		respondJSON(w, http.StatusCreated, map[string]string{
			"token":     token.Token,
			"firstName": token.FirstName,
			"lastName":  token.LastName,
			"role":      token.Role,
		})
	}
}

func listCompetitors(svc *service.CompetitorService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		search := r.URL.Query().Get("search")
		staff := authmw.StaffFromContext(r.Context())
		adminView := staff != nil && staff.Role == "admin"
		competitors, err := svc.GetAll(search, adminView)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		respondJSON(w, http.StatusOK, competitors)
	}
}

func getCompetitor(svc *service.CompetitorService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		competitor, err := svc.GetByID(id)
		if err != nil {
			respondJSON(w, http.StatusNotFound, map[string]string{"error": "competitor not found"})
			return
		}
		respondJSON(w, http.StatusOK, competitor)
	}
}

func createCompetitor(svc *service.CompetitorService, audit *service.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var competitor db.Competitor
		if err := json.NewDecoder(r.Body).Decode(&competitor); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if err := svc.Create(&competitor); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		actorID, actorName := actorFrom(r)
		audit.Log(service.LogEntry{
			ActorID:    actorID,
			ActorName:  actorName,
			Action:     "competitor.created",
			EntityType: "competitor",
			EntityID:   competitor.ID,
			EntityName: competitor.NameFirst + " " + competitor.NameLast,
			Detail:     map[string]any{"studio": competitor.Studio, "lastRegisteredEvent": competitor.LastRegisteredEvent},
			IP:         authmw.GetClientIP(r),
		})
		respondJSON(w, http.StatusCreated, competitor)
	}
}

func checkInCompetitor(svc *service.CompetitorService, audit *service.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		actorID, actorName := actorFrom(r)
		result, err := svc.CheckIn(id, actorName)
		if err != nil {
			if errors.Is(err, service.ErrNoCurrentEvent) {
				respondJSON(w, http.StatusConflict, map[string]string{"error": "no current event is set"})
				return
			}
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		detail := map[string]any{"eventId": result.CurrentCheckIn.EventID}
		audit.Log(service.LogEntry{
			ActorID:    actorID,
			ActorName:  actorName,
			Action:     "competitor.checked_in",
			EntityType: "competitor",
			EntityID:   id,
			EntityName: result.NameFirst + " " + result.NameLast,
			Detail:     detail,
			IP:         authmw.GetClientIP(r),
		})
		respondJSON(w, http.StatusOK, result)
	}
}

func updateDOB(svc *service.CompetitorService, audit *service.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var body struct {
			DateOfBirth time.Time `json:"dateOfBirth"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		competitor, err := svc.UpdateDOB(id, body.DateOfBirth)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		actorID, actorName := actorFrom(r)
		audit.Log(service.LogEntry{
			ActorID:    actorID,
			ActorName:  actorName,
			Action:     "competitor.dob_updated",
			EntityType: "competitor",
			EntityID:   id,
			EntityName: competitor.NameFirst + " " + competitor.NameLast,
			Detail:     map[string]any{"newDob": body.DateOfBirth.Format("2006-01-02")},
			IP:         authmw.GetClientIP(r),
		})
		respondJSON(w, http.StatusOK, competitor)
	}
}

func validateCompetitor(svc *service.CompetitorService, audit *service.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		competitor, err := svc.Validate(id)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		actorID, actorName := actorFrom(r)
		audit.Log(service.LogEntry{
			ActorID:    actorID,
			ActorName:  actorName,
			Action:     "competitor.validated",
			EntityType: "competitor",
			EntityID:   id,
			EntityName: competitor.NameFirst + " " + competitor.NameLast,
			IP:         authmw.GetClientIP(r),
		})
		respondJSON(w, http.StatusOK, competitor)
	}
}

func updateCompetitor(svc *service.CompetitorService, audit *service.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var input db.Competitor
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		competitor, err := svc.Update(id, input)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		actorID, actorName := actorFrom(r)
		audit.Log(service.LogEntry{
			ActorID:    actorID,
			ActorName:  actorName,
			Action:     "competitor.updated",
			EntityType: "competitor",
			EntityID:   id,
			EntityName: competitor.NameFirst + " " + competitor.NameLast,
			Detail: map[string]any{
				"studio":              competitor.Studio,
				"teacher":             competitor.Teacher,
				"lastRegisteredEvent": competitor.LastRegisteredEvent,
			},
			IP: authmw.GetClientIP(r),
		})
		respondJSON(w, http.StatusOK, competitor)
	}
}

func deleteCompetitor(svc *service.CompetitorService, audit *service.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		// Fetch name before deletion for the audit record.
		existing, _ := svc.GetByID(id)
		if err := svc.Delete(id); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		entityName := id
		if existing != nil {
			entityName = existing.NameFirst + " " + existing.NameLast
		}
		actorID, actorName := actorFrom(r)
		audit.Log(service.LogEntry{
			ActorID:    actorID,
			ActorName:  actorName,
			Action:     "competitor.deleted",
			EntityType: "competitor",
			EntityID:   id,
			EntityName: entityName,
			IP:         authmw.GetClientIP(r),
		})
		w.WriteHeader(http.StatusNoContent)
	}
}

func getCompetitorEvents(svc *service.CompetitorService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		history, err := svc.GetEventHistory(id)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		respondJSON(w, http.StatusOK, history)
	}
}

func listEvents(svc *service.EventService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		events, err := svc.List()
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		respondJSON(w, http.StatusOK, events)
	}
}

func getCurrentEvent(svc *service.EventService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		event, err := svc.GetCurrent()
		if err != nil {
			if errors.Is(err, service.ErrNoCurrentEvent) {
				respondJSON(w, http.StatusOK, nil)
				return
			}
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		respondJSON(w, http.StatusOK, event)
	}
}

func createEvent(svc *service.EventService, audit *service.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var event db.Event
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if event.ID == "" || event.Name == "" {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "id and name are required"})
			return
		}
		if err := svc.Create(&event); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		actorID, actorName := actorFrom(r)
		audit.Log(service.LogEntry{
			ActorID:    actorID,
			ActorName:  actorName,
			Action:     "event.created",
			EntityType: "event",
			EntityID:   event.ID,
			EntityName: event.Name,
			Detail:     map[string]any{"startDate": event.StartDate.Format("2006-01-02"), "endDate": event.EndDate.Format("2006-01-02")},
			IP:         authmw.GetClientIP(r),
		})
		respondJSON(w, http.StatusCreated, event)
	}
}

func setCurrentEvent(svc *service.EventService, audit *service.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		event, err := svc.SetCurrent(id)
		if err != nil {
			if errors.Is(err, service.ErrEventNotFound) {
				respondJSON(w, http.StatusNotFound, map[string]string{"error": "event not found"})
				return
			}
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		actorID, actorName := actorFrom(r)
		audit.Log(service.LogEntry{
			ActorID:    actorID,
			ActorName:  actorName,
			Action:     "event.set_current",
			EntityType: "event",
			EntityID:   event.ID,
			EntityName: event.Name,
			IP:         authmw.GetClientIP(r),
		})
		respondJSON(w, http.StatusOK, event)
	}
}

func listStaff(svc *service.StaffService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokens, err := svc.List()
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		respondJSON(w, http.StatusOK, tokens)
	}
}

func updateStaffRole(svc *service.StaffService, audit *service.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		requestor := authmw.StaffFromContext(r.Context())

		var body struct {
			Role string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}

		// Capture old role before the update for the audit detail.
		existing, _ := svc.GetByID(id)
		token, err := svc.UpdateRole(id, body.Role, requestor.ID)
		if err != nil {
			switch {
			case errors.Is(err, service.ErrStaffNotFound):
				respondJSON(w, http.StatusNotFound, map[string]string{"error": "staff token not found"})
			case errors.Is(err, service.ErrInvalidRole):
				respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			case errors.Is(err, service.ErrCannotSelfEdit):
				respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			default:
				respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			}
			return
		}
		detail := map[string]any{"newRole": body.Role}
		if existing != nil {
			detail["oldRole"] = existing.Role
		}
		actorID, actorName := actorFrom(r)
		audit.Log(service.LogEntry{
			ActorID:    actorID,
			ActorName:  actorName,
			Action:     "staff.role_updated",
			EntityType: "staff_token",
			EntityID:   id,
			EntityName: token.FirstName + " " + token.LastName,
			Detail:     detail,
			IP:         authmw.GetClientIP(r),
		})
		respondJSON(w, http.StatusOK, token)
	}
}

func revokeStaff(svc *service.StaffService, audit *service.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		requestor := authmw.StaffFromContext(r.Context())

		existing, _ := svc.GetByID(id)
		if err := svc.Revoke(id, requestor.ID); err != nil {
			switch {
			case errors.Is(err, service.ErrStaffNotFound):
				respondJSON(w, http.StatusNotFound, map[string]string{"error": "staff token not found"})
			case errors.Is(err, service.ErrCannotSelfEdit):
				respondJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			default:
				respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			}
			return
		}
		entityName := id
		if existing != nil {
			entityName = existing.FirstName + " " + existing.LastName
		}
		actorID, actorName := actorFrom(r)
		audit.Log(service.LogEntry{
			ActorID:    actorID,
			ActorName:  actorName,
			Action:     "staff.revoked",
			EntityType: "staff_token",
			EntityID:   id,
			EntityName: entityName,
			IP:         authmw.GetClientIP(r),
		})
		w.WriteHeader(http.StatusNoContent)
	}
}

func listAudit(svc *service.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		action := r.URL.Query().Get("action")
		actorName := r.URL.Query().Get("actor")
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		logs, err := svc.List(action, actorName, limit)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		respondJSON(w, http.StatusOK, logs)
	}
}

// bulkImportCompetitors accepts a multipart CSV upload (field name "file") containing a
// normalized competitor import file and inserts all records into the database.
// A Postgres snapshot is taken before any writes so the import can be rolled back if needed.
func bulkImportCompetitors(svc *service.CompetitorService, audit *service.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 32 MB max upload.
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
			IP: authmw.GetClientIP(r),
		})

		respondJSON(w, http.StatusOK, result)
	}
}

// parseImportCSV reads a normalized import CSV and returns the parsed rows and any
// non-fatal row-level errors. A missing header row or completely unreadable file
// returns only errors with no rows.
func parseImportCSV(r io.Reader) ([]service.ImportRow, []string) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true

	headers, err := cr.Read()
	if err != nil {
		return nil, []string{"could not read CSV header: " + err.Error()}
	}

	// Build column index map from the normalized header.
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
			continue // skip blank rows
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
