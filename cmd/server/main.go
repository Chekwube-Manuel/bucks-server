package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"church-audio-streaming-backend/internal/auth"
	"church-audio-streaming-backend/internal/db"
	"church-audio-streaming-backend/internal/metrics"
	"church-audio-streaming-backend/internal/relay"
	"church-audio-streaming-backend/internal/router"
	"church-audio-streaming-backend/internal/tenant"
)

func main() {
	// ── Configuration from environment ──────────────────────────────────────
	port := getenv("PORT", "8080")
	dsn := getenv("DATABASE_URL", "")
	jwtSecret := getenv("JWT_SECRET", "change-me-in-production")

	// ── Database ─────────────────────────────────────────────────────────────
	var pool interface{ Close() } // lazy — only connect when DSN is set
	if dsn != "" {
		p, err := db.Connect(dsn)
		if err != nil {
			slog.Error("db: connect failed", "err", err)
			os.Exit(1)
		}
		pool = p
		defer p.Close()
	}

	pgPool := func() interface{ Close() } { return pool }()

	// ── Services ──────────────────────────────────────────────────────────────
	var pgxPool interface{}
	if pgPool != nil {
		pgxPool = pgPool
	}
	_ = pgxPool

	authSvc := auth.NewService(jwtSecret, nil) // nil pool: login disabled until DB is up
	tenantReg := tenant.NewRegistry(nil)
	hub := relay.NewHub()
	col := metrics.New(nil)
	rl := auth.NewRateLimiter()

	// If a real DB connection is available, wire it in.
	if dsn != "" {
		p, err := db.Connect(dsn)
		if err == nil {
			authSvc = auth.NewService(jwtSecret, p)
			tenantReg = tenant.NewRegistry(p)
			col = metrics.New(p)
			defer p.Close()
		}
	}

	// ── Router ────────────────────────────────────────────────────────────────
	h := router.New(router.Deps{
		AuthSvc:   authSvc,
		TenantReg: tenantReg,
		RelayHub:  hub,
		Metrics:   col,
		AuthRL:    rl,
	})

	// ── HTTP server with TLS >= 1.2 ───────────────────────────────────────────
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      h,
		TLSConfig:    router.MinTLSConfig(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// ── Graceful shutdown ────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		slog.Info("server starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "err", err)
			os.Exit(1)
		}
	}()

	<-quit
	slog.Info("server shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("shutdown error", "err", err)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
