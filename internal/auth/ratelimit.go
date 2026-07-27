package auth

import (
"net"
"net/http"
"strings"
"sync"
"time"
)

const (
// rateLimitWindow is the sliding window duration for counting failures.
rateLimitWindow = 60 * time.Second

// rateLimitMax is the maximum number of failures allowed within the window
// before subsequent requests are blocked (Req 9.5).
// The 11th+ attempt is blocked, i.e. block when failCount > 10.
rateLimitMax = 10
)

// windowEntry tracks failed login attempts within a sliding window for a single
// IP address.
type windowEntry struct {
failCount   int
windowStart time.Time
}

// RateLimiter is an in-memory, per-IP sliding-window failure counter.
// It is safe for concurrent use.
type RateLimiter struct {
mu      sync.Mutex
entries map[string]*windowEntry
}

// NewRateLimiter creates and returns a new, ready-to-use RateLimiter.
func NewRateLimiter() *RateLimiter {
return &RateLimiter{
entries: make(map[string]*windowEntry),
}
}

// RecordFailure records a failed authentication attempt for the given IP
// address and reports whether the IP is now blocked.
//
// Behaviour:
//   - If no window exists for the IP, or the current window has expired
//     (>=60 s since windowStart), a fresh window is started.
//   - The failure count is incremented.
//   - Returns true (blocked) if failCount > rateLimitMax (i.e. the 11th+
//     attempt).
//
// (Req 9.5 / Property 8)
func (r *RateLimiter) RecordFailure(ip string) (blocked bool) {
r.mu.Lock()
defer r.mu.Unlock()

now := time.Now()
entry, ok := r.entries[ip]

// If no entry exists, or the window has expired, start a fresh window.
if !ok || now.Sub(entry.windowStart) >= rateLimitWindow {
r.entries[ip] = &windowEntry{
failCount:   1,
windowStart: now,
}
return false // first failure — not blocked yet
}

entry.failCount++
return entry.failCount > rateLimitMax
}

// ResetOnSuccess clears the failure counter for the given IP address.
// Call this after a successful login so that a legitimate user is not locked
// out after previous failures.
func (r *RateLimiter) ResetOnSuccess(ip string) {
r.mu.Lock()
defer r.mu.Unlock()

delete(r.entries, ip)
}

// IsBlocked reports whether the given IP address currently has >= rateLimitMax
// recorded failures within an active (non-expired) window.
func (r *RateLimiter) IsBlocked(ip string) bool {
r.mu.Lock()
defer r.mu.Unlock()

entry, ok := r.entries[ip]
if !ok {
return false
}

// If the window has expired the IP is no longer blocked.
if time.Since(entry.windowStart) >= rateLimitWindow {
return false
}

return entry.failCount >= rateLimitMax
}

// RateLimitMiddleware returns an HTTP middleware that enforces the per-IP
// rate limit.  It should be applied to the login endpoint only.
//
// If IsBlocked returns true for the client IP the middleware responds with
// HTTP 429 and a Retry-After: 60 header; otherwise it passes the request to
// the next handler unchanged.
//
// The client IP is resolved from the X-Forwarded-For header (first value) when
// present, falling back to the RemoteAddr from the request.
func RateLimitMiddleware(limiter *RateLimiter) func(http.Handler) http.Handler {
return func(next http.Handler) http.Handler {
return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
ip := clientIP(r)

if limiter.IsBlocked(ip) {
w.Header().Set("Retry-After", "60")
http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
return
}

next.ServeHTTP(w, r)
})
}
}

// clientIP extracts the best-effort client IP from the request.
// It checks X-Forwarded-For first (leftmost non-empty value), then falls back
// to the host portion of RemoteAddr.
func clientIP(r *http.Request) string {
if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
// X-Forwarded-For may be a comma-separated list; use the first value.
parts := strings.SplitN(xff, ",", 2)
ip := strings.TrimSpace(parts[0])
if ip != "" {
return ip
}
}

host, _, err := net.SplitHostPort(r.RemoteAddr)
if err != nil {
// RemoteAddr may already be an IP without a port in some test contexts.
return r.RemoteAddr
}
return host
}
