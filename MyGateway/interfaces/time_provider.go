package interfaces

import "time"

// TimeProvider supplies the current time for JWT expiry checks and logging.
// Injected so tests can use a fixed clock instead of time.Now().
//
// Used by service.JWTValidator to compare "now" with the token's expires_at claim.
// Constructed in cmd/main as TimeProviderFunc(func() time.Time { return time.Now().UTC() }).
//
//go:generate moq -stub -out mock/time_provider.go -pkg mock . TimeProvider
type TimeProvider interface {
	// Now returns current time (UTC in prod; in tests — fixed time for deterministic expiry checks).
	// Parameters: none.
	// Returns: time.Time — "now" for comparison with expires_at in the JWT.
	// Called from service.jwtValidator.ValidateToken when checking token expiry.
	Now() time.Time
}
