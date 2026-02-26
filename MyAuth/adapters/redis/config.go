package redis

import (
	"fmt"

	"github.com/go-redis/redis/v8"
)

type RedisConfig struct {
	Addr string
}

// NewRedisUniversalClient creates and configures instance of redis universal client.
func NewRedisUniversalClient(redisAddr string, options ...ConfigOption) (redis.UniversalClient, error) {
	redisOptions, err := redis.ParseURL(redisAddr)
	if err != nil {
		return nil, fmt.Errorf("cant parse redis url: %w", err)
	}
	for _, opt := range options {
		opt(redisOptions)
	}
	c := redis.NewUniversalClient(universalOptions(redisOptions))
	return c, nil
}

// ConfigOption configures the client.
type ConfigOption func(*redis.Options)

func universalOptions(options *redis.Options) *redis.UniversalOptions {
	return &redis.UniversalOptions{
		Addrs:              []string{options.Addr},
		DB:                 options.DB,
		Username:           options.Username,
		Password:           options.Password,
		ReadOnly:           false,
		MasterName:         "",
		WriteTimeout:       options.WriteTimeout,
		ReadTimeout:        options.ReadTimeout,
		DialTimeout:        options.DialTimeout,
		MaxRetries:         options.MaxRetries,
		PoolSize:           options.PoolSize,
		PoolTimeout:        options.PoolTimeout,
		MinIdleConns:       options.MinIdleConns,
		IdleTimeout:        options.IdleTimeout,
		IdleCheckFrequency: options.IdleCheckFrequency,
	}
}
