package db

import (
	"time"
)

// Tenant represents a row in the tenants table.
// Each church organisation has exactly one Tenant record.
type Tenant struct {
	ID                 string
	Slug               string
	DisplayName        string
	ContactEmail       string
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
	Status             string // "active" | "suspended"
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// BroadcasterCredential represents a row in the broadcaster_credentials table.
// Each credential is scoped to a single tenant.
type BroadcasterCredential struct {
	ID           string
	TenantID     string
	Username     string
	PasswordHash string // bcrypt hash, cost >= 12
}

// StreamRecord represents a row in the stream_records table.
// Populated by metrics.FinaliseStream when a broadcast ends.
type StreamRecord struct {
	ID             string
	TenantID       string
	StartedAt      time.Time
	EndedAt        *time.Time // nullable until stream ends
	PeakListeners  int
	AvgBitrateKbps *float64   // nullable until stream ends
	BroadcasterID  *string    // nullable (anonymous broadcast path)
}

// BitrateEvent represents a row in the bitrate_events table.
// One row is inserted for every adaptive-bitrate change emitted by the client.
type BitrateEvent struct {
	ID          string
	StreamID    string
	TenantID    string
	OccurredAt  time.Time
	PrevBitrate int // kbps
	NewBitrate  int // kbps
	DetectedBW  int // kbps
}

// AuthRateLimit represents a row in the auth_rate_limit table.
// Tracks failed authentication attempts per IP address within a time window.
type AuthRateLimit struct {
	IPAddress   string
	WindowStart time.Time
	FailCount   int
}
