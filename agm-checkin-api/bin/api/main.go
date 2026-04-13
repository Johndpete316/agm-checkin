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

		r.Get("/api/competitors", listCompetitors(competitorSvc))
		r.Get("/api/competitors/{id}", getCompetitor(competitorSvc))
		r.Post("/api/competitors", createCompetitor(competitorSvc))
		r.Patch("/api/competitors/{id}/checkin", checkInCompetitor(competitorSvc))
		r.Patch("/api/competitors/{id}/dob", updateDOB(competitorSvc))
		r.Patch("/api/competitors/{id}/validate", validateCompetitor(competitorSvc))
		r.Delete("/api/competitors/{id}", deleteCompetitor(competitorSvc))
	})

	log.Println("Listening on :8080")
	http.ListenAndServe(":8080", r)
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
