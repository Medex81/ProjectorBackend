// pkg/common/database/redis.go
package database

import (
	"context"

	"github.com/Medex81/ProjectorBackend/pkg/common/config"
	"github.com/Medex81/ProjectorBackend/pkg/common/logger"
	"github.com/go-redis/redis/v8"
)

func NewRedisConnection(cfg *config.RedisConfig) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	logger.Info().Str("addr", cfg.Addr).Msg("Connected to Redis")
	return client, nil
}
