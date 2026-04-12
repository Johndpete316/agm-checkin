package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"johndpete316/agm-checkin-api/internal/db"
	authmw "johndpete316/agm-checkin-api/internal/middleware"
	"johndpete316/agm-checkin-api/internal/service"
)

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

	r := chi.NewRouter()
	r.Use(chimw.Logger)

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

	// IP blocklist applies to all routes — blocked IPs can't reach auth either
	r.Use(authmw.IPBlocklist(authSvc))

	// Public endpoints
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	r.Post("/api/auth/token", createToken(authSvc))

	// Protected routes — require a valid staff token
	r.Group(func(r chi.Router) {
		r.Use(authmw.RequireToken(authSvc))

		r.Get("/api/auth/me", func(w http.ResponseWriter, r *http.Request) {
			respondJSON(w, http.StatusOK, authmw.StaffFromContext(r.Context()))
		})

		r.Get("/api/competitors", listCompetitors(competitorSvc))
		r.Get("/api/competitors/{id}", getCompetitor(competitorSvc))
		r.Post("/api/competitors", createCompetitor(competitorSvc))
		r.Patch("/api/competitors/{id}/checkin", checkInCompetitor(competitorSvc))
		r.Patch("/api/competitors/{id}/dob", updateDOB(competitorSvc))
		r.Patch("/api/competitors/{id}/validate", validateCompetitor(competitorSvc))
		r.Delete("/api/competitors/{id}", deleteCompetitor(competitorSvc))

		// Admin-only staff management
		r.Group(func(r chi.Router) {
			r.Use(authmw.RequireAdmin)
			r.Get("/api/staff", listStaff(staffSvc))
			r.Patch("/api/staff/{id}/role", updateStaffRole(staffSvc))
			r.Delete("/api/staff/{id}", revokeStaff(staffSvc))
		})
	})

	log.Println("Listening on :8080")
	http.ListenAndServe(":8080", r)
}

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

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
		competitors, err := svc.GetAll(search)
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

func createCompetitor(svc *service.CompetitorService) http.HandlerFunc {
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
		respondJSON(w, http.StatusCreated, competitor)
	}
}

func checkInCompetitor(svc *service.CompetitorService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		staffName := ""
		if staff := authmw.StaffFromContext(r.Context()); staff != nil {
			staffName = staff.FirstName + " " + staff.LastName
		}
		competitor, err := svc.CheckIn(id, staffName)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		respondJSON(w, http.StatusOK, competitor)
	}
}

func updateDOB(svc *service.CompetitorService) http.HandlerFunc {
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
		respondJSON(w, http.StatusOK, competitor)
	}
}

func validateCompetitor(svc *service.CompetitorService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		competitor, err := svc.Validate(id)
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		respondJSON(w, http.StatusOK, competitor)
	}
}

func deleteCompetitor(svc *service.CompetitorService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		if err := svc.Delete(id); err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		w.WriteHeader(http.StatusNoContent)
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

func updateStaffRole(svc *service.StaffService) http.HandlerFunc {
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
		respondJSON(w, http.StatusOK, token)
	}
}

func revokeStaff(svc *service.StaffService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		requestor := authmw.StaffFromContext(r.Context())

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
		w.WriteHeader(http.StatusNoContent)
	}
}
