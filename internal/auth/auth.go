package auth

import (
"context"
"errors"
"fmt"
"time"

"github.com/golang-jwt/jwt/v5"
"github.com/jackc/pgx/v5"
"github.com/jackc/pgx/v5/pgxpool"
"golang.org/x/crypto/bcrypt"

"church-audio-streaming-backend/internal/db"
)

const (
// tokenValidity is the lifetime of a newly issued JWT (Req 2.1).
tokenValidity = 8 * time.Hour

// refreshWindow is the duration before expiry within which a token may be
// refreshed (Req 2.6).  The token must be non-expired AND within this
// window.
refreshWindow = 15 * time.Minute
)

// Claims carries the application-specific JWT payload.
// It embeds jwt.RegisteredClaims to include standard fields (exp, iat, etc.).
type Claims struct {
Sub      string `json:"sub"`
TenantID string `json:"tenantId"`
Role     string `json:"role"` // "broadcaster" | "listener" | "platform_admin"
IP       string `json:"ip"`
jwt.RegisteredClaims
}

// Service holds the dependencies needed to issue and verify tokens.
type Service struct {
secret []byte
db     *pgxpool.Pool
}

// NewService creates a new auth Service.
//   - secret: the HMAC-SHA256 signing key (JWT_SECRET env var)
//   - db: an open pgxpool connection pool used to look up tenants and
//     broadcaster credentials
func NewService(secret string, db *pgxpool.Pool) *Service {
return &Service{
secret: []byte(secret),
db:     db,
}
}

// Login validates broadcaster credentials and returns a signed JWT on success.
//
// Flow:
//  1. Look up the tenant by slug; return ErrInvalidCredentials on not-found so
//     the caller cannot tell whether the slug or the username was wrong
//     (Req 2.2).
//  2. Look up the broadcaster credential for (tenantID, username).
//  3. Compare the supplied password against the bcrypt hash (cost >= 12,
//     Req 9.3).
//  4. On success, issue a signed HS256 JWT with the required claims (Req 2.1,
//     9.4).
func (s *Service) Login(ctx context.Context, tenantSlug, username, password, clientIP string) (string, error) {
// Step 1: resolve tenant — use ErrInvalidCredentials to avoid leaking
// whether the slug is known.
tenant, err := db.GetTenantBySlug(ctx, s.db, tenantSlug)
if err != nil {
if errors.Is(err, pgx.ErrNoRows) {
return "", ErrInvalidCredentials
}
return "", fmt.Errorf("Login: tenant lookup: %w", err)
}

// Step 2: resolve broadcaster credential.
cred, err := db.GetBroadcasterCredential(ctx, s.db, tenant.ID, username)
if err != nil {
if errors.Is(err, pgx.ErrNoRows) {
return "", ErrInvalidCredentials
}
return "", fmt.Errorf("Login: credential lookup: %w", err)
}

// Step 3: constant-time bcrypt comparison (Req 9.3).
if err := bcrypt.CompareHashAndPassword([]byte(cred.PasswordHash), []byte(password)); err != nil {
// bcrypt.ErrMismatchedHashAndPassword and bcrypt.ErrHashTooShort both
// map to the same opaque error to prevent oracle attacks (Req 2.2).
return "", ErrInvalidCredentials
}

// Step 4: issue JWT.
return s.issueToken(cred.ID, tenant.ID, "broadcaster", clientIP)
}

// Refresh issues a new JWT using an existing, valid token.
//
// Rules (Req 2.6 / Property 11):
//   - The presented token must be valid (not expired, signature OK).
//   - Refresh is only allowed if the token is within the refreshWindow (i.e.
//     exp - now <= 15 min).  A token with more than 15 minutes of remaining
//     validity is rejected with ErrRefreshWindowNotOpen.
func (s *Service) Refresh(ctx context.Context, tokenString, clientIP string) (string, error) {
claims, err := s.parseAndVerify(tokenString)
if err != nil {
return "", err
}

expiry := claims.RegisteredClaims.ExpiresAt.Time
remaining := time.Until(expiry)

// Reject tokens that still have more than 15 minutes left.
if remaining > refreshWindow {
return "", ErrRefreshWindowNotOpen
}

// Issue a fresh token with the same identity claims.
return s.issueToken(claims.Sub, claims.TenantID, claims.Role, clientIP)
}

// VerifyToken validates the JWT signature, expiry, and IP binding.
//
// Returns the parsed Claims on success.  Returns:
//   - ErrTokenExpired if the token has expired.
//   - ErrTokenIPMismatch if claims.IP != clientIP (Req 9.4).
//   - A wrapped error for any other parse / signature failure.
func (s *Service) VerifyToken(tokenString, clientIP string) (*Claims, error) {
claims, err := s.parseAndVerify(tokenString)
if err != nil {
return nil, err
}

// IP binding check (Req 9.4 / session invalidation).
if claims.IP != clientIP {
return nil, ErrTokenIPMismatch
}

return claims, nil
}

// parseAndVerify parses the JWT string, verifies the HS256 signature, and
// checks the standard expiry claim.  It does NOT perform IP binding — that is
// left to VerifyToken so that Refresh can read claims without enforcing the
// origin IP (the caller may be refreshing from the same IP, which is fine, but
// we keep the semantics consistent).
func (s *Service) parseAndVerify(tokenString string) (*Claims, error) {
token, err := jwt.ParseWithClaims(
tokenString,
&Claims{},
func(t *jwt.Token) (interface{}, error) {
// Enforce HS256 — reject tokens signed with any other algorithm
// to prevent the "alg:none" attack.
if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
}
return s.secret, nil
},
jwt.WithValidMethods([]string{"HS256"}),
)

if err != nil {
// Map JWT library's expiry error to our sentinel.
if errors.Is(err, jwt.ErrTokenExpired) {
return nil, ErrTokenExpired
}
return nil, fmt.Errorf("parseAndVerify: %w", err)
}

claims, ok := token.Claims.(*Claims)
if !ok || !token.Valid {
return nil, fmt.Errorf("parseAndVerify: invalid token claims")
}

return claims, nil
}

// issueToken constructs and signs a new HS256 JWT with the supplied identity
// fields.  exp = now + 8h, iat = now.
func (s *Service) issueToken(sub, tenantID, role, clientIP string) (string, error) {
now := time.Now().UTC()
claims := &Claims{
Sub:      sub,
TenantID: tenantID,
Role:     role,
IP:       clientIP,
RegisteredClaims: jwt.RegisteredClaims{
Subject:   sub,
IssuedAt:  jwt.NewNumericDate(now),
ExpiresAt: jwt.NewNumericDate(now.Add(tokenValidity)),
},
}

token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
signed, err := token.SignedString(s.secret)
if err != nil {
return "", fmt.Errorf("issueToken: %w", err)
}
return signed, nil
}
