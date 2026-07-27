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
	port := getenv("PORT", "8080")
	dsn := getenv("DATABASE_URL", "")
	jwtSecret := getenv("JWT_SECRET", "change-me-in-production")

	authSvc := auth.NewService(jwtSecret, nil)
	tenantReg := tenant.NewRegistry(nil)
	col := metrics.New(nil)
	hub := relay.NewHub()
	rl := auth.NewRateLimiter()

	if dsn != "" {
		p, err := db.Connect(dsn)
		if err != nil {
			slog.Error("db: connect failed", "err", err)
			os.Exit(1)
		}
		defer p.Close()
		authSvc = auth.NewService(jwtSecret, p)
		tenantReg = tenant.NewRegistry(p)
		col = metrics.New(p)
	}

	h := router.New(router.Deps{
		AuthSvc:   authSvc,
		TenantReg: tenantReg,
		RelayHub:  hub,
		Metrics:   col,
		AuthRL:    rl,
	})

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      h,
		TLSConfig:    router.MinTLSConfig(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

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
	slog.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
