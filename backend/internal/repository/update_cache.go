package repository

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

const updateCacheKey = "update:latest"

type updateCache struct {
	rdb *redis.Client
}

func NewUpdateCache(rdb *redis.Client) service.UpdateCache {
	return &updateCache{rdb: rdb}
}

func namespacedUpdateCacheKey(namespace []string) string {
	if len(namespace) == 0 || namespace[0] == "" {
		return updateCacheKey
	}
	sum := sha256.Sum256([]byte(namespace[0]))
	return updateCacheKey + ":" + hex.EncodeToString(sum[:8])
}

func (c *updateCache) GetUpdateInfo(ctx context.Context, namespace ...string) (string, error) {
	return c.rdb.Get(ctx, namespacedUpdateCacheKey(namespace)).Result()
}

func (c *updateCache) SetUpdateInfo(ctx context.Context, data string, ttl time.Duration, namespace ...string) error {
	return c.rdb.Set(ctx, namespacedUpdateCacheKey(namespace), data, ttl).Err()
}
