package tenant

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"church-audio-streaming-backend/internal/db"
)

// Registry is the Tenant Registry component described in the design.
// It holds a DB pool for persistence and an in-memory map of StreamStoppers
// that lets the relay terminate active streams on tenant suspension (Req 1.4).
type Registry struct {
	db      *pgxpool.Pool
	mu      sync.RWMutex
	streams map[string]StreamStopper // tenantID -> stopper
}

// NewRegistry creates a new Registry backed by the given connection pool.
func NewRegistry(pool *pgxpool.Pool) *Registry {
	return &Registry{
		db:      pool,
		streams: make(map[string]StreamStopper),
	}
}

// GetBySlug looks up a Tenant by subdomain slug.
// Returns ErrTenantNotFound if no tenant matches — no information leakage
// about other tenants (Req 1.6 / Property 7).
func (r *Registry) GetBySlug(ctx context.Context, slug string) (*db.Tenant, error) {
	t, err := db.GetTenantBySlug(ctx, r.db, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrTenantNotFound
		}
		return nil, fmt.Errorf("GetBySlug: %w", err)
	}
	return &t, nil
}

// Create provisions a new tenant. Returns ErrSlugAlreadyExists if the slug is
// already registered. Platform-admin only; the caller is responsible for the
// auth check (Req 1.1, 1.2).
func (r *Registry) Create(ctx context.Context, req CreateTenantRequest) (*db.Tenant, error) {
	// Apply defaults for any zero-value numeric fields.
	if req.MaxListeners == 0 {
		req.MaxListeners = 500
	}
	if req.DefaultBitrateKbps == 0 {
		req.DefaultBitrateKbps = 64
	}
	if req.MinBitrateKbps == 0 {
		req.MinBitrateKbps = 24
	}
	if req.MaxBitrateKbps == 0 {
		req.MaxBitrateKbps = 128
	}
	if req.JitterBufMs == 0 {
		req.JitterBufMs = 200
	}

	t := db.Tenant{
		Slug:               req.Slug,
		DisplayName:        req.DisplayName,
		ContactEmail:       req.ContactEmail,
		LogoURL:            req.LogoURL,
		PrimaryColor:       req.PrimaryColor,
		WelcomeMsg:         req.WelcomeMsg,
		MaxListeners:       req.MaxListeners,
		DefaultBitrateKbps: req.DefaultBitrateKbps,
		MinBitrateKbps:     req.MinBitrateKbps,
		MaxBitrateKbps:     req.MaxBitrateKbps,
		Stereo:             req.Stereo,
		JitterBufMs:        req.JitterBufMs,
		PublicAccess:       req.PublicAccess,
		Status:             "active",
	}

	id, err := db.CreateTenant(ctx, r.db, t)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, ErrSlugAlreadyExists
		}
		return nil, fmt.Errorf("Create: %w", err)
	}
	t.ID = id
	return &t, nil
}

// Suspend marks a tenant as suspended in the DB and calls any registered
// StreamStopper to terminate active streams (Req 1.4 / Property 3).
// After suspension, GetBySlug still returns the tenant with Status="suspended".
func (r *Registry) Suspend(ctx context.Context, tenantID string) error {
	if err := db.UpdateTenantStatus(ctx, r.db, tenantID, "suspended"); err != nil {
		return fmt.Errorf("Suspend: %w", err)
	}

	// Terminate any active streams for this tenant.
	r.mu.Lock()
	stopper, ok := r.streams[tenantID]
	delete(r.streams, tenantID)
	r.mu.Unlock()

	if ok && stopper != nil {
		if err := stopper(); err != nil {
			return fmt.Errorf("Suspend: stream stopper: %w", err)
		}
	}
	return nil
}

// UpdateConfig updates the mutable configuration fields for a tenant.
// Validates: MinBitrateKbps <= DefaultBitrateKbps <= MaxBitrateKbps (Req 4.5).
func (r *Registry) UpdateConfig(ctx context.Context, tenantID string, cfg TenantConfigUpdate) error {
	if cfg.MinBitrateKbps > cfg.DefaultBitrateKbps || cfg.DefaultBitrateKbps > cfg.MaxBitrateKbps {
		return ErrInvalidBitrateRange
	}
	dbCfg := db.TenantConfig{
		DisplayName:        cfg.DisplayName,
		LogoURL:            cfg.LogoURL,
		PrimaryColor:       cfg.PrimaryColor,
		WelcomeMsg:         cfg.WelcomeMsg,
		DefaultBitrateKbps: cfg.DefaultBitrateKbps,
		MinBitrateKbps:     cfg.MinBitrateKbps,
		MaxBitrateKbps:     cfg.MaxBitrateKbps,
		Stereo:             cfg.Stereo,
		JitterBufMs:        cfg.JitterBufMs,
		PublicAccess:       cfg.PublicAccess,
	}
	if err := db.UpdateTenantConfig(ctx, r.db, tenantID, dbCfg); err != nil {
		return fmt.Errorf("UpdateConfig: %w", err)
	}
	return nil
}

// RegisterStreamStopper lets the relay register a callback to stop all streams
// for a tenant when it gets suspended (Req 1.4).
func (r *Registry) RegisterStreamStopper(tenantID string, stopper StreamStopper) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.streams[tenantID] = stopper
}

// UnregisterStreamStopper removes the stopper when all streams have ended.
func (r *Registry) UnregisterStreamStopper(tenantID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.streams, tenantID)
}

// isUniqueViolation returns true when err is a PostgreSQL unique-constraint
// violation (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pgErr interface{ SQLState() string }
	if errors.As(err, &pgErr) {
		return pgErr.SQLState() == "23505"
	}
	return false
}
