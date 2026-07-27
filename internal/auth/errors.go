package auth

import "errors"

// ErrInvalidCredentials is returned by Login when the tenant slug is not found
// or when the username/password combination does not match. A single error is
// used for both cases so that callers cannot distinguish which field was wrong
// (Req 2.2 — no information leakage).
var ErrInvalidCredentials = errors.New("invalid credentials")

// ErrTokenExpired is returned by VerifyToken or Refresh when the JWT's
// expiry claim (exp) is in the past.
var ErrTokenExpired = errors.New("token expired")

// ErrTokenIPMismatch is returned by VerifyToken when the IP address in the
// token's claims does not match the clientIP supplied by the caller (Req 9.4).
var ErrTokenIPMismatch = errors.New("token IP address mismatch")

// ErrRefreshWindowNotOpen is returned by Refresh when the token is still more
// than 15 minutes from expiry — refresh is only permitted within that window
// (Req 2.6).
var ErrRefreshWindowNotOpen = errors.New("refresh window not yet open")
