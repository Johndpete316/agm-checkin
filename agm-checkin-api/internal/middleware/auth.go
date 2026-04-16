package middleware

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"

	"johndpete316/agm-checkin-api/internal/db"
	"johndpete316/agm-checkin-api/internal/service"
)

type contextKey string

const StaffTokenKey contextKey = "staffToken"

// TrustedProxy controls which upstream headers GetClientIP will believe.
// Set via the TRUSTED_PROXY environment variable.
//
//   - "cloudflare" (default): trust CF-Connecting-IP set by Cloudflare Tunnel,
//     fall back to X-Forwarded-For, then RemoteAddr.  Use only when the server
//     is exclusively reachable via a Cloudflare Tunnel so these headers cannot
//     be spoofed by a direct caller.
//   - "direct": ignore all forwarding headers; use RemoteAddr only.  Safe for
//     local development or when there is no trusted reverse proxy.
type TrustedProxy string

const (
	TrustedProxyCloudflare TrustedProxy = "cloudflare"
	TrustedProxyDirect     TrustedProxy = "direct"
)

// GetClientIP returns the real client IP according to the configured proxy
// trust level.  When mode is TrustedProxyCloudflare the CF-Connecting-IP and
// X-Forwarded-For headers are used; otherwise only RemoteAddr is used.
func GetClientIP(r *http.Request) string {
	return GetClientIPWithMode(r, TrustedProxyCloudflare)
}

// GetClientIPWithMode is the same as GetClientIP but accepts an explicit mode.
// Use this variant when the proxy trust mode is read from configuration.
func GetClientIPWithMode(r *http.Request, mode TrustedProxy) string {
	if mode == TrustedProxyCloudflare {
		if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
			return strings.TrimSpace(ip)
		}
		if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
			return strings.TrimSpace(strings.Split(ip, ",")[0])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// IPResolverMiddleware returns an http.Handler middleware that attaches an
// IPResolver func to the request context so handlers can call GetClientIPWithMode
// without hard-coding the proxy trust mode.  This avoids threading the mode
// value through every call site.
type IPResolver func(r *http.Request) string

type ipResolverKey struct{}

// WithIPResolver stores an IPResolver in the context so handlers can retrieve
// the client IP without hard-coding a proxy trust mode.
func WithIPResolver(resolver IPResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), ipResolverKey{}, resolver)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ClientIP retrieves the client IP using the IPResolver stored in the context,
// falling back to the default (cloudflare-trusted) mode if none is configured.
func ClientIP(r *http.Request) string {
	if fn, ok := r.Context().Value(ipResolverKey{}).(IPResolver); ok {
		return fn(r)
	}
	return GetClientIP(r)
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
			ip := ClientIP(r)
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

// RequireAdmin must be used inside a RequireToken group.
// Returns 403 if the authenticated staff member does not have the "admin" role.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		staff := StaffFromContext(r.Context())
		if staff == nil || staff.Role != "admin" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(map[string]string{"error": "admin access required"})
			return
		}
		next.ServeHTTP(w, r)
	})
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
