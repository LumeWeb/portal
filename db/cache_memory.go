// Package db provides database functionality for the portal application.
package db

import (
	"context"
	"sync"

	"github.com/go-gorm/caches/v4"
)

// memoryCacher implements the caches.Cacher interface using an in-memory store.
// It provides a thread-safe caching mechanism using sync.Map.
type memoryCacher struct {
	store *sync.Map
	mu    sync.Mutex
}

// init initializes the memory store if it hasn't been initialized yet.
// It uses a mutex to ensure thread safety during initialization.
func (c *memoryCacher) init() {
	if c.store == nil {
		c.store = &sync.Map{}
	}
}

// Get retrieves a cached query by key.
// It returns nil if the key doesn't exist or if there's an error unmarshaling the data.
func (c *memoryCacher) Get(ctx context.Context, key string, q *caches.Query[any]) (*caches.Query[any], error) {
	c.init()
	val, ok := c.store.Load(key)
	if !ok {
		return nil, nil
	}

	if err := q.Unmarshal(val.([]byte)); err != nil {
		return nil, err
	}

	return q, nil
}

// Store stores a query in the cache with the given key.
// It marshals the query data before storing it.
func (c *memoryCacher) Store(ctx context.Context, key string, val *caches.Query[any]) error {
	c.init()
	res, err := val.Marshal()
	if err != nil {
		return err
	}

	c.store.Store(key, res)
	return nil
}

// Invalidate clears all entries from the cache by reinitializing the store.
func (c *memoryCacher) Invalidate(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store = &sync.Map{}
	return nil
}
