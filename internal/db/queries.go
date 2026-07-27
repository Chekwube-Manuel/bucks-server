package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ─── Tenants ────────────────────────────────────────────────────────────────

// GetTenantBySlug returns the Tenant whose slug matches the provided value.
// Returns pgx.ErrNoRows (wrapped) if the tenant does not exist.
func GetTenantBySlug(ctx context.Context, pool *pgxpool.Pool, slug string) (Tenant, error) {
	const q = `
		SELECT id, slug, display_name, contact_email, logo_url, primary_color,
		       welcome_msg, max_listeners, default_bitrate_kbps, min_bitrate_kbps,
		       max_bitrate_kbps, stereo, jitter_buf_ms, public_access, status,
		       created_at, updated_at
		FROM tenants
		WHERE slug = @slug`

	row := pool.QueryRow(ctx, q, pgx.NamedArgs{"slug": slug})
	var t Tenant
	err := row.Scan(
		&t.ID, &t.Slug, &t.DisplayName, &t.ContactEmail, &t.LogoURL,
		&t.PrimaryColor, &t.WelcomeMsg, &t.MaxListeners, &t.DefaultBitrateKbps,
		&t.MinBitrateKbps, &t.MaxBitrateKbps, &t.Stereo, &t.JitterBufMs,
		&t.PublicAccess, &t.Status, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return Tenant{}, fmt.Errorf("GetTenantBySlug: %w", err)
	}
	return t, nil
}

// CreateTenant inserts a new tenant row and returns the assigned ID.
func CreateTenant(ctx context.Context, pool *pgxpool.Pool, t Tenant) (string, error) {
	const q = `
		INSERT INTO tenants (
			slug, display_name, contact_email, logo_url, primary_color, welcome_msg,
			max_listeners, default_bitrate_kbps, min_bitrate_kbps, max_bitrate_kbps,
			stereo, jitter_buf_ms, public_access, status
		) VALUES (
			@slug, @display_name, @contact_email, @logo_url, @primary_color, @welcome_msg,
			@max_listeners, @default_bitrate_kbps, @min_bitrate_kbps, @max_bitrate_kbps,
			@stereo, @jitter_buf_ms, @public_access, @status
		) RETURNING id`

	args := pgx.NamedArgs{
		"slug":                 t.Slug,
		"display_name":         t.DisplayName,
		"contact_email":        t.ContactEmail,
		"logo_url":             t.LogoURL,
		"primary_color":        t.PrimaryColor,
		"welcome_msg":          t.WelcomeMsg,
		"max_listeners":        t.MaxListeners,
		"default_bitrate_kbps": t.DefaultBitrateKbps,
		"min_bitrate_kbps":     t.MinBitrateKbps,
		"max_bitrate_kbps":     t.MaxBitrateKbps,
		"stereo":               t.Stereo,
		"jitter_buf_ms":        t.JitterBufMs,
		"public_access":        t.PublicAccess,
		"status":               t.Status,
	}

	var id string
	if err := pool.QueryRow(ctx, q, args).Scan(&id); err != nil {
		return "", fmt.Errorf("CreateTenant: %w", err)
	}
	return id, nil
}

