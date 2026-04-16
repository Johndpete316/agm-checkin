package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"johndpete316/agm-checkin-api/internal/db"
	authmw "johndpete316/agm-checkin-api/internal/middleware"
	"johndpete316/agm-checkin-api/internal/service"
)

func createToken(authSvc *service.AuthService) http.HandlerFunc {
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

		ip := authmw.ClientIP(r)
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

func listStaff(svc *service.StaffService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokens, err := svc.List()
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		views := make([]db.StaffView, len(tokens))
		for i := range tokens {
			views[i] = tokens[i].ToView()
		}
		respondJSON(w, http.StatusOK, views)
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
			IP:         authmw.ClientIP(r),
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
			IP:         authmw.ClientIP(r),
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
