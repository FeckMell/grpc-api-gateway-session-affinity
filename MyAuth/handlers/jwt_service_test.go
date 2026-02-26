package handlers

import (
	"testing"
	"time"

	"myauth/service"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJwtService_CreateToken(t *testing.T) {
	secret := []byte("test-secret")
	svc := NewJwtService(secret)
	now := service.TestNow()
	expiresAt := now.Add(time.Hour)

	token, err := svc.CreateToken("user1", "admin", "sess-1", expiresAt, now)
	require.NoError(t, err)
	require.NotEmpty(t, token)
	assert.Contains(t, token, ".")
}

func TestJwtService_CreateToken_RoundTrip(t *testing.T) {
	secret := []byte("roundtrip-secret")
	svc := NewJwtService(secret)
	now := service.TestNow()
	expiresAt := now.Add(2 * time.Hour)

	token, err := svc.CreateToken("login1", "role1", "session-123", expiresAt, now)
	require.NoError(t, err)

	claims, err := service.ParseAndVerify(token, secret)
	require.NoError(t, err)
	assert.Equal(t, "login1", claims.Login)
	assert.Equal(t, "role1", claims.Role)
	assert.Equal(t, "session-123", claims.SessionID)
	assert.Equal(t, expiresAt.UTC().Format(time.RFC3339), claims.ExpiresAt)
	assert.Equal(t, now.UTC().Format(time.RFC3339), claims.IssuedAt)
}
