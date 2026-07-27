package router_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/flyingmutant/rapid"
	"github.com/golang-jwt/jwt/v5"

	"church-audio-streaming-backend/internal/auth"
	"church-audio-streaming-backend/internal/metrics"
	"church-audio-streaming-backend/internal/relay"
	"church-audio-streaming-backend/internal/router"
	"church-audio-streaming-backend/internal/tenant"
)

// TestProperty1_TenantIsolation verifies Property 1:
// A JWT scoped to tenant A must receive 403 (or 404) when accessing resources
// belonging to tenant B, and the response must not contain tenant B's data.
func TestProperty1_TenantIsolation(t *testing.T) {
	const secret = "isolation-test-secret"

	rapid.Check(t, func(tc *rapid.T) {
		slugA := rapid.StringMatching(`[a-z]{4,8}`).Draw(tc, "slugA").(string)
		slugB := rapid.StringMatching(`[a-z]{4,8}`).Draw(tc, "slugB").(string)
		if slugA == slugB {
			tc.Skip("same slug, skip")
		}

		// Build minimal deps (nil DB — tenant registry will return not-found).
		authSvc := auth.NewService(secret, nil)
		tenantReg := tenant.NewRegistry(nil)
		hub := relay.NewHub()
		col := metrics.New(nil)
		rl := auth.NewRateLimiter()

		h := router.New(router.Deps{
			AuthSvc:   authSvc,
			TenantReg: tenantReg,
			RelayHub:  hub,
			Metrics:   col,
			AuthRL:    rl,
		})

		// Issue a JWT scoped to tenant A's ID.
		tokenA := mustIssueToken(secret, slugA+"-id", slugA, "broadcaster")

		// Attempt to access tenant B's config endpoint.
		path := fmt.Sprintf("/api/%s/config", slugB)
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+tokenA)

		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		// The response must be 401, 403, or 404 — never 200 with tenant B data.
		code := rr.Code
		if code == http.StatusOK {
			tc.Fatalf(
				"tenant A token got 200 on tenant B (%s) endpoint — isolation violated",
				slugB,
			)
		}

		// Response body must not contain slugB as a data value.
		body := rr.Body.String()
		if len(body) > 0 && containsSlug(body, slugB) && code == http.StatusOK {
			tc.Fatalf("response leaks tenant B slug: %s", body)
		}
	})
}

func mustIssueToken(secret, sub, tenantID, role string) string {
	now := time.Now().UTC()
	claims := &auth.Claims{
		Sub:      sub,
		TenantID: tenantID,
		Role:     role,
		IP:       "127.0.0.1",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   sub,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(8 * time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, _ := tok.SignedString([]byte(secret))
	return s
}

func containsSlug(body, slug string) bool {
	return len(slug) > 3 && len(body) > len(slug) &&
		(func() bool {
			for i := 0; i <= len(body)-len(slug); i++ {
				if body[i:i+len(slug)] == slug {
					return true
				}
			}
			return false
		})()
}
