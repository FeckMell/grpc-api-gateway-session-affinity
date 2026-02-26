package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_RedisAddrRequired(t *testing.T) {
	t.Setenv("REDIS_ADDR", "")
	t.Setenv("SERVICE_PORT_HTTP", "8080")

	cfg, err := LoadConfig()
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "REDIS_ADDR is required")
}

func TestLoadConfig_ServicePortRequired(t *testing.T) {
	t.Setenv("REDIS_ADDR", "redis://localhost:6379")
	t.Setenv("SERVICE_PORT_HTTP", "")

	cfg, err := LoadConfig()
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "SERVICE_PORT_HTTP is required")
}

func TestLoadConfig_Ok(t *testing.T) {
	t.Setenv("REDIS_ADDR", "redis://localhost:6379")
	t.Setenv("SERVICE_PORT_HTTP", "8080")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "redis://localhost:6379", cfg.Redis.Addr)
	assert.Equal(t, 8080, cfg.HTTPPort)
}

func TestLoadConfig_CustomPort(t *testing.T) {
	t.Setenv("REDIS_ADDR", "redis://localhost:6379")
	t.Setenv("SERVICE_PORT_HTTP", "9000")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, 9000, cfg.HTTPPort)
	assert.Equal(t, "redis://localhost:6379", cfg.Redis.Addr)
}

func TestLoadConfig_CustomRedisAddr(t *testing.T) {
	t.Setenv("REDIS_ADDR", "redis://other:6380")
	t.Setenv("SERVICE_PORT_HTTP", "8080")

	cfg, err := LoadConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "redis://other:6380", cfg.Redis.Addr)
	assert.Equal(t, 8080, cfg.HTTPPort)
}

func TestLoadConfig_InvalidSERVICE_PORT_HTTP(t *testing.T) {
	t.Setenv("REDIS_ADDR", "redis://localhost:6379")
	t.Setenv("SERVICE_PORT_HTTP", "not-a-number")

	cfg, err := LoadConfig()
	require.Error(t, err)
	assert.Nil(t, cfg)
	assert.Contains(t, err.Error(), "SERVICE_PORT_HTTP")
}
