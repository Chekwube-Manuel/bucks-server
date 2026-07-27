package auth

import (
"net/http"
"net/http/httptest"
"testing"
"time"
)

// --- RateLimiter unit tests ---

func TestNewRateLimiter(t *testing.T) {
rl := NewRateLimiter()
if rl == nil {
t.Fatal("NewRateLimiter() returned nil")
}
if rl.entries == nil {
t.Fatal("entries map not initialised")
}
}

// TestIsBlocked_NoEntry reports false for an IP with no recorded failures.
func TestIsBlocked_NoEntry(t *testing.T) {
rl := NewRateLimiter()
if rl.IsBlocked("1.2.3.4") {
t.Error("expected not blocked for unknown IP")
}
}

// TestRecordFailure_NotBlockedBefore10 verifies that the first 10 failures
// do not block the IP (failures 1-10 return false).
func TestRecordFailure_NotBlockedBefore10(t *testing.T) {
rl := NewRateLimiter()
ip := "10.0.0.1"
for i := 1; i <= 10; i++ {
blocked := rl.RecordFailure(ip)
if blocked {
t.Errorf("attempt %d: expected not blocked, got blocked", i)
}
}
}

// TestRecordFailure_BlockedAfter10 verifies that the 11th and subsequent
// failures return blocked = true.
func TestRecordFailure_BlockedAfter10(t *testing.T) {
rl := NewRateLimiter()
ip := "10.0.0.2"
for i := 1; i <= 10; i++ {
rl.RecordFailure(ip)
}
// 11th attempt must be blocked.
if !rl.RecordFailure(ip) {
t.Error("11th attempt: expected blocked, got not blocked")
}
// 12th attempt must also be blocked.
if !rl.RecordFailure(ip) {
t.Error("12th attempt: expected blocked, got not blocked")
}
}

// TestIsBlocked_AfterTenFailures verifies that IsBlocked returns true once
// the failure count reaches rateLimitMax.
func TestIsBlocked_AfterTenFailures(t *testing.T) {
rl := NewRateLimiter()
ip := "10.0.0.3"
for i := 0; i < rateLimitMax; i++ {
rl.RecordFailure(ip)
}
if !rl.IsBlocked(ip) {
t.Errorf("expected blocked after %d failures", rateLimitMax)
}
}

// TestResetOnSuccess_ClearsCounter verifies that a successful login clears
// the failure counter and IsBlocked returns false afterwards.
func TestResetOnSuccess_ClearsCounter(t *testing.T) {
rl := NewRateLimiter()
ip := "10.0.0.4"
for i := 0; i < rateLimitMax; i++ {
rl.RecordFailure(ip)
}
if !rl.IsBlocked(ip) {
t.Fatal("pre-condition: expected blocked before reset")
}
rl.ResetOnSuccess(ip)
if rl.IsBlocked(ip) {
t.Error("expected not blocked after ResetOnSuccess")
}
}

// TestRecordFailure_WindowExpiry verifies that after the 60-second window
// expires the counter resets.  We do this by injecting a backdated entry
// directly into the map.
func TestRecordFailure_WindowExpiry(t *testing.T) {
rl := NewRateLimiter()
ip := "10.0.0.5"

// Manually insert an expired entry with failCount at the limit.
rl.mu.Lock()
rl.entries[ip] = &windowEntry{
failCount:   rateLimitMax,
windowStart: time.Now().Add(-rateLimitWindow - time.Second),
}
rl.mu.Unlock()

// The first call after expiry should start a fresh window and not block.
blocked := rl.RecordFailure(ip)
if blocked {
t.Error("expected not blocked after window expiry; window should have reset")
}

// failCount should now be 1 (fresh window).
rl.mu.Lock()
got := rl.entries[ip].failCount
rl.mu.Unlock()
if got != 1 {
t.Errorf("expected failCount=1 after window reset, got %d", got)
}
}

// TestIsBlocked_ExpiredWindow verifies that IsBlocked returns false once the
// window has expired, even if failCount >= rateLimitMax.
func TestIsBlocked_ExpiredWindow(t *testing.T) {
rl := NewRateLimiter()
ip := "10.0.0.6"

rl.mu.Lock()
rl.entries[ip] = &windowEntry{
failCount:   rateLimitMax,
windowStart: time.Now().Add(-rateLimitWindow - time.Second),
}
rl.mu.Unlock()

if rl.IsBlocked(ip) {
t.Error("expected not blocked after window expiry")
}
}

