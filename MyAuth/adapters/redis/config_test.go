package redis

import (
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRedisUniversalClient(t *testing.T) {
	t.Run("valid URL returns client", func(t *testing.T) {
		client, err := NewRedisUniversalClient("redis://localhost:6379")
		require.NoError(t, err)
		require.NotNil(t, client)
		defer client.Close()
	})

	t.Run("invalid URL returns error", func(t *testing.T) {
		client, err := NewRedisUniversalClient("://invalid")
		require.Error(t, err)
		assert.Nil(t, client)
	})

	t.Run("with options returns client", func(t *testing.T) {
		client, err := NewRedisUniversalClient("redis://localhost:6379", func(o *redis.Options) {
			o.DialTimeout = time.Second
		})
		require.NoError(t, err)
		require.NotNil(t, client)
		defer client.Close()
	})
}
