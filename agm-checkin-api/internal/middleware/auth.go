package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"johndpete316/agm-checkin-api/internal/db"
	"johndpete316/agm-checkin-api/internal/service"
)

type contextKey string

const StaffTokenKey contextKey = "staffToken"

// GetClientIP returns the real client IP, preferring the CF-Connecting-IP header
// set by Cloudflare Tunnel. Falls back to X-Forwarded-For, then RemoteAddr.
func GetClientIP(r *http.Request) string {
	if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
		return strings.TrimSpace(ip)
	}
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return strings.TrimSpace(strings.Split(ip, ",")[0])
	}
	addr := r.RemoteAddr
	if i := strings.LastIndex(addr, ":"); i != -1 {
		return addr[:i]
	}
	return addr
}

// StaffFromContext retrieves the authenticated staff token from the request context.
// Returns nil if not present (i.e. on an unprotected route).
func StaffFromContext(ctx context.Context) *db.StaffToken {
	if val := ctx.Value(StaffTokenKey); val != nil {
		if token, ok := val.(*db.StaffToken); ok {
			return token
		}
	}
	return nil
}

// IPBlocklist checks every request against the database blocklist.
// Blocked IPs receive 403 immediately. Apply this globally, before auth routes.
func IPBlocklist(authSvc *service.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := GetClientIP(r)
			if authSvc.IsIPBlocked(ip) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				json.NewEncoder(w).Encode(map[string]string{"error": "access denied"})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireToken validates the Bearer token in the Authorization header.
// On success, injects the StaffToken into the request context.
// Apply this to all protected API routes.
func RequireToken(authSvc *service.AuthService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Bearer ") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
				return
			}

			token := strings.TrimPrefix(authHeader, "Bearer ")
			staffToken, ok := authSvc.ValidateToken(token)
			if !ok {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
				return
			}

			ctx := context.WithValue(r.Context(), StaffTokenKey, staffToken)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
