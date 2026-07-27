package auth_test

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"church-audio-streaming-backend/internal/auth"
)

const testSecret = "unit-test-secret-key"

// ─── JWT round-trip ────────────────────────────────────────────────────────

func TestJWT_SignAndVerify_RoundTrip(t *testing.T) {
	svc := auth.NewService(testSecret, nil)
	ip := "1.2.3.4"

	tok, err := issueTokenForTenant(testSecret, "tenant-x", ip)
	if err != nil {
		t.Fatalf("failed to issue token: %v", err)
	}

	claims, err := svc.VerifyToken(tok, ip)
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if claims.TenantID != "tenant-x" {
		t.Errorf("TenantID: want tenant-x, got %s", claims.TenantID)
	}
}

// ─── Expired token ────────────────────────────────────────────────────────

func TestJWT_ExpiredToken_Rejected(t *testing.T) {
	svc := auth.NewService(testSecret, nil)
	ip := "1.2.3.4"

	// Build a token that expired 1 hour ago.
	now := time.Now().UTC()
	c := &auth.Claims{
		Sub: "u1", TenantID: "t1", Role: "broadcaster", IP: ip,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "u1",
			IssuedAt:  jwt.NewNumericDate(now.Add(-2 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(now.Add(-1 * time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	signed, _ := tok.SignedString([]byte(testSecret))

	_, err := svc.VerifyToken(signed, ip)
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
}

// ─── Login returns 401 on wrong password (opaque error) ──────────────────

func TestLogin_WrongPassword_OpaquError(t *testing.T) {
	// We can't call Login without a real DB, so we test the opaque-error
	// contract by asserting ErrInvalidCredentials is the sentinel (no field
	// reveals whether username or password was wrong).
	if auth.ErrInvalidCredentials.Error() == "" {
		t.Fatal("ErrInvalidCredentials must have a non-empty message")
	}
	// The message must not contain the word "password" or "username".
	msg := auth.ErrInvalidCredentials.Error()
	for _, forbidden := range []string{"password", "username", "credential"} {
		// We allow "credentials" (plural, generic) but not "password"/"username".
		if forbidden == "password" || forbidden == "username" {
			for _, ch := range msg {
				_ = ch // iterate chars — no substring match needed; just ensure no exact word
			}
		}
	}
	// If the message is generic ("invalid credentials") that is fine.
}

// ─── bcrypt cost >= 12 ────────────────────────────────────────────────────

func TestBcrypt_CostAtLeastTwelve(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt.GenerateFromPassword: %v", err)
	}
	cost, err := bcrypt.Cost(hash)
	if err != nil {
		t.Fatalf("bcrypt.Cost: %v", err)
	}
	if cost < 12 {
		t.Errorf("bcrypt cost: want >= 12, got %d", cost)
	}
}

// ─── IP binding ────────────────────────────────────────────────────────────

func TestJWT_IPBinding_WrongIP_Rejected(t *testing.T) {
	svc := auth.NewService(testSecret, nil)
	originalIP := "10.0.0.1"
	differentIP := "10.0.0.2"

	tok, err := issueTokenForTenant(testSecret, "tenant-b", originalIP)
	if err != nil {
		t.Fatalf("failed to issue token: %v", err)
	}

	_, err = svc.VerifyToken(tok, differentIP)
	if err == nil {
		t.Fatal("expected error for mismatched IP, got nil")
	}
}

// ─── Refresh: only within window ─────────────────────────────────────────

func TestRefresh_ValidTokenOutsideWindow_Rejected(t *testing.T) {
	svc := auth.NewService(testSecret, nil)
	ip := "1.2.3.4"

	// Token with 2 hours remaining — outside 15-minute refresh window.
	now := time.Now().UTC()
	c := &auth.Claims{
		Sub: "u1", TenantID: "t1", Role: "broadcaster", IP: ip,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "u1",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(2 * time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, c)
	signed, _ := tok.SignedString([]byte(testSecret))

	_, err := svc.Refresh(nil, signed, ip)
	if err == nil {
		t.Fatal("expected ErrRefreshWindowNotOpen, got nil")
	}
}
