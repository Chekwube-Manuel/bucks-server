package auth_test

import (
	"testing"
	"time"

	"github.com/flyingmutant/rapid"
	"github.com/golang-jwt/jwt/v5"

	"church-audio-streaming-backend/internal/auth"
)

// TestProperty11_TokenRefreshWindowOnly verifies Property 11:
// Refresh succeeds iff the token is non-expired AND was issued at least
// 15 minutes before expiry (i.e. remaining validity <= 15 min).
func TestProperty11_TokenRefreshWindowOnly(t *testing.T) {
	secret := "test-secret-property-11"
	svc := auth.NewService(secret, nil)
	ip := "10.0.0.1"

	rapid.Check(t, func(tc *rapid.T) {
		// offsetSec: seconds until expiry from now.
		// Positive => token still valid. Negative => token already expired.
		offsetSec := rapid.Int64Range(-3600, 3600).Draw(tc, "offsetSec").(int64)

		now := time.Now().UTC()
		expiry := now.Add(time.Duration(offsetSec) * time.Second)

		rawClaims := &auth.Claims{
			Sub:      "u1",
			TenantID: "tenant-a",
			Role:     "broadcaster",
			IP:       ip,
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   "u1",
				IssuedAt:  jwt.NewNumericDate(now),
				ExpiresAt: jwt.NewNumericDate(expiry),
			},
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodHS256, rawClaims)
		signed, err := tok.SignedString([]byte(secret))
		if err != nil {
			tc.Fatalf("failed to sign token: %v", err)
		}

		refreshErr := tryRefresh(svc, signed, ip)

		expired := offsetSec <= 0
		withinWindow := !expired && time.Duration(offsetSec)*time.Second <= 15*time.Minute

		if withinWindow {
			if refreshErr != nil {
				tc.Fatalf(
					"offset=%ds: expected refresh to succeed (within window), got: %v",
					offsetSec, refreshErr,
				)
			}
		} else if expired {
			if refreshErr == nil {
				tc.Fatalf("offset=%ds: expected ErrTokenExpired, got nil", offsetSec)
			}
		} else {
			if refreshErr == nil {
				tc.Fatalf(
					"offset=%ds: expected ErrRefreshWindowNotOpen, got nil",
					offsetSec,
				)
			}
		}
	})
}

// tryRefresh calls Refresh and returns the error (nil on success).
func tryRefresh(svc *auth.Service, token, ip string) error {
	_, err := svc.Refresh(nil, token, ip) //nolint:staticcheck
	return err
}
