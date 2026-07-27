-- ============================================================
-- Migration: 001_init.up.sql
-- Creates the initial schema for the church audio streaming platform.
-- ============================================================

-- Tenants
-- One row per church organisation. Slug is the subdomain routing key.
CREATE TABLE tenants (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug          TEXT UNIQUE NOT NULL,            -- subdomain / routing key
    display_name  TEXT NOT NULL,
    contact_email TEXT NOT NULL,
    logo_url      TEXT,
    primary_color TEXT DEFAULT '#1a1a2e',
    welcome_msg   TEXT,
    max_listeners INTEGER NOT NULL DEFAULT 500,
    default_bitrate_kbps INTEGER NOT NULL DEFAULT 64,
    min_bitrate_kbps     INTEGER NOT NULL DEFAULT 24,
    max_bitrate_kbps     INTEGER NOT NULL DEFAULT 128,
    stereo        BOOLEAN NOT NULL DEFAULT TRUE,
    jitter_buf_ms INTEGER NOT NULL DEFAULT 200,
    public_access BOOLEAN NOT NULL DEFAULT TRUE,
    status        TEXT NOT NULL DEFAULT 'active',  -- active | suspended
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Broadcaster credentials (one per tenant for v1)
-- password_hash stores a bcrypt hash with cost >= 12.
CREATE TABLE broadcaster_credentials (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID NOT NULL REFERENCES tenants(id),
    username      TEXT NOT NULL,
    password_hash TEXT NOT NULL,         -- bcrypt cost >= 12
    UNIQUE (tenant_id, username)
);

-- Completed stream records
-- Populated by FinaliseStream when a broadcast ends.
CREATE TABLE stream_records (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        UUID NOT NULL REFERENCES tenants(id),
    started_at       TIMESTAMPTZ NOT NULL,
    ended_at         TIMESTAMPTZ,
    peak_listeners   INTEGER NOT NULL DEFAULT 0,
    avg_bitrate_kbps NUMERIC(6,2),
    broadcaster_id   UUID REFERENCES broadcaster_credentials(id)
);

-- Bitrate change events (adaptive bitrate log)
-- One row per bitrate change emitted by the Adaptive Bitrate Controller.
-- Satisfies Requirement 5.6: every event must record timestamp, prev bitrate,
-- new bitrate, and detected bandwidth.
CREATE TABLE bitrate_events (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    stream_id    UUID NOT NULL REFERENCES stream_records(id),
    tenant_id    UUID NOT NULL REFERENCES tenants(id),
    occurred_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    prev_bitrate INTEGER NOT NULL,
    new_bitrate  INTEGER NOT NULL,
    detected_bw  INTEGER NOT NULL     -- kbps
);

-- Auth rate limit (DB fallback; in-memory store is preferred at runtime)
-- Tracks failed authentication attempts per IP within a sliding window.
-- Satisfies Requirement 9.5: block after 10 failures per IP per minute.
CREATE TABLE auth_rate_limit (
    ip_address   TEXT NOT NULL,
    window_start TIMESTAMPTZ NOT NULL,
    fail_count   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (ip_address, window_start)
);
