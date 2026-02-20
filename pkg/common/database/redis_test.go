// pkg/common/database/redis_test.go
package database

import (
	"context"
	"testing"
	"time"

	"github.com/Medex81/ProjectorBackend/pkg/common/config"
	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRedisConnection(t *testing.T) {
	// Start mini redis for testing
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	cfg := &config.RedisConfig{
		Addr:     mr.Addr(),
		Password: "",
		DB:       0,
	}

	t.Run("Successful connection", func(t *testing.T) {
		client, err := NewRedisConnection(cfg)
		require.NoError(t, err)
		defer client.Close()

		// Test connection
		pong, err := client.Ping(ctx).Result()
		assert.NoError(t, err)
		assert.Equal(t, "PONG", pong)
	})

	t.Run("Wrong password", func(t *testing.T) {
		wrongCfg := &config.RedisConfig{
			Addr:     mr.Addr(),
			Password: "wrong",
			DB:       0,
		}

		client, err := NewRedisConnection(wrongCfg)
		if err == nil {
			defer client.Close()
		}
		assert.Error(t, err)
	})

	t.Run("Wrong address", func(t *testing.T) {
		wrongCfg := &config.RedisConfig{
			Addr: "localhost:9999",
		}

		client, err := NewRedisConnection(wrongCfg)
		if err == nil {
			defer client.Close()
		}
		assert.Error(t, err)
	})
}

func TestRedisOperations(t *testing.T) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	defer mr.Close()

	cfg := &config.RedisConfig{
		Addr: mr.Addr(),
	}

	client, err := NewRedisConnection(cfg)
	require.NoError(t, err)
	defer client.Close()

	ctx := context.Background()

	t.Run("Set and Get", func(t *testing.T) {
		err := client.Set(ctx, "key", "value", 0).Err()
		require.NoError(t, err)

		val, err := client.Get(ctx, "key").Result()
		assert.NoError(t, err)
		assert.Equal(t, "value", val)
	})

	t.Run("Get non-existent key", func(t *testing.T) {
		val, err := client.Get(ctx, "nonexistent").Result()
		assert.Error(t, err)
		assert.Equal(t, redis.Nil, err)
		assert.Empty(t, val)
	})

	t.Run("Delete key", func(t *testing.T) {
		client.Set(ctx, "todelete", "value", 0)

		deleted, err := client.Del(ctx, "todelete").Result()
		assert.NoError(t, err)
		assert.Equal(t, int64(1), deleted)

		exists, err := client.Exists(ctx, "todelete").Result()
		assert.NoError(t, err)
		assert.Equal(t, int64(0), exists)
	})

	t.Run("Expiration", func(t *testing.T) {
		err := client.Set(ctx, "expiring", "value", 100*time.Millisecond).Err()
		require.NoError(t, err)

		ttl, err := client.TTL(ctx, "expiring").Result()
		assert.NoError(t, err)
		assert.True(t, ttl > 0)

		time.Sleep(150 * time.Millisecond)

		exists, err := client.Exists(ctx, "expiring").Result()
		assert.NoError(t, err)
		assert.Equal(t, int64(0), exists)
	})
}
