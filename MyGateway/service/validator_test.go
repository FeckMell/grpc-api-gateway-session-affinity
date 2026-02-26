package service

import (
	"testing"
	"time"

	"mygateway/auth"
	"mygateway/interfaces/mock"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testNow() time.Time {
	return time.Date(2026, 2, 11, 12, 0, 0, 0, time.UTC)
}

func TestNewJWTValidator_Panics(t *testing.T) {
	now := testNow()
	tp := &mock.TimeProviderMock{NowFunc: func() time.Time { return now }}

	t.Run("secret_nil", func(t *testing.T) {
		assert.PanicsWithValue(t, "service.validator.go: secret is required", func() {
			NewJWTValidator(nil, tp)
		})
	})
	t.Run("time_provider_nil", func(t *testing.T) {
		assert.PanicsWithValue(t, "service.validator.go: time provider is required", func() {
			NewJWTValidator([]byte("x"), nil)
		})
	})
}

func TestJWTValidator_ValidToken(t *testing.T) {
	now := testNow()
	expiresAt := now.Add(time.Hour)
	secret := []byte("secret")
	token, err := auth.CreateToken("u", "admin", "sess1", expiresAt, now, secret)
	require.NoError(t, err)

	tp := &mock.TimeProviderMock{NowFunc: func() time.Time { return now }}
	v := NewJWTValidator(secret, tp)
	ok, err := v.ValidateToken("sess1", token)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestJWTValidator_ExpiredToken(t *testing.T) {
	now := testNow()
	expiresAt := now.Add(-time.Hour) // already expired
	secret := []byte("secret")
	token, err := auth.CreateToken("u", "admin", "sess1", expiresAt, now, secret)
	require.NoError(t, err)

	tp := &mock.TimeProviderMock{NowFunc: func() time.Time { return now }}
	v := NewJWTValidator(secret, tp)
	ok, err := v.ValidateToken("sess1", token)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestJWTValidator_WrongSessionID(t *testing.T) {
	now := testNow()
	expiresAt := now.Add(time.Hour)
	secret := []byte("secret")
	token, err := auth.CreateToken("u", "admin", "sess1", expiresAt, now, secret)
	require.NoError(t, err)

	tp := &mock.TimeProviderMock{NowFunc: func() time.Time { return now }}
	v := NewJWTValidator(secret, tp)
	ok, err := v.ValidateToken("other-session", token)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestJWTValidator_InvalidSignature(t *testing.T) {
	now := testNow()
	expiresAt := now.Add(time.Hour)
	token, err := auth.CreateToken("u", "r", "sid", expiresAt, now, []byte("secret"))
	require.NoError(t, err)

	tp := &mock.TimeProviderMock{NowFunc: func() time.Time { return now }}
	v := NewJWTValidator([]byte("wrong-secret"), tp)
	ok, err := v.ValidateToken("sid", token)
	require.NoError(t, err)
	assert.False(t, ok)
}
