package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"johndpete316/agm-checkin-api/internal/db"
	authmw "johndpete316/agm-checkin-api/internal/middleware"
	"johndpete316/agm-checkin-api/internal/service"
)

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
			IP:         authmw.ClientIP(r),
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
			IP:         authmw.ClientIP(r),
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
			IP:         authmw.ClientIP(r),
		})
		respondJSON(w, http.StatusOK, competitor)
	}
}

func updateCompetitorContact(svc *service.CompetitorService, audit *service.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var body struct {
			Note  *string `json:"note"`
			Email *string `json:"email"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		competitor, err := svc.UpdateContact(id, body.Note, body.Email)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		actorID, actorName := actorFrom(r)
		audit.Log(service.LogEntry{
			ActorID:    actorID,
			ActorName:  actorName,
			Action:     "competitor.contact_updated",
			EntityType: "competitor",
			EntityID:   id,
			EntityName: competitor.NameFirst + " " + competitor.NameLast,
			IP:         authmw.ClientIP(r),
		})
		respondJSON(w, http.StatusOK, competitor)
	}
}

func validateCompetitor(svc *service.CompetitorService, audit *service.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		competitor, err := svc.Validate(id)
		if err != nil {
			switch {
			case errors.Is(err, service.ErrValidationNotRequired):
				respondJSON(w, http.StatusConflict, map[string]string{
					"error": "competitor does not require identity validation",
				})
			default:
				respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			}
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
			IP:         authmw.ClientIP(r),
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
			IP: authmw.ClientIP(r),
		})
		respondJSON(w, http.StatusOK, competitor)
	}
}

func deleteCompetitor(svc *service.CompetitorService, audit *service.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
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
			IP:         authmw.ClientIP(r),
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
