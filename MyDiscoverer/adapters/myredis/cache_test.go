package myredis

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"mydiscoverer/domain"
	"mydiscoverer/service"

	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testRedisAddr = "redis://localhost:6379"
const testPrefix = "instance"

func setupTestRedis(t *testing.T) (redis.UniversalClient, func()) {
	client, err := NewRedisUniversalClient(testRedisAddr)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	keys, err := client.Keys(ctx, testPrefix+":*").Result()
	if err == nil && len(keys) > 0 {
		client.Del(ctx, keys...)
	}

	cleanup := func() {
		keys, _ := client.Keys(ctx, testPrefix+":*").Result()
		if len(keys) > 0 {
			client.Del(ctx, keys...)
		}
		client.Close()
	}
	return client, cleanup
}

func marshalInstance(i domain.Instance) ([]byte, error) { return json.Marshal(i) }
func unmarshalInstance(b []byte) (domain.Instance, error) {
	var i domain.Instance
	err := json.Unmarshal(b, &i)
	return i, err
}

func TestCache_WriteValue(t *testing.T) {
	ctx := context.Background()
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	cache := NewCache[domain.Instance](client, testPrefix, marshalInstance, unmarshalInstance)
	inst := domain.Instance{
		InstanceID:  "inst-1",
		ServiceType: "grpc",
		Ipv4:        "127.0.0.1",
		Port:        9000,
		Timestamp:   time.Now(),
		TTLMs:       300000,
	}

	t.Run("success", func(t *testing.T) {
		err := cache.WriteValue(ctx, inst.InstanceID, inst, 60000)
		require.NoError(t, err)

		items, err := cache.ListAllValues(ctx)
		require.NoError(t, err)
		require.Len(t, items, 1)
		got := items[0]
		assert.Equal(t, inst.InstanceID, got.InstanceID)
		assert.Equal(t, inst.ServiceType, got.ServiceType)
		assert.Equal(t, inst.Ipv4, got.Ipv4)
		assert.Equal(t, inst.Port, got.Port)
		assert.Equal(t, inst.TTLMs, got.TTLMs)
	})

	t.Run("when Redis write fails returns internal_server_error", func(t *testing.T) {
		closedClient, err := NewRedisUniversalClient(testRedisAddr)
		require.NoError(t, err)
		closedClient.Close()
		cacheClosed := NewCache[domain.Instance](closedClient, testPrefix, marshalInstance, unmarshalInstance)

		err = cacheClosed.WriteValue(ctx, "x", inst, 60000)
		require.Error(t, err)
		assert.True(t, service.IsInternalServerError(err))
	})
}

func TestCache_DeleteValue(t *testing.T) {
	ctx := context.Background()
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	cache := NewCache[domain.Instance](client, testPrefix, marshalInstance, unmarshalInstance)
	inst := domain.Instance{InstanceID: "inst-del", ServiceType: "grpc", Ipv4: "127.0.0.1", Port: 9000, Timestamp: time.Now(), TTLMs: 300000}
	err := cache.WriteValue(ctx, inst.InstanceID, inst, 60000)
	require.NoError(t, err)

	err = cache.DeleteValue(ctx, inst.InstanceID)
	require.NoError(t, err)

	items, err := cache.ListAllValues(ctx)
	require.Error(t, err)
	assert.True(t, service.IsEntityNotFoundError(err))
	assert.Nil(t, items)
}

func TestCache_ListAllValues(t *testing.T) {
	ctx := context.Background()
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	cache := NewCache[domain.Instance](client, testPrefix, marshalInstance, unmarshalInstance)

	t.Run("empty cache returns entity not found", func(t *testing.T) {
		items, err := cache.ListAllValues(ctx)
		require.Error(t, err)
		assert.True(t, service.IsEntityNotFoundError(err))
		assert.Nil(t, items)
	})

	t.Run("returns all values", func(t *testing.T) {
		inst := domain.Instance{InstanceID: "list-1", ServiceType: "grpc", Ipv4: "127.0.0.1", Port: 9000, Timestamp: time.Now(), TTLMs: 300000}
		err := cache.WriteValue(ctx, inst.InstanceID, inst, 60000)
		require.NoError(t, err)

		items, err := cache.ListAllValues(ctx)
		require.NoError(t, err)
		require.Len(t, items, 1)
		assert.Equal(t, "list-1", items[0].InstanceID)
		assert.Equal(t, 9000, items[0].Port)
	})

	t.Run("invalid JSON in redis yields entity not found", func(t *testing.T) {
		keys, err := client.Keys(ctx, testPrefix+":*").Result()
		require.NoError(t, err)
		if len(keys) > 0 {
			client.Del(ctx, keys...)
		}
		err = client.Set(ctx, testPrefix+":badjson", "invalid json", 0).Err()
		require.NoError(t, err)

		items, err := cache.ListAllValues(ctx)
		require.Error(t, err)
		assert.True(t, service.IsEntityNotFoundError(err))
		assert.Nil(t, items)
	})
}
