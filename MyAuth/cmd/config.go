package main

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Port             int
	RedisAddr        string
	JWTSecret        []byte
	TokenExpiration  time.Duration
}

func LoadConfig() (*Config, error) {
	config := &Config{
		Port:            5001,
		RedisAddr:       "redis://localhost:6379",
		TokenExpiration: time.Hour,
	}

	if v := os.Getenv("SERVICE_PORT_GRPC"); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid SERVICE_PORT_GRPC: %w", err)
		}
		config.Port = port
	}

	if v := os.Getenv("REDIS_ADDR"); v != "" {
		config.RedisAddr = v
	}

	if v := os.Getenv("JWT_SECRET"); v != "" {
		config.JWTSecret = []byte(v)
	}

	if v := os.Getenv("TOKEN_EXPIRATION"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("invalid TOKEN_EXPIRATION: %w", err)
		}
		config.TokenExpiration = d
	}

	if len(config.JWTSecret) == 0 {
		return nil, fmt.Errorf("JWT_SECRET is required and must not be empty")
	}

	return config, nil
}
