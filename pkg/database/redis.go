package database

import (
	"context"
	"pai_smart_go_v2/pkg/log"

	"github.com/go-redis/redis/v8"
)

var RDB *redis.Client

func InitRedis(addr, password string, db int) {
	RDB = redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	ctx := context.Background()
	if err := RDB.Ping(ctx).Err(); err != nil {
		log.Fatal("failed to connect to redis", err)
	}

	log.Info("Redis client connected successfully")
}
