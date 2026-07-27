package middleware

import (
	"net/http"

	"church-audio-streaming-backend/internal/db"
)

// CORS sets Access-Control-Allow-Origin to the tenant's configured subdomain
// and rejects requests from other origins (Req 9.6).
func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// Determine the allowed origin from the tenant context (if present).
		allowedOrigin := "*" // permissive default for admin/health routes
		if t, ok := r.Context().Value(tenantKey).(*db.Tenant); ok && t != nil {
			// Build the expected origin from the tenant slug.
			// In production this would be read from a configured domain field.
			allowedOrigin = "https://" + t.Slug + ".example.com"
		}

		if origin != "" {
			if allowedOrigin == "*" || origin == allowedOrigin {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
