package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"johndpete316/agm-checkin-api/internal/db"
	"johndpete316/agm-checkin-api/internal/service"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}
	database := db.Connect(dsn)
	db.AutoMigrate(database)

	competitorSvc := service.NewCompetitorService(database)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	allowedOrigin := os.Getenv("ALLOWED_ORIGIN")
	if allowedOrigin == "" {
		allowedOrigin = "*"
	}
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{allowedOrigin},
		AllowedMethods: []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type", "Accept"},
		MaxAge:         300,
	}))
	r.Use(middleware.Recoverer)

	r.Get("/api/competitors", listCompetitors(competitorSvc))
	r.Get("/api/competitors/{id}", getCompetitor(competitorSvc))
	r.Post("/api/competitors", createCompetitor(competitorSvc))
	r.Patch("/api/competitors/{id}/checkin", checkInCompetitor(competitorSvc))
	r.Delete("/api/competitors/{id}", deleteCompetitor(competitorSvc))

	log.Println("Listening on :8080")
	http.ListenAndServe(":8080", r)
}

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
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
		competitor, err := svc.CheckIn(id)
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
