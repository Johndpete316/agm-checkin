package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

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

	trustedProxy := authmw.TrustedProxy(os.Getenv("TRUSTED_PROXY"))
	switch trustedProxy {
	case authmw.TrustedProxyCloudflare, authmw.TrustedProxyDirect:
	case "":
		trustedProxy = authmw.TrustedProxyCloudflare
		log.Println("WARNING: TRUSTED_PROXY not set; defaulting to 'cloudflare'." +
			" Set TRUSTED_PROXY=direct when running without Cloudflare Tunnel" +
			" to prevent IP-header spoofing.")
	default:
		log.Fatalf("invalid TRUSTED_PROXY value %q; must be 'cloudflare' or 'direct'", trustedProxy)
	}
	if trustedProxy == authmw.TrustedProxyCloudflare {
		log.Println("INFO: trusted-proxy mode is 'cloudflare'. Ensure the server is" +
			" exclusively reachable via Cloudflare Tunnel; direct access bypasses IP security.")
	}
	ipResolver := func(r *http.Request) string {
		return authmw.GetClientIPWithMode(r, trustedProxy)
	}

	database := db.Connect(dsn)
	db.AutoMigrate(database)

	if err := db.Migrate(database); err != nil {
		log.Fatalf("migrations failed: %v", err)
	}

	competitorSvc := service.NewCompetitorService(database)
	authSvc := service.NewAuthService(database, pin)
	staffSvc := service.NewStaffService(database)
	eventSvc := service.NewEventService(database)
	auditSvc := service.NewAuditService(database)
	scheduleSvc := service.NewScheduleService(database)

	r := chi.NewRouter()
	r.Use(chimw.Logger)

	allowedOrigin := os.Getenv("ALLOWED_ORIGIN")
	if allowedOrigin == "" {
		log.Fatal("no allowed origin set, please set an allowed origin with ALLOWED_ORIGIN environment variable.")
	}
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: []string{allowedOrigin},
		AllowedMethods: []string{"GET", "POST", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type", "Accept", "Authorization"},
		MaxAge:         300,
	}))
	r.Use(chimw.Recoverer)

	r.Use(authmw.WithIPResolver(ipResolver))

	r.Use(authmw.IPBlocklist(authSvc))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		// Finding 11: expose the trusted-proxy mode in the health response so
		// operators and monitoring tools can verify the security configuration
		// without having access to server logs.  If this endpoint is directly
		// reachable from the internet without going through a Cloudflare Tunnel,
		// the trustedProxy value shows whether IP spoofing is possible.
		respondJSON(w, http.StatusOK, map[string]string{
			"status":       "ok",
			"trustedProxy": string(trustedProxy),
		})
	})
	r.Post("/api/auth/token", createToken(authSvc))

	r.Group(func(r chi.Router) {
		r.Use(authmw.RequireToken(authSvc))

		r.Get("/api/auth/me", func(w http.ResponseWriter, r *http.Request) {
			staff := authmw.StaffFromContext(r.Context())
			if staff == nil {
				respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				return
			}
			respondJSON(w, http.StatusOK, staff.ToView())
		})

		r.Get("/api/competitors", listCompetitors(competitorSvc))
		r.Get("/api/competitors/{id}", getCompetitor(competitorSvc))
		r.Post("/api/competitors", createCompetitor(competitorSvc, auditSvc))
		r.Patch("/api/competitors/{id}/checkin", checkInCompetitor(competitorSvc, auditSvc))
		r.Patch("/api/competitors/{id}/contact", updateCompetitorContact(competitorSvc, auditSvc))
		r.Patch("/api/competitors/{id}/dob", updateDOB(competitorSvc, auditSvc))
		r.Patch("/api/competitors/{id}/validate", validateCompetitor(competitorSvc, auditSvc))
		r.Get("/api/competitors/{id}/events", getCompetitorEvents(competitorSvc))
		r.Get("/api/competitors/{id}/schedule", getCompetitorSchedule(scheduleSvc, eventSvc))

		r.Get("/api/events", listEvents(eventSvc))
		r.Get("/api/events/current", getCurrentEvent(eventSvc))

		r.Group(func(r chi.Router) {
			r.Use(authmw.RequireAdmin)
			r.Patch("/api/competitors/{id}", updateCompetitor(competitorSvc, auditSvc))

			// Deleting a competitor cascades their whole attendance history for
			// every event, not just the one being staffed. It sat outside this
			// group while the far milder edit beside it was admin-only, so a
			// registration token could destroy records it was not allowed to
			// change. No caller in the frontend uses it.
			r.Delete("/api/competitors/{id}", deleteCompetitor(competitorSvc, auditSvc))

			r.Post("/api/events", createEvent(eventSvc, auditSvc))
			r.Patch("/api/events/{id}/current", setCurrentEvent(eventSvc, auditSvc))

			r.Get("/api/staff", listStaff(staffSvc))
			r.Patch("/api/staff/{id}/role", updateStaffRole(staffSvc, auditSvc))
			r.Delete("/api/staff/{id}", revokeStaff(staffSvc, auditSvc))

			r.Get("/api/audit", listAudit(auditSvc))

			r.Post("/api/competitors/import", bulkImportCompetitors(competitorSvc, auditSvc))

			r.Post("/api/competitors/{id}/schedule/import", bulkImportSchedule(scheduleSvc, auditSvc))
			r.Post("/api/competitors/{id}/schedule", createScheduleEntry(scheduleSvc, auditSvc))
			r.Patch("/api/schedule/{id}", updateScheduleEntry(scheduleSvc, auditSvc))
			r.Delete("/api/schedule/{id}", deleteScheduleEntry(scheduleSvc, auditSvc))
		})
	})

	log.Println("Listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}

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
