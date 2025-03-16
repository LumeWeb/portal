// Package db provides database functionality for the portal application.
package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-gorm/caches/v4"
	"github.com/redis/go-redis/v9"
)

// redisCacher implements the caches.Cacher interface using Redis.
// It provides a distributed caching mechanism for database queries.
type redisCacher struct {
	rdb *redis.Client
}

// Get retrieves a cached query by key from Redis.
// It returns nil if the key doesn't exist or if there's an error unmarshaling the data.
func (c *redisCacher) Get(ctx context.Context, key string, q *caches.Query[any]) (*caches.Query[any], error) {
	res, err := c.rdb.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	if err := q.Unmarshal([]byte(res)); err != nil {
		return nil, err
	}

	return q, nil
}

// Store stores a query in Redis with the given key.
// It marshals the query data before storing it with a 5-minute expiration.
func (c *redisCacher) Store(ctx context.Context, key string, val *caches.Query[any]) error {
	res, err := val.Marshal()
	if err != nil {
		return err
	}

	c.rdb.Set(ctx, key, res, 300*time.Second) // Set proper cache time
	return nil
}

// Invalidate clears all cache keys from Redis that match the cache identifier prefix.
// It scans for matching keys and deletes them in batches.
func (c *redisCacher) Invalidate(ctx context.Context) error {
	var (
		cursor uint64
		keys   []string
	)
	for {
		var (
			k   []string
			err error
		)
		k, cursor, err = c.rdb.Scan(ctx, cursor, fmt.Sprintf("%s*", caches.IdentifierPrefix), 0).Result()
		if err != nil {
			return err
		}
		keys = append(keys, k...)
		if cursor == 0 {
			break
		}
	}

	if len(keys) > 0 {
		if _, err := c.rdb.Del(ctx, keys...).Result(); err != nil {
			return err
		}
	}
	return nil
}
