package middleware

import (
	"context"
	"net/http"
	"strings"

	"church-audio-streaming-backend/internal/auth"
	"church-audio-streaming-backend/internal/db"
)

const claimsKey contextKey = "claims"

// AuthRequired returns middleware that verifies the JWT from the
// "Authorization: Bearer <token>" header and enforces the required role(s).
// On success the *auth.Claims are stored in the request context.
// Returns 401 on missing/invalid/expired token, 403 on wrong role or wrong
// tenant (Req 2.3, 2.5, 9.4).
func AuthRequired(svc *auth.Service, roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := bearerToken(r)
			if token == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			clientIP := clientIPFromRequest(r)
			claims, err := svc.VerifyToken(token, clientIP)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			// Role check.
			if len(roles) > 0 && !hasRole(claims.Role, roles) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}

			// Tenant scope check: the JWT tenantID must match the URL tenant
			// (Req 2.5 / Property 1).
			if t, ok := r.Context().Value(tenantKey).(*db.Tenant); ok && t != nil {
				if claims.TenantID != t.ID && claims.Role != "platform_admin" {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
			}

			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if after, ok := strings.CutPrefix(h, "Bearer "); ok {
		return strings.TrimSpace(after)
	}
	// Fallback: allow token as query param for WebSocket upgrades.
	return r.URL.Query().Get("token")
}

func hasRole(role string, allowed []string) bool {
	for _, a := range allowed {
		if role == a {
			return true
		}
	}
	return false
}

func clientIPFromRequest(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	// RemoteAddr is "ip:port" in standard Go HTTP.
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i != -1 {
		host = host[:i]
	}
	return host
}
