package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_Defaults(t *testing.T) {
	t.Setenv("SERVICE_PORT_GRPC", "")
	t.Setenv("REDIS_ADDR", "")
	t.Setenv("JWT_SECRET", "default-secret")
	t.Setenv("TOKEN_EXPIRATION", "")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, 5001, cfg.Port)
	assert.Equal(t, "redis://localhost:6379", cfg.RedisAddr)
	assert.Equal(t, time.Hour, cfg.TokenExpiration)
	assert.Equal(t, []byte("default-secret"), cfg.JWTSecret)
}

func TestLoadConfig_OverrideSERVICE_PORT_GRPC(t *testing.T) {
	t.Setenv("SERVICE_PORT_GRPC", "9000")
	t.Setenv("REDIS_ADDR", "")
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("TOKEN_EXPIRATION", "")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, 9000, cfg.Port)
}

func TestLoadConfig_OverrideREDIS_ADDR(t *testing.T) {
	t.Setenv("SERVICE_PORT_GRPC", "")
	t.Setenv("REDIS_ADDR", "redis://other:6380")
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("TOKEN_EXPIRATION", "")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, "redis://other:6380", cfg.RedisAddr)
}

func TestLoadConfig_OverrideJWT_SECRET(t *testing.T) {
	t.Setenv("SERVICE_PORT_GRPC", "")
	t.Setenv("REDIS_ADDR", "")
	t.Setenv("JWT_SECRET", "my-secret")
	t.Setenv("TOKEN_EXPIRATION", "")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, []byte("my-secret"), cfg.JWTSecret)
}

func TestLoadConfig_OverrideTOKEN_EXPIRATION(t *testing.T) {
	t.Setenv("SERVICE_PORT_GRPC", "")
	t.Setenv("REDIS_ADDR", "")
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("TOKEN_EXPIRATION", "30m")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, 30*time.Minute, cfg.TokenExpiration)
}

func TestLoadConfig_InvalidSERVICE_PORT_GRPC(t *testing.T) {
	t.Setenv("SERVICE_PORT_GRPC", "not-a-number")
	t.Setenv("REDIS_ADDR", "")
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("TOKEN_EXPIRATION", "")

	cfg, err := LoadConfig()
	require.Error(t, err)
	assert.Nil(t, cfg)
}

func TestLoadConfig_InvalidTOKEN_EXPIRATION(t *testing.T) {
	t.Setenv("SERVICE_PORT_GRPC", "")
	t.Setenv("REDIS_ADDR", "")
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("TOKEN_EXPIRATION", "invalid")

	cfg, err := LoadConfig()
	require.Error(t, err)
	assert.Nil(t, cfg)
}

func TestLoadConfig_EmptyJWT_SECRET(t *testing.T) {
	t.Setenv("SERVICE_PORT_GRPC", "")
	t.Setenv("REDIS_ADDR", "")
	t.Setenv("JWT_SECRET", "")
	t.Setenv("TOKEN_EXPIRATION", "")

	cfg, err := LoadConfig()
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "JWT_SECRET is required")
}
