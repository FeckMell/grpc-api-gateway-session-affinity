package auth

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testNow() time.Time {
	return time.Date(2026, 2, 11, 12, 0, 0, 0, time.UTC)
}

func TestCreateToken(t *testing.T) {
	now := testNow()
	expiresAt := now.Add(time.Hour)
	secret := []byte("secret")

	token, err := CreateToken("u1", "admin", "sess1", expiresAt, now, secret)
	require.NoError(t, err)
	require.NotEmpty(t, token)
	parts := strings.Split(token, ".")
	require.Len(t, parts, 2)
	_, err = base64.StdEncoding.DecodeString(parts[0])
	require.NoError(t, err)
	_, err = base64.StdEncoding.DecodeString(parts[1])
	require.NoError(t, err)
}

func TestParseAndVerify_RoundTrip(t *testing.T) {
	now := testNow()
	expiresAt := now.Add(2 * time.Hour)
	secret := []byte("roundtrip")

	token, err := CreateToken("login1", "role1", "session-1", expiresAt, now, secret)
	require.NoError(t, err)

	claims, err := ParseAndVerify(token, secret)
	require.NoError(t, err)
	assert.Equal(t, "login1", claims.Login)
	assert.Equal(t, "role1", claims.Role)
	assert.Equal(t, "session-1", claims.SessionID)
	assert.Equal(t, expiresAt.UTC().Format(time.RFC3339), claims.ExpiresAt)
	assert.Equal(t, now.UTC().Format(time.RFC3339), claims.IssuedAt)
}

func TestParseAndVerify_InvalidFormat(t *testing.T) {
	secret := []byte("secret")

	t.Run("one part", func(t *testing.T) {
		_, err := ParseAndVerify("onlyonepart", secret)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidTokenFormat))
	})

	t.Run("three parts", func(t *testing.T) {
		_, err := ParseAndVerify("a.b.c", secret)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrInvalidTokenFormat))
	})
}

func TestParseAndVerify_InvalidSignature(t *testing.T) {
	now := testNow()
	expiresAt := now.Add(time.Hour)
	secret := []byte("secret")
	token, err := CreateToken("u", "r", "sid", expiresAt, now, secret)
	require.NoError(t, err)

	parts := strings.Split(token, ".")
	_, err = ParseAndVerify(parts[0]+"."+parts[1], []byte("wrong-secret"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidSignature))
}

func TestParseAndVerify_InvalidBase64(t *testing.T) {
	secret := []byte("secret")

	t.Run("invalid payload base64", func(t *testing.T) {
		_, err := ParseAndVerify("!!!.dGVzdA==", secret)
		require.Error(t, err)
		assert.False(t, errors.Is(err, ErrInvalidTokenFormat))
		assert.False(t, errors.Is(err, ErrInvalidSignature))
	})

	t.Run("invalid signature base64", func(t *testing.T) {
		payloadB64 := base64.StdEncoding.EncodeToString([]byte(`{"login":"x","role":"y","session_id":"s","expires_at":"","issued_at":""}`))
		_, err := ParseAndVerify(payloadB64+".!!!", secret)
		require.Error(t, err)
	})
}

func TestParseAndVerify_Valid(t *testing.T) {
	now := testNow()
	expiresAt := now.Add(time.Hour)
	secret := []byte("valid-secret")
	token, err := CreateToken("user", "role", "sess-valid", expiresAt, now, secret)
	require.NoError(t, err)

	claims, err := ParseAndVerify(token, secret)
	require.NoError(t, err)
	assert.Equal(t, "user", claims.Login)
	assert.Equal(t, "role", claims.Role)
	assert.Equal(t, "sess-valid", claims.SessionID)
}
