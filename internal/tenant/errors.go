package tenant

import "errors"

// ErrTenantNotFound is returned by GetBySlug when no tenant matches the given
// slug. The message is deliberately generic to avoid leaking information about
// existing tenants (Req 1.6 / Property 7).
var ErrTenantNotFound = errors.New("tenant not found")

// ErrSlugAlreadyExists is returned by Create when the requested slug is already
// registered to another tenant.
var ErrSlugAlreadyExists = errors.New("tenant slug already exists")

// ErrTenantSuspended is returned when an operation is attempted against a
// tenant whose status is "suspended" (Req 1.4 / Property 3).
var ErrTenantSuspended = errors.New("tenant is suspended")

// ErrInvalidBitrateRange is returned by UpdateConfig when the supplied bitrate
// values violate the constraint minBitrateKbps <= defaultBitrateKbps <= maxBitrateKbps.
var ErrInvalidBitrateRange = errors.New("invalid bitrate range: must satisfy minBitrateKbps <= defaultBitrateKbps <= maxBitrateKbps")
