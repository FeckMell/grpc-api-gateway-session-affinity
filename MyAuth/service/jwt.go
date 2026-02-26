package service

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

// TokenClaims is the JWT payload: login, role, session_id, valid_till (serialized in token).
type TokenClaims struct {
	Login     string `json:"login"`
	Role      string `json:"role"`
	SessionID string `json:"session_id"`
	ExpiresAt string `json:"expires_at"` // RFC3339
	IssuedAt  string `json:"issued_at"`  // RFC3339
}

// CreateToken builds a token: payload (login, role, session_id, valid_till) signed with HMAC-SHA256(secret, payload), both encoded in base64; returns "base64(payload).base64(signature)".
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

// ErrInvalidTokenFormat is returned when the token does not have exactly two parts separated by ".".
var ErrInvalidTokenFormat = errors.New("invalid token format: expected base64(payload).base64(signature)")

// ErrInvalidSignature is returned when the token signature does not match the expected HMAC-SHA256.
var ErrInvalidSignature = errors.New("invalid token signature")

// ParseAndVerify decodes the token, verifies the HMAC-SHA256 signature with secret, and returns TokenClaims.
// Token must be in the form "base64(payload).base64(signature)" as produced by CreateToken.
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
