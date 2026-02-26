package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrInvalidTokenFormat is returned when the token does not have exactly two parts separated by ".".
var ErrInvalidTokenFormat = errors.New("invalid token format: expected base64(payload).base64(signature)")

// ErrInvalidSignature is returned when the token signature does not match the expected HMAC-SHA256.
var ErrInvalidSignature = errors.New("invalid token signature")

// TokenClaims is the JWT payload decoded from the token: login, role, session_id, expires_at (RFC3339), issued_at (RFC3339).
// Used by CreateToken to build the payload and by ParseAndVerify to return the decoded claims.
type TokenClaims struct {
	Login     string `json:"login"`
	Role      string `json:"role"`
	SessionID string `json:"session_id"`
	ExpiresAt string `json:"expires_at"` // RFC3339
	IssuedAt  string `json:"issued_at"`  // RFC3339
}

// CreateToken builds a token in MyAuth format: JSON payload (login, role, session_id, expires_at, issued_at), HMAC-SHA256(secret, payload) signature, payload and signature in base64, joined as "payload.signature". The gateway only verifies tokens (ParseAndVerify); creation is used in tests and external issuers.
//
// Parameters: login, role, sessionID — claim fields; expiresAt, issuedAt — serialized as RFC3339; secret — HMAC key (must match MyAuth). Empty secret is allowed but signature will be predictable.
//
// Returns: (token string, nil) on success; ("", error) on json.Marshal error.
//
// Called from tests and from code that issues tokens (not from the gateway at runtime).
func CreateToken(login, role, sessionID string, expiresAt, issuedAt time.Time, secret []byte) (string, error) {
	claims := TokenClaims{
		Login:     login,
		Role:      role,
		SessionID: sessionID,
		ExpiresAt: expiresAt.Format(time.RFC3339),
		IssuedAt:  issuedAt.Format(time.RFC3339),
	}

	payload, err := json.Marshal(claims)
	if err != nil {
		return "", fmt.Errorf("marshal claims: %w", err)
	}

	mac := hmac.New(sha256.New, secret)
	mac.Write(payload)
	signature := mac.Sum(nil)

	payloadB64 := base64.StdEncoding.EncodeToString(payload)
	signatureB64 := base64.StdEncoding.EncodeToString(signature)
	return payloadB64 + "." + signatureB64, nil
}

// ParseAndVerify parses the token (separator ".", base64 payload and signature), verifies HMAC-SHA256 with secret and decodes payload into TokenClaims. Expected format is "base64(payload).base64(signature)" as in CreateToken.
//
// Parameters: token — raw string from authorization metadata; secret — shared HMAC key (must match MyAuth). Empty token or not exactly two parts yields ErrInvalidTokenFormat; invalid signature — ErrInvalidSignature; unmarshal error — wrapped error.
//
// Returns: (TokenClaims, nil) on success; (zero TokenClaims, error) on invalid format (ErrInvalidTokenFormat), invalid signature (ErrInvalidSignature) or decode/unmarshal error.
//
// Called from service.jwtValidator.ValidateToken.
func ParseAndVerify(token string, secret []byte) (TokenClaims, error) {
	var zero TokenClaims
	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 2 {
		return zero, ErrInvalidTokenFormat
	}
	payloadB64, signatureB64 := parts[0], parts[1]

	payloadBytes, err := base64.StdEncoding.DecodeString(payloadB64)
	if err != nil {
		return zero, fmt.Errorf("decode payload: %w", err)
	}

	receivedSig, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return zero, fmt.Errorf("decode signature: %w", err)
	}

	mac := hmac.New(sha256.New, secret)
	mac.Write(payloadBytes)
	expectedSig := mac.Sum(nil)
	if subtle.ConstantTimeCompare(receivedSig, expectedSig) != 1 {
		return zero, ErrInvalidSignature
	}

	var claims TokenClaims
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return zero, fmt.Errorf("unmarshal claims: %w", err)
	}
	return claims, nil
}