// UpdateTenantStatus sets the status field (e.g. "active" or "suspended") for
// the tenant identified by tenantID.
func UpdateTenantStatus(ctx context.Context, pool *pgxpool.Pool, tenantID, status string) error {
	const q = `
		UPDATE tenants
		SET    status = @status, updated_at = now()
		WHERE  id = @id`

	tag, err := pool.Exec(ctx, q, pgx.NamedArgs{"status": status, "id": tenantID})
	if err != nil {
		return fmt.Errorf("UpdateTenantStatus: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("UpdateTenantStatus: tenant %q not found", tenantID)
	}
	return nil
}

// TenantConfig holds the mutable configuration fields for a tenant.
type TenantConfig struct {
	DisplayName        string
	LogoURL            string
	PrimaryColor       string
	WelcomeMsg         string
	MaxListeners       int
	DefaultBitrateKbps int
	MinBitrateKbps     int
	MaxBitrateKbps     int
	Stereo             bool
	JitterBufMs        int
	PublicAccess       bool
}

// UpdateTenantConfig writes the configurable fields of a tenant.
func UpdateTenantConfig(ctx context.Context, pool *pgxpool.Pool, tenantID string, cfg TenantConfig) error {
	const q = `
		UPDATE tenants
		SET    display_name        = @display_name,
		       logo_url            = @logo_url,
		       primary_color       = @primary_color,
		       welcome_msg         = @welcome_msg,
		       max_listeners       = @max_listeners,
		       default_bitrate_kbps = @default_bitrate_kbps,
		       min_bitrate_kbps    = @min_bitrate_kbps,
		       max_bitrate_kbps    = @max_bitrate_kbps,
		       stereo              = @stereo,
		       jitter_buf_ms       = @jitter_buf_ms,
		       public_access       = @public_access,
		       updated_at          = now()
		WHERE  id = @id`

	args := pgx.NamedArgs{
		"display_name":         cfg.DisplayName,
		"logo_url":             cfg.LogoURL,
		"primary_color":        cfg.PrimaryColor,
		"welcome_msg":          cfg.WelcomeMsg,
		"max_listeners":        cfg.MaxListeners,
		"default_bitrate_kbps": cfg.DefaultBitrateKbps,
		"min_bitrate_kbps":     cfg.MinBitrateKbps,
		"max_bitrate_kbps":     cfg.MaxBitrateKbps,
		"stereo":               cfg.Stereo,
		"jitter_buf_ms":        cfg.JitterBufMs,
		"public_access":        cfg.PublicAccess,
		"id":                   tenantID,
	}

	tag, err := pool.Exec(ctx, q, args)
	if err != nil {
		return fmt.Errorf("UpdateTenantConfig: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("UpdateTenantConfig: tenant %q not found", tenantID)
	}
	return nil
}

// ─── Broadcaster credentials ─────────────────────────────────────────────────

// GetBroadcasterCredential returns the credential record for the given tenant
// and username. Returns pgx.ErrNoRows (wrapped) if not found.
func GetBroadcasterCredential(ctx context.Context, pool *pgxpool.Pool, tenantID, username string) (BroadcasterCredential, error) {
	const q = `
		SELECT id, tenant_id, username, password_hash
		FROM   broadcaster_credentials
		WHERE  tenant_id = @tenant_id AND username = @username`

	row := pool.QueryRow(ctx, q, pgx.NamedArgs{"tenant_id": tenantID, "username": username})
	var c BroadcasterCredential
	err := row.Scan(&c.ID, &c.TenantID, &c.Username, &c.PasswordHash)
	if err != nil {
		return BroadcasterCredential{}, fmt.Errorf("GetBroadcasterCredential: %w", err)
	}
	return c, nil
}

// CreateBroadcasterCredential inserts a new broadcaster credential row and
// returns the assigned ID.
func CreateBroadcasterCredential(ctx context.Context, pool *pgxpool.Pool, c BroadcasterCredential) (string, error) {
	const q = `
		INSERT INTO broadcaster_credentials (tenant_id, username, password_hash)
		VALUES (@tenant_id, @username, @password_hash)
		RETURNING id`

	args := pgx.NamedArgs{
		"tenant_id":     c.TenantID,
		"username":      c.Username,
		"password_hash": c.PasswordHash,
	}

	var id string
	if err := pool.QueryRow(ctx, q, args).Scan(&id); err != nil {
		return "", fmt.Errorf("CreateBroadcasterCredential: %w", err)
	}
	return id, nil
}

// ─── Stream records ───────────────────────────────────────────────────────────

// CreateStreamRecord inserts a new stream record and returns the assigned ID.
// Call this when the broadcaster starts a stream.
func CreateStreamRecord(ctx context.Context, pool *pgxpool.Pool, tenantID, broadcasterID string, startedAt time.Time) (string, error) {
	const q = `
		INSERT INTO stream_records (tenant_id, broadcaster_id, started_at)
		VALUES (@tenant_id, @broadcaster_id, @started_at)
		RETURNING id`

	args := pgx.NamedArgs{
		"tenant_id":      tenantID,
		"broadcaster_id": broadcasterID,
		"started_at":     startedAt,
	}

	var id string
	if err := pool.QueryRow(ctx, q, args).Scan(&id); err != nil {
		return "", fmt.Errorf("CreateStreamRecord: %w", err)
	}
	return id, nil
}

// UpdateStreamRecord updates the mutable fields of a stream record.
// Call this when finalising a stream (setting ended_at, peak_listeners, avg_bitrate_kbps).
func UpdateStreamRecord(ctx context.Context, pool *pgxpool.Pool, s StreamRecord) error {
	const q = `
		UPDATE stream_records
		SET    ended_at         = @ended_at,
		       peak_listeners   = @peak_listeners,
		       avg_bitrate_kbps = @avg_bitrate_kbps
		WHERE  id = @id`

	args := pgx.NamedArgs{
		"ended_at":         s.EndedAt,
		"peak_listeners":   s.PeakListeners,
		"avg_bitrate_kbps": s.AvgBitrateKbps,
		"id":               s.ID,
	}

	tag, err := pool.Exec(ctx, q, args)
	if err != nil {
		return fmt.Errorf("UpdateStreamRecord: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("UpdateStreamRecord: stream %q not found", s.ID)
	}
	return nil
}

// GetStreamRecord retrieves a stream record by its ID.
func GetStreamRecord(ctx context.Context, pool *pgxpool.Pool, streamID string) (StreamRecord, error) {
	const q = `
		SELECT id, tenant_id, started_at, ended_at,
		       peak_listeners, avg_bitrate_kbps, broadcaster_id
		FROM   stream_records
		WHERE  id = @id`

	row := pool.QueryRow(ctx, q, pgx.NamedArgs{"id": streamID})
	var s StreamRecord
	err := row.Scan(
		&s.ID, &s.TenantID, &s.StartedAt, &s.EndedAt,
		&s.PeakListeners, &s.AvgBitrateKbps, &s.BroadcasterID,
	)
	if err != nil {
		return StreamRecord{}, fmt.Errorf("GetStreamRecord: %w", err)
	}
	return s, nil
}

// ─── Bitrate events ───────────────────────────────────────────────────────────

// InsertBitrateEvent inserts a single bitrate change event row.
func InsertBitrateEvent(ctx context.Context, pool *pgxpool.Pool, e BitrateEvent) error {
	const q = `
		INSERT INTO bitrate_events (stream_id, tenant_id, prev_bitrate, new_bitrate, detected_bw)
		VALUES (@stream_id, @tenant_id, @prev_bitrate, @new_bitrate, @detected_bw)`

	args := pgx.NamedArgs{
		"stream_id":    e.StreamID,
		"tenant_id":    e.TenantID,
		"prev_bitrate": e.PrevBitrate,
		"new_bitrate":  e.NewBitrate,
		"detected_bw":  e.DetectedBW,
	}

	if _, err := pool.Exec(ctx, q, args); err != nil {
		return fmt.Errorf("InsertBitrateEvent: %w", err)
	}
	return nil
}

// ─── Auth rate limit ──────────────────────────────────────────────────────────

// GetRateLimitCount returns the current fail count for an IP address within the
// specified window. Returns 0 if no row exists (first attempt in window).
func GetRateLimitCount(ctx context.Context, pool *pgxpool.Pool, ipAddress string, windowStart time.Time) (int, error) {
	const q = `
		SELECT fail_count
		FROM   auth_rate_limit
		WHERE  ip_address = @ip_address AND window_start = @window_start`

	var count int
	err := pool.QueryRow(ctx, q, pgx.NamedArgs{
		"ip_address":   ipAddress,
		"window_start": windowStart,
	}).Scan(&count)
	if err != nil {
		if err == pgx.ErrNoRows {
			return 0, nil
		}
		return 0, fmt.Errorf("GetRateLimitCount: %w", err)
	}
	return count, nil
}

// IncrementRateLimitCount atomically upserts the fail counter for the given IP
// and window, incrementing it by 1.
func IncrementRateLimitCount(ctx context.Context, pool *pgxpool.Pool, ipAddress string, windowStart time.Time) error {
	const q = `
		INSERT INTO auth_rate_limit (ip_address, window_start, fail_count)
		VALUES (@ip_address, @window_start, 1)
		ON CONFLICT (ip_address, window_start)
		DO UPDATE SET fail_count = auth_rate_limit.fail_count + 1`

	if _, err := pool.Exec(ctx, q, pgx.NamedArgs{
		"ip_address":   ipAddress,
		"window_start": windowStart,
	}); err != nil {
		return fmt.Errorf("IncrementRateLimitCount: %w", err)
	}
	return nil
}

// ResetRateLimitCount deletes the rate-limit row for the given IP and window,
// effectively resetting the counter (e.g. after a successful login).
func ResetRateLimitCount(ctx context.Context, pool *pgxpool.Pool, ipAddress string, windowStart time.Time) error {
	const q = `
		DELETE FROM auth_rate_limit
		WHERE  ip_address = @ip_address AND window_start = @window_start`

	if _, err := pool.Exec(ctx, q, pgx.NamedArgs{
		"ip_address":   ipAddress,
		"window_start": windowStart,
	}); err != nil {
		return fmt.Errorf("ResetRateLimitCount: %w", err)
	}
	return nil
}
