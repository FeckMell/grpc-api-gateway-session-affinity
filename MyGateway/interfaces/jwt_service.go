package interfaces

// JwtService validates JWT tokens for routes that require authorization.
//
// ValidateToken(sessionID, token) verifies the token (signature and expiry) and that
// the session_id claim in the token matches the given sessionID (from the session-id
// metadata). Returns (true, nil) if the token is valid and session_id matches;
// (false, nil) if the token is invalid or expired or session mismatch; (false, err)
// if an internal error occurs during validation. "Now" for expiry checks is supplied
// by the implementation (e.g. via TimeProvider in the constructor).
//
// Implemented by service.JWTValidator. Called from helpers.ConfigurableAuthProcessor.Process
// when the matched route has authorization=required.
//
//go:generate moq -stub -out mock/jwt_service.go -pkg mock . JwtService
type JwtService interface {
	// ValidateToken verifies JWT signature and expiry and that the token's session_id claim matches the sessionID from metadata (session-id).
	// Parameters: sessionID — session-id header value; token — raw token string from authorization. Empty token or invalid format yield (false, nil). Token session_id mismatch with sessionID — (false, nil).
	// Returns: (true, nil) when token is valid and session_id matches; (false, nil) when token is invalid, expired or session_id mismatch; (false, err) on internal validation error (e.g. time parse).
	// Called from helpers.ConfigurableAuthProcessor.Process when authorization=required for the matched route.
	ValidateToken(sessionID string, token string) (bool, error)
}
