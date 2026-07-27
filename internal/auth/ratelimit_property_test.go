package auth_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"pgregory.net/rapid"

	"church-audio-streaming-backend/internal/auth"
)

// TestProperty8_AuthRateLimitEnforcement verifies Property 8:
// After 10 failed attempts within 60 seconds for an IP, all subsequent
// attempts return 429 regardless of credential validity (Req 9.5).
func TestProperty8_AuthRateLimitEnforcement(t *testing.T) {
	rapid.Check(t, func(tc *rapid.T) {
		// Generate a random number of extra attempts after hitting the limit.
		extraAttempts := rapid.IntRange(1, 5).Draw(tc, "extraAttempts").(int)

		// Unique IP per run so windows don't bleed across iterations.
		ip := fmt.Sprintf("192.168.%d.%d",
			rapid.IntRange(0, 255).Draw(tc, "ipB").(int),
			rapid.IntRange(1, 254).Draw(tc, "ipC").(int),
		)

		rl := auth.NewRateLimiter()
		// Wrap a 401 handler (simulated failed login) with the rate-limit middleware.
		inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		})
		mw := auth.RateLimitMiddleware(rl)
		handler := mw(inner)

		makeReq := func() *httptest.ResponseRecorder {
			req := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
			req.RemoteAddr = ip + ":12345"
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			return rr
		}

		// Drive exactly 10 failures through the rate limiter.
		const limit = 10
		for i := 0; i < limit; i++ {
			rl.RecordFailure(ip)
			rr := makeReq()
			// Still within the window â€” middleware should pass through (401 from inner).
			if rr.Code == http.StatusTooManyRequests {
				tc.Fatalf("failure %d/%d: got 429 before limit was reached", i+1, limit)
			}
		}

		// Now the IP is blocked. Every subsequent request must return 429 + Retry-After.
		for i := 0; i < extraAttempts; i++ {
			rr := makeReq()
			if rr.Code != http.StatusTooManyRequests {
				tc.Fatalf(
					"extra attempt %d: expected 429 after %d failures, got %d",
					i+1, limit, rr.Code,
				)
			}
			if rr.Header().Get("Retry-After") == "" {
				tc.Fatalf("extra attempt %d: missing Retry-After header", i+1)
			}
		}
	})
}
