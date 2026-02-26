package main

import (
	"fmt"
	"os"
	"strconv"

	"mydiscoverer/adapters/myredis"
)

type MyDiscovererConfig struct {
	Redis    myredis.RedisConfig
	HTTPPort int
}

// LoadConfig loads configuration from environment variables.
// REDIS_ADDR and SERVICE_PORT_HTTP are required.
func LoadConfig() (*MyDiscovererConfig, error) {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		return nil, fmt.Errorf("REDIS_ADDR is required")
	}

	httpPortStr := os.Getenv("SERVICE_PORT_HTTP")
	if httpPortStr == "" {
		return nil, fmt.Errorf("SERVICE_PORT_HTTP is required")
	}
	httpPort, err := strconv.Atoi(httpPortStr)
	if err != nil {
		return nil, fmt.Errorf("invalid SERVICE_PORT_HTTP: %w", err)
	}

	return &MyDiscovererConfig{
		Redis: myredis.RedisConfig{
			Addr: redisAddr,
		},
		HTTPPort: httpPort,
	}, nil
}