// TestDifferentIPsAreIndependent verifies that failures for one IP do not
// affect another IP.
func TestDifferentIPsAreIndependent(t *testing.T) {
rl := NewRateLimiter()
ipA := "192.168.1.1"
ipB := "192.168.1.2"

for i := 0; i < rateLimitMax; i++ {
rl.RecordFailure(ipA)
}

if rl.IsBlocked(ipB) {
t.Error("ipB should not be blocked because of ipA failures")
}
}

// --- clientIP unit tests ---

func TestClientIP_XForwardedFor(t *testing.T) {
req := httptest.NewRequest(http.MethodPost, "/", nil)
req.Header.Set("X-Forwarded-For", "203.0.113.1, 10.0.0.1")
got := clientIP(req)
if got != "203.0.113.1" {
t.Errorf("expected 203.0.113.1, got %q", got)
}
}

func TestClientIP_RemoteAddr(t *testing.T) {
req := httptest.NewRequest(http.MethodPost, "/", nil)
req.RemoteAddr = "198.51.100.42:12345"
// Remove X-Forwarded-For to fall through to RemoteAddr.
req.Header.Del("X-Forwarded-For")
got := clientIP(req)
if got != "198.51.100.42" {
t.Errorf("expected 198.51.100.42, got %q", got)
}
}

// --- RateLimitMiddleware tests ---

// dummyHandler is a trivial http.Handler that records whether it was called.
type dummyHandler struct {
called bool
}

func (d *dummyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
d.called = true
w.WriteHeader(http.StatusOK)
}

// TestRateLimitMiddleware_PassesThrough verifies that requests from a
// non-blocked IP are forwarded to the next handler.
func TestRateLimitMiddleware_PassesThrough(t *testing.T) {
rl := NewRateLimiter()
next := &dummyHandler{}
mw := RateLimitMiddleware(rl)(next)

req := httptest.NewRequest(http.MethodPost, "/auth/token", nil)
req.RemoteAddr = "10.1.2.3:9999"
rr := httptest.NewRecorder()
mw.ServeHTTP(rr, req)

if rr.Code != http.StatusOK {
t.Errorf("expected 200, got %d", rr.Code)
}
if !next.called {
t.Error("expected next handler to be called")
}
}

// TestRateLimitMiddleware_Blocks verifies that a blocked IP receives HTTP 429
// with the Retry-After header and the next handler is NOT called.
func TestRateLimitMiddleware_Blocks(t *testing.T) {
rl := NewRateLimiter()
ip := "172.16.0.1"

// Reach the limit.
for i := 0; i < rateLimitMax; i++ {
rl.RecordFailure(ip)
}

next := &dummyHandler{}
mw := RateLimitMiddleware(rl)(next)

req := httptest.NewRequest(http.MethodPost, "/auth/token", nil)
req.RemoteAddr = ip + ":8080"
rr := httptest.NewRecorder()
mw.ServeHTTP(rr, req)

if rr.Code != http.StatusTooManyRequests {
t.Errorf("expected 429, got %d", rr.Code)
}
if rr.Header().Get("Retry-After") != "60" {
t.Errorf("expected Retry-After: 60, got %q", rr.Header().Get("Retry-After"))
}
if next.called {
t.Error("next handler must not be called when IP is blocked")
}
}

// TestRateLimitMiddleware_BlocksViaXForwardedFor verifies that the middleware
// reads the real IP from X-Forwarded-For.
func TestRateLimitMiddleware_BlocksViaXForwardedFor(t *testing.T) {
rl := NewRateLimiter()
realIP := "9.8.7.6"
for i := 0; i < rateLimitMax; i++ {
rl.RecordFailure(realIP)
}

next := &dummyHandler{}
mw := RateLimitMiddleware(rl)(next)

req := httptest.NewRequest(http.MethodPost, "/auth/token", nil)
req.RemoteAddr = "127.0.0.1:1234" // proxy address
req.Header.Set("X-Forwarded-For", realIP)
rr := httptest.NewRecorder()
mw.ServeHTTP(rr, req)

if rr.Code != http.StatusTooManyRequests {
t.Errorf("expected 429, got %d", rr.Code)
}
}
