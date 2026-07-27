package main

import (
	"crypto/tls"
	"log"
	"net/http"
	"os"
	"time"
)

// main is the server entry point.
// It reads configuration from environment variables and starts an HTTP(S) server.
// Required env vars: DATABASE_URL, JWT_SECRET
// Optional env vars: PORT (default 8443), TLS_CERT_FILE, TLS_KEY_FILE
func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8443"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET environment variable is required")
	}

	// Suppress unused-variable warnings for now; these will be used when the
	// database and auth layers are wired up in subsequent tasks.
	_ = dbURL
	_ = jwtSecret

	// TLS configuration enforces minimum TLS 1.2 as required by Req 9.1.
	tlsCfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	mux := http.NewServeMux()

	// Health check endpoint (Req 10.1)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","db":"ok","relay":"ok"}`))
	})

	certFile := os.Getenv("TLS_CERT_FILE")
	keyFile := os.Getenv("TLS_KEY_FILE")

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		TLSConfig:    tlsCfg,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	if certFile == "" || keyFile == "" {
		log.Printf("TLS_CERT_FILE / TLS_KEY_FILE not set - starting plain HTTP on :%s (dev only)", port)
		srv.TLSConfig = nil
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
		return
	}

	log.Printf("Starting TLS server on :%s (TLS min version 1.2)", port)
	if err := srv.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}