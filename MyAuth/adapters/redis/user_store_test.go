package redis

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"myauth/domain"
	"myauth/service"

	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testRedisAddr = "redis://localhost:6379"

func setupTestRedis(t *testing.T) (redis.UniversalClient, func()) {
	client, err := NewRedisUniversalClient(testRedisAddr)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	keys, err := client.Keys(ctx, "external_user:*").Result()
	if err == nil && len(keys) > 0 {
		client.Del(ctx, keys...)
	}

	cleanup := func() {
		keys, _ := client.Keys(ctx, "external_user:*").Result()
		if len(keys) > 0 {
			client.Del(ctx, keys...)
		}
		client.Close()
	}
	return client, cleanup
}

func TestUserStore_GetByLogin(t *testing.T) {
	ctx := context.Background()
	client, cleanup := setupTestRedis(t)
	defer cleanup()

	store := NewUserStore(client)

	t.Run("success", func(t *testing.T) {
		login := "user1"
		user := domain.User{Login: login, Password: "pass1", Role: "admin"}
		data, err := json.Marshal(user)
		require.NoError(t, err)
		err = client.Set(ctx, "external_user:"+login, data, 0).Err()
		require.NoError(t, err)

		got, err := store.GetByLogin(ctx, login)
		require.NoError(t, err)
		assert.Equal(t, user.Login, got.Login)
		assert.Equal(t, user.Password, got.Password)
		assert.Equal(t, user.Role, got.Role)
	})

	t.Run("key missing returns entity not found", func(t *testing.T) {
		_, err := store.GetByLogin(ctx, "nonexistent")
		require.Error(t, err)
		assert.True(t, service.IsEntityNotFound(err))
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		login := "badjson"
		err := client.Set(ctx, "external_user:"+login, "invalid json", 0).Err()
		require.NoError(t, err)

		_, err = store.GetByLogin(ctx, login)
		require.Error(t, err)
		assert.False(t, service.IsEntityNotFound(err))
	})
}
