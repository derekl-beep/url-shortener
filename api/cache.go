package main

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultTTL = 24 * time.Hour

type Cache struct {
	client *redis.Client
}

func NewCache(client *redis.Client) *Cache {
	return &Cache{client: client}
}

func (c *Cache) Get(ctx context.Context, key string) (string, error) {
	val, err := c.client.Get(ctx, "url:"+key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

func (c *Cache) Set(ctx context.Context, key, originalURL string) error {
	// URLs with expires_at will be evicted from DB correctly; Redis may serve
	// a stale entry for up to defaultTTL after expiry. Acceptable for this use case.
	return c.client.Set(ctx, "url:"+key, originalURL, defaultTTL).Err()
}
