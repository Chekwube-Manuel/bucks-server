package tenant

// StreamStopper is a callback registered by the relay layer to stop all active
// streams for a tenant when it gets suspended (Req 1.4).
type StreamStopper func() error

// CreateTenantRequest carries the fields required to provision a new tenant.
// Platform-admin only (Req 1.1, 1.2).
type CreateTenantRequest struct {
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
}

// TenantConfigUpdate contains the mutable configuration fields that a
// Broadcaster (or platform-admin) may change via UpdateConfig (Req 8.1).
// Validates: MinBitrateKbps <= DefaultBitrateKbps <= MaxBitrateKbps.
type TenantConfigUpdate struct {
	DisplayName        string
	LogoURL            string
	PrimaryColor       string
	WelcomeMsg         string
	DefaultBitrateKbps int
	MinBitrateKbps     int
	MaxBitrateKbps     int
	Stereo             bool
	JitterBufMs        int
	PublicAccess       bool
}
