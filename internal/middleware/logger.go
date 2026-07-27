package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"church-audio-streaming-backend/internal/db"
)

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// Logger emits a structured JSON log line for every request containing ts,
// method, path, tenantId, status, and latencyMs (Req 10.4).
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rw, r)

		tenantID := ""
		if t, ok := r.Context().Value(tenantKey).(*db.Tenant); ok && t != nil {
			tenantID = t.ID
		}

		slog.Info("request",
			"ts", start.UTC().Format(time.RFC3339Nano),
			"method", r.Method,
			"path", r.URL.Path,
			"tenant_id", tenantID,
			"status", rw.status,
			"latency_ms", time.Since(start).Milliseconds(),
		)
	})
}
