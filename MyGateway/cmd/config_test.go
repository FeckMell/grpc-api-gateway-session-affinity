package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"mygateway/domain"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig_YAML(t *testing.T) {
	t.Setenv(envGRPCPort, "50051")
	t.Setenv(envJWTSecret, "secret")
	t.Setenv(envRetryCount, "3")
	t.Setenv(envRetryTimeoutMs, "5000")
	cfgPath := filepath.Join(t.TempDir(), "gateway.yaml")
	content := `
default:
  action: use_cluster
  use_cluster: my_auth
routes:
  - prefix: myservice/login*
    cluster: my_auth
    authorization: none
    balancer:
      type: round_robin
  - prefix: myservice/myservice*
    cluster: my_service
    authorization: required
    balancer:
      type: sticky_sessions
      header: session-id
clusters:
  my_auth:
    type: static
    address: myauth:50051
  my_service:
    type: dynamic
    discoverer_url: http://discoverer:8080
    discoverer_interval_ms: 5000
`
	err := os.WriteFile(cfgPath, []byte(content), 0o644)
	require.NoError(t, err)
	t.Setenv(envConfigPath, cfgPath)

	cfg, err := LoadConfig()
	require.NoError(t, err)
	require.Len(t, cfg.Routes.Routes, 2)
	assert.Equal(t, "/myservice/login", cfg.Routes.Routes[0].Prefix)
	assert.Equal(t, domain.BalancerRoundRobin, cfg.Routes.Routes[0].Balancer.Type)
	assert.Equal(t, domain.AuthorizationRequired, cfg.Routes.Routes[1].Authorization)
	assert.Equal(t, domain.BalancerStickySession, cfg.Routes.Routes[1].Balancer.Type)
	assert.Equal(t, domain.DefaultRouteUseCluster, cfg.Routes.Default.Action)
	assert.Equal(t, domain.ClusterID("my_auth"), cfg.Routes.Default.Cluster)
	assert.Equal(t, domain.ClusterID("my_auth"), cfg.Routes.Routes[0].Cluster)
	assert.Equal(t, domain.ClusterTypeStatic, cfg.Clusters[domain.ClusterID("my_auth")].Type)
	assert.Equal(t, domain.ClusterTypeDynamic, cfg.Clusters[domain.ClusterID("my_service")].Type)
	assert.Equal(t, 3, cfg.RetryCount)
	assert.Equal(t, 5*time.Second, cfg.RetryTimeout)
}

func TestLoadConfig_RetryMissing(t *testing.T) {
	t.Setenv(envGRPCPort, "50051")
	t.Setenv(envJWTSecret, "secret")
	cfgPath := filepath.Join(t.TempDir(), "gateway.yaml")
	content := `
default:
  action: error
routes:
  - prefix: /svc/*
    cluster: c1
    authorization: none
    balancer:
      type: round_robin
clusters:
  c1:
    type: static
    address: localhost:50052
`
	require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0o644))
	t.Setenv(envConfigPath, cfgPath)

	t.Run("retry_count_missing", func(t *testing.T) {
		t.Setenv(envRetryCount, "")
		t.Setenv(envRetryTimeoutMs, "5000")
		_, err := LoadConfig()
		require.Error(t, err)
		assert.Contains(t, err.Error(), envRetryCount)
		assert.Contains(t, err.Error(), "required")
	})
	t.Run("retry_timeout_missing", func(t *testing.T) {
		t.Setenv(envRetryCount, "3")
		t.Setenv(envRetryTimeoutMs, "")
		_, err := LoadConfig()
		require.Error(t, err)
		assert.Contains(t, err.Error(), envRetryTimeoutMs)
		assert.Contains(t, err.Error(), "required")
	})
}

func TestLoadConfig_RetryFromEnv(t *testing.T) {
	t.Setenv(envGRPCPort, "50051")
	t.Setenv(envJWTSecret, "secret")
	t.Setenv(envRetryCount, "5")
	t.Setenv(envRetryTimeoutMs, "2000")
	cfgPath := filepath.Join(t.TempDir(), "gateway.yaml")
	content := `
default:
  action: error
routes:
  - prefix: /svc/*
    cluster: c1
    authorization: none
    balancer:
      type: round_robin
clusters:
  c1:
    type: static
    address: localhost:50052
`
	require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0o644))
	t.Setenv(envConfigPath, cfgPath)

	cfg, err := LoadConfig()
	require.NoError(t, err)
	assert.Equal(t, 5, cfg.RetryCount)
	assert.Equal(t, 2*time.Second, cfg.RetryTimeout)
}

func TestLoadConfig_RetryInvalid(t *testing.T) {
	t.Setenv(envGRPCPort, "50051")
	t.Setenv(envJWTSecret, "secret")
	t.Setenv(envRetryCount, "3")
	t.Setenv(envRetryTimeoutMs, "5000")
	cfgPath := filepath.Join(t.TempDir(), "gateway.yaml")
	content := `
default:
  action: error
routes:
  - prefix: /svc/*
    cluster: c1
    authorization: none
    balancer:
      type: round_robin
clusters:
  c1:
    type: static
    address: localhost:50052
`
	require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0o644))
	t.Setenv(envConfigPath, cfgPath)

	t.Run("retry_count_zero", func(t *testing.T) {
		t.Setenv(envRetryCount, "0")
		t.Setenv(envRetryTimeoutMs, "5000")
		_, err := LoadConfig()
		require.Error(t, err)
		assert.Contains(t, err.Error(), envRetryCount)
	})
	t.Run("retry_timeout_zero", func(t *testing.T) {
		t.Setenv(envRetryCount, "3")
		t.Setenv(envRetryTimeoutMs, "0")
		_, err := LoadConfig()
		require.Error(t, err)
		assert.Contains(t, err.Error(), envRetryTimeoutMs)
	})
}

func TestLoadConfig_MissingConfigPath(t *testing.T) {
	t.Setenv(envGRPCPort, "50051")
	t.Setenv(envConfigPath, "")
	_, err := LoadConfig()
	require.Error(t, err)
	assert.Contains(t, err.Error(), envConfigPath)
}

func TestNormalizePrefix(t *testing.T) {
	assert.Equal(t, "/myservice/login", normalizePrefix("myservice/login*"))
	assert.Equal(t, "/svc/method", normalizePrefix("/svc/method"))
}
