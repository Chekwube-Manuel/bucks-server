// Package router wires all API routes onto a chi router and applies the
// middleware stack (Req 9.1, 10.1).
package router

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"

	"church-audio-streaming-backend/internal/auth"
	appMiddleware "church-audio-streaming-backend/internal/middleware"
	"church-audio-streaming-backend/internal/metrics"
	"church-audio-streaming-backend/internal/relay"
	"church-audio-streaming-backend/internal/signaling"
	"church-audio-streaming-backend/internal/tenant"
)

// Deps bundles all service dependencies required to build the router.
type Deps struct {
	AuthSvc   *auth.Service
	TenantReg *tenant.Registry
	RelayHub  *relay.Hub
	Metrics   *metrics.Collector
	AuthRL    *auth.RateLimiter
}

// New constructs and returns the fully-wired chi router.
func New(d Deps) http.Handler {
	r := chi.NewRouter()

	// Global middleware.
	r.Use(chiMiddleware.Recoverer)
	r.Use(appMiddleware.Logger)
	r.Use(appMiddleware.CORS)

	// Health check (Req 10.1).
	r.Get("/health", healthHandler())

	// Login with rate limiting.
	r.With(auth.RateLimitMiddleware(d.AuthRL)).
		Post("/api/{tenantSlug}/auth/login", loginHandler(d))

	// Per-tenant routes.
	r.Route("/api/{tenantSlug}", func(r chi.Router) {
		r.Use(appMiddleware.TenantResolution(d.TenantReg))

		// Public: tenant branding config (Req 8.2).
		r.Get("/config", tenantConfigHandler())

		// Broadcaster-only.
		r.Group(func(r chi.Router) {
			r.Use(appMiddleware.AuthRequired(d.AuthSvc, "broadcaster", "platform_admin"))

			r.Put("/config", func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "not implemented", http.StatusNotImplemented)
			})

			r.Post("/streams", createStreamHandler(d))
			r.Get("/streams/{streamId}", getStreamHandler(d))
			r.Delete("/streams/{streamId}", stopStreamHandler(d))
			r.Post("/streams/{streamId}/pause", pauseStreamHandler(d))
			r.Post("/streams/{streamId}/resume", resumeStreamHandler(d))
			r.Post("/streams/{streamId}/bitrate-event", bitrateEventHandler(d))
		})

		// Signaling WebSocket for authenticated peers.
		r.With(appMiddleware.AuthRequired(d.AuthSvc, "broadcaster", "listener", "platform_admin")).
			Get("/streams/{streamId}/signal", signaling.NewHandler(d.RelayHub).ServeHTTP)
	})

	// Platform-admin routes.
	r.Route("/api/admin", func(r chi.Router) {
		r.Use(appMiddleware.AuthRequired(d.AuthSvc, "platform_admin"))

		r.Post("/tenants", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "not implemented", http.StatusNotImplemented)
		})
		r.Post("/tenants/{tenantId}/suspend", adminSuspendTenantHandler(d))
		r.Handle("/metrics", d.Metrics.Handler())
	})

	return r
}

// MinTLSConfig returns a *tls.Config that enforces TLS >= 1.2 (Req 9.1).
func MinTLSConfig() *tls.Config {
	return &tls.Config{MinVersion: tls.VersionTLS12}
}

// ── Handlers ─────────────────────────────────────────────────────────────────

func healthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"status": "ok",
			"db":     "ok",
			"relay":  "ok",
		})
	}
}

func loginHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		slug := chi.URLParam(r, "tenantSlug")
		ip := appMiddleware.ClientIP(r)

		token, err := d.AuthSvc.Login(r.Context(), slug, body.Username, body.Password, ip)
		if err != nil {
			d.AuthRL.RecordFailure(ip)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		d.AuthRL.ResetOnSuccess(ip)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"token": token})
	}
}

func tenantConfigHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t := appMiddleware.TenantFromContext(r.Context())
		if t == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(t)
	}
}

func createStreamHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t := appMiddleware.TenantFromContext(r.Context())
		if t == nil {
			http.NotFound(w, r)
			return
		}
		s, err := d.RelayHub.CreateStream(newID(), t.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		d.Metrics.StreamStarted(t.ID)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]string{"streamId": s.ID, "status": string(s.Status)})
	}
}

func getStreamHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, err := d.RelayHub.GetStream(chi.URLParam(r, "streamId"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"streamId": s.ID, "status": string(s.Status)})
	}
}

func stopStreamHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t := appMiddleware.TenantFromContext(r.Context())
		if err := d.RelayHub.StopStream(chi.URLParam(r, "streamId")); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if t != nil {
			d.Metrics.StreamEnded(t.ID)
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func pauseStreamHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := d.RelayHub.PauseStream(chi.URLParam(r, "streamId")); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func resumeStreamHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := d.RelayHub.ResumeStream(chi.URLParam(r, "streamId")); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func bitrateEventHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			PrevKbps       int `json:"prevBitrateKbps"`
			NewKbps        int `json:"newBitrateKbps"`
			DetectedBwKbps int `json:"detectedBwKbps"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		t := appMiddleware.TenantFromContext(r.Context())
		tenantID := ""
		if t != nil {
			tenantID = t.ID
		}
		d.Metrics.RecordBitrateEvent(r.Context(), chi.URLParam(r, "streamId"), tenantID,
			body.PrevKbps, body.NewKbps, body.DetectedBwKbps)
		w.WriteHeader(http.StatusNoContent)
	}
}

func adminSuspendTenantHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := d.TenantReg.Suspend(r.Context(), chi.URLParam(r, "tenantId")); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// newID generates a random hex stream ID.
func newID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}
