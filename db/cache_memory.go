package db

import (
	"context"
	"sync"

	"github.com/go-gorm/caches/v4"
)

type memoryCacher struct {
	store *sync.Map
	mu    sync.Mutex
}

func (c *memoryCacher) init() {
	if c.store == nil {
		c.store = &sync.Map{}
	}
}

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

func (c *memoryCacher) Store(ctx context.Context, key string, val *caches.Query[any]) error {
	c.init()
	res, err := val.Marshal()
	if err != nil {
		return err
	}

	c.store.Store(key, res)
	return nil
}

func (c *memoryCacher) Invalidate(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store = &sync.Map{}
	return nil
}
