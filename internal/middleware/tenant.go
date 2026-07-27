package middleware

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"church-audio-streaming-backend/internal/tenant"
)

type contextKey string

const tenantKey contextKey = "tenant"

// TenantResolution extracts {tenantSlug} from the URL, looks it up via the
// Registry, injects the *db.Tenant into the request context, and returns 404
// on unknown slugs — no information about other tenants is leaked (Req 1.3,
// 1.6).
func TenantResolution(reg *tenant.Registry) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			slug := chi.URLParam(r, "tenantSlug")
			if slug == "" {
				http.NotFound(w, r)
				return
			}

			t, err := reg.GetBySlug(r.Context(), slug)
			if err != nil {
				// ErrTenantNotFound and any other error both map to 404
				// to avoid leaking existence (Req 1.6).
				http.NotFound(w, r)
				return
			}

			ctx := context.WithValue(r.Context(), tenantKey, t)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
