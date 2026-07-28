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

func getCompetitorSchedule(scheduleSvc *service.ScheduleService, eventSvc *service.EventService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		competitorID := chi.URLParam(r, "id")
		eventID := r.URL.Query().Get("eventId")
		if eventID == "" {
			current, err := eventSvc.GetCurrent()
			if err != nil {
				respondJSON(w, http.StatusOK, []db.CompetitorSchedule{})
				return
			}
			eventID = current.ID
		}
		entries, err := scheduleSvc.GetByCompetitorEvent(competitorID, eventID)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		respondJSON(w, http.StatusOK, entries)
	}
}

func createScheduleEntry(scheduleSvc *service.ScheduleService, audit *service.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		competitorID := chi.URLParam(r, "id")
		var body struct {
			EventID      string    `json:"eventId"`
			Instrument   string    `json:"instrument"`
			ScheduleDate time.Time `json:"scheduleDate"`
			ScheduleTime string    `json:"scheduleTime"`
			Room         string    `json:"room"`
			Category     string    `json:"category"`
			Division     string    `json:"division"`
			PageNumber   string    `json:"pageNumber"`
			SortOrder    int       `json:"sortOrder"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if body.EventID == "" {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "eventId is required"})
			return
		}
		entry := db.CompetitorSchedule{
			CompetitorID: competitorID,
			EventID:      body.EventID,
			Instrument:   body.Instrument,
			ScheduleDate: body.ScheduleDate,
			ScheduleTime: body.ScheduleTime,
			Room:         body.Room,
			Category:     body.Category,
			Division:     body.Division,
			PageNumber:   body.PageNumber,
			SortOrder:    body.SortOrder,
		}
		if err := scheduleSvc.Create(&entry); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		actorID, actorName := actorFrom(r)
		audit.Log(service.LogEntry{
			ActorID:    actorID,
			ActorName:  actorName,
			Action:     "schedule.created",
			EntityType: "schedule",
			EntityID:   entry.ID,
			EntityName: competitorID,
			Detail:     map[string]any{"eventId": entry.EventID, "instrument": entry.Instrument},
			IP:         authmw.ClientIP(r),
		})
		respondJSON(w, http.StatusCreated, entry)
	}
}

func updateScheduleEntry(scheduleSvc *service.ScheduleService, audit *service.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var body struct {
			Instrument   string    `json:"instrument"`
			ScheduleDate time.Time `json:"scheduleDate"`
			ScheduleTime string    `json:"scheduleTime"`
			Room         string    `json:"room"`
			Category     string    `json:"category"`
			Division     string    `json:"division"`
			PageNumber   string    `json:"pageNumber"`
			SortOrder    int       `json:"sortOrder"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		updated, err := scheduleSvc.Update(id, db.CompetitorSchedule{
			Instrument:   body.Instrument,
			ScheduleDate: body.ScheduleDate,
			ScheduleTime: body.ScheduleTime,
			Room:         body.Room,
			Category:     body.Category,
			Division:     body.Division,
			PageNumber:   body.PageNumber,
			SortOrder:    body.SortOrder,
		})
		if err != nil {
			if errors.Is(err, service.ErrScheduleNotFound) {
				respondJSON(w, http.StatusNotFound, map[string]string{"error": "schedule entry not found"})
				return
			}
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		actorID, actorName := actorFrom(r)
		audit.Log(service.LogEntry{
			ActorID:    actorID,
			ActorName:  actorName,
			Action:     "schedule.updated",
			EntityType: "schedule",
			EntityID:   id,
			EntityName: updated.CompetitorID,
			Detail:     map[string]any{"instrument": updated.Instrument},
			IP:         authmw.ClientIP(r),
		})
		respondJSON(w, http.StatusOK, updated)
	}
}

func deleteScheduleEntry(scheduleSvc *service.ScheduleService, audit *service.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := scheduleSvc.Delete(id); err != nil {
			if errors.Is(err, service.ErrScheduleNotFound) {
				respondJSON(w, http.StatusNotFound, map[string]string{"error": "schedule entry not found"})
				return
			}
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		actorID, actorName := actorFrom(r)
		audit.Log(service.LogEntry{
			ActorID:    actorID,
			ActorName:  actorName,
			Action:     "schedule.deleted",
			EntityType: "schedule",
			EntityID:   id,
			EntityName: id,
			IP:         authmw.ClientIP(r),
		})
		w.WriteHeader(http.StatusNoContent)
	}
}

func bulkImportSchedule(scheduleSvc *service.ScheduleService, audit *service.AuditService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		competitorID := chi.URLParam(r, "id")
		var body struct {
			EventID string `json:"eventId"`
			Entries []struct {
				Instrument   string    `json:"instrument"`
				ScheduleDate time.Time `json:"scheduleDate"`
				ScheduleTime string    `json:"scheduleTime"`
				Room         string    `json:"room"`
				Category     string    `json:"category"`
				Division     string    `json:"division"`
				PageNumber   string    `json:"pageNumber"`
				SortOrder    int       `json:"sortOrder"`
			} `json:"entries"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if body.EventID == "" {
			respondJSON(w, http.StatusBadRequest, map[string]string{"error": "eventId is required"})
			return
		}
		entries := make([]db.CompetitorSchedule, len(body.Entries))
		for i, e := range body.Entries {
			entries[i] = db.CompetitorSchedule{
				Instrument:   e.Instrument,
				ScheduleDate: e.ScheduleDate,
				ScheduleTime: e.ScheduleTime,
				Room:         e.Room,
				Category:     e.Category,
				Division:     e.Division,
				PageNumber:   e.PageNumber,
				SortOrder:    e.SortOrder,
			}
		}
		count, err := scheduleSvc.BulkUpsert(competitorID, body.EventID, entries)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		actorID, actorName := actorFrom(r)
		audit.Log(service.LogEntry{
			ActorID:    actorID,
			ActorName:  actorName,
			Action:     "schedule.bulk_import",
			EntityType: "schedule",
			EntityID:   competitorID,
			EntityName: competitorID,
			Detail:     map[string]any{"eventId": body.EventID, "entriesInserted": count},
			IP:         authmw.ClientIP(r),
		})
		respondJSON(w, http.StatusOK, map[string]int{"inserted": count})
	}
}
