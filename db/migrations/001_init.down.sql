-- ============================================================
-- Migration: 001_init.down.sql
-- Drops all tables created by 001_init.up.sql in reverse
-- dependency order so that foreign key constraints are not
-- violated during rollback.
-- ============================================================

-- 1. Drop tables that reference others first.
DROP TABLE IF EXISTS bitrate_events;
DROP TABLE IF EXISTS auth_rate_limit;
DROP TABLE IF EXISTS stream_records;
DROP TABLE IF EXISTS broadcaster_credentials;

-- 2. Drop the root table last.
DROP TABLE IF EXISTS tenants;
