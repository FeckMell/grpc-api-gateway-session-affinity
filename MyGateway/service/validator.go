package service

import (
	"time"

	"mygateway/auth"
	"mygateway/helpers"
	"mygateway/interfaces"
)

// jwtValidator implements interfaces.JwtService. It validates tokens using ParseAndVerify (signature)
// and then checks expiry (using the injected TimeProvider.Now()) and that the token's session_id claim
// matches the sessionID argument (from the session-id metadata). Used for routes with authorization=required.
type jwtValidator struct {
	secret       []byte
	timeProvider interfaces.TimeProvider
}

// NewJWTValidator creates a JwtService that validates tokens with the given secret (HMAC-SHA256); time for expiry check comes from timeProvider.
//
// Parameters: secret — shared HMAC key (must match MyAuth); timeProvider — source of current time (prod — time.Now().UTC(), tests — fixed). Panics on nil secret or timeProvider.
//
// Returns: interfaces.JwtService (*jwtValidator).
//
// Called from cmd/main when building the header chain.
func NewJWTValidator(secret []byte, timeProvider interfaces.TimeProvider) interfaces.JwtService {
	return &jwtValidator{
		secret:       helpers.NilPanic(secret, "service.validator.go: secret is required"),
		timeProvider: helpers.NilPanic(timeProvider, "service.validator.go: time provider is required"),
	}
}

// ValidateToken verifies token signature (ParseAndVerify), parses ExpiresAt (RFC3339), compares "now" with expiry and claims.SessionID with the given sessionID.
//
// Parameters: sessionID — session-id metadata value; token — authorization metadata value. Empty/invalid token or session_id mismatch yield (false, nil).
//
// Returns: (true, nil) when token is valid and session_id matches; (false, nil) when invalid, expired or session_id mismatch; (false, err) on internal error (e.g. time parse).
//
// Called from helpers.ConfigurableAuthProcessor.Process when authorization=required for the matched route.
func (v *jwtValidator) ValidateToken(sessionID string, token string) (bool, error) {
	claims, err := auth.ParseAndVerify(token, v.secret)
	if err != nil {
		return false, nil
	}
	t, err := time.Parse(time.RFC3339, claims.ExpiresAt)
	if err != nil {
		return false, nil
	}
	now := v.timeProvider.Now()
	if now.After(t) {
		return false, nil
	}
	if claims.SessionID != sessionID {
		return false, nil
	}
	return true, nil
}
