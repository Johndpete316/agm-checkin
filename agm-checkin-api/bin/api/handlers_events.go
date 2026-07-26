package main

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"johndpete316/agm-checkin-api/internal/db"
	authmw "johndpete316/agm-checkin-api/internal/middleware"
	"johndpete316/agm-checkin-api/internal/service"
)

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
			IP:         authmw.ClientIP(r),
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
			IP:         authmw.ClientIP(r),
		})
		respondJSON(w, http.StatusOK, event)
	}
}
