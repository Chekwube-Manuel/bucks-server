package middleware

import (
	"context"
	"net/http"
	"strings"

	"church-audio-streaming-backend/internal/db"
)

// TenantFromContext returns the *db.Tenant stored by TenantResolution, or nil.
func TenantFromContext(ctx context.Context) *db.Tenant {
	t, _ := ctx.Value(tenantKey).(*db.Tenant)
	return t
}

// ClaimsFromContext returns the *auth.Claims stored by AuthRequired, or nil.
func ClaimsFromContext(ctx context.Context) interface{} {
	return ctx.Value(claimsKey)
}

// ClientIP extracts the best-effort client IP from the request.
func ClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.SplitN(xff, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i != -1 {
		host = host[:i]
	}
	return host
}
