package myredis

import (
	"context"
	"fmt"
	"strings"
	"time"

	"mydiscoverer/service"

	"github.com/go-redis/redis/v8"
)

type redisCache[T any] struct {
	client    redis.UniversalClient
	prefix    string
	marshal   func(T) ([]byte, error)
	unmarshal func([]byte) (T, error)
	zero      T
}

// NewCache creates redis implementation of generic cache interface.
func NewCache[T any](client redis.UniversalClient, prefix string, marshal func(T) ([]byte, error), unmarshal func([]byte) (T, error)) *redisCache[T] {
	var zero T
	return &redisCache[T]{
		client:    client,
		prefix:    prefix,
		zero:      zero,
		marshal:   marshal,
		unmarshal: unmarshal,
	}
}

func (r *redisCache[T]) WriteValue(ctx context.Context, key string, item T, ttlMs int) error {
	bytes, err := r.marshal(item)
	if err != nil {
		return service.NewInternalServerError("Redis marshal item error", fmt.Errorf("can't marshal item of type %T, err: %w", item, err))
	}

	err = r.client.Set(ctx, r.generateKey(key), bytes, time.Duration(ttlMs)*time.Millisecond).Err()
	if err != nil {
		return service.NewInternalServerError("Redis write key error", fmt.Errorf("can't write item of type %T to redis (key='%s'), err: %w", item, key, err))
	}

	return nil
}

func (r *redisCache[T]) DeleteValue(ctx context.Context, key string) error {
	err := r.client.Del(ctx, r.generateKey(key)).Err()
	if err != nil {
		return service.NewInternalServerError("Redis delete key error", fmt.Errorf("can't delete item of type %T from redis (key='%s'), err: %w", r.zero, key, err))
	}
	return nil
}

// ListAllValues lists all keys under the cache prefix then fetches their values.
func (r *redisCache[T]) ListAllValues(ctx context.Context) ([]T, error) {
	fullKeys, err := r.client.Keys(ctx, r.prefix+":*").Result()
	if err != nil {
		return nil, service.NewInternalServerError("Redis get keys error", fmt.Errorf("redis get keys error, err: %w", err))
	}

	if len(fullKeys) == 0 {
		return nil, service.NewEntityNotFoundError("Entity not found", nil)
	}

	prefixWithColon := r.prefix + ":"
	keys := make([]string, 0, len(fullKeys))
	for _, k := range fullKeys {
		if strings.HasPrefix(k, prefixWithColon) {
			keys = append(keys, strings.TrimPrefix(k, prefixWithColon))
		}
	}

	items := make([]T, 0, len(keys))
	for _, key := range keys {
		bytes, err := r.client.Get(ctx, r.generateKey(key)).Bytes()
		if err != nil {
			continue
		}

		item, err := r.unmarshal(bytes)
		if err != nil {
			continue
		}

		items = append(items, item)
	}
	if len(items) == 0 {
		return nil, service.NewEntityNotFoundError("Entity not found", nil)
	}

	return items, nil
}

func (r *redisCache[T]) generateKey(key string) string {
	return r.prefix + ":" + key
}
