package config

import (
	"context"
	"errors"
	clientv3 "go.etcd.io/etcd/client/v3"
	"time"
)

type Etcd struct {
	client  *clientv3.Client
	key     string
	timeout time.Duration
}

// Provider returns a provider that takes etcd config.
func NewEtcdProvider(client *clientv3.Client, key string, timeout time.Duration) *Etcd {
	return &Etcd{client: client, key: key, timeout: timeout}
}

// ReadBytes is not supported by etcd provider.
func (e *Etcd) ReadBytes() ([]byte, error) {
	return nil, errors.New("etcd provider does not support this method")
}

// Read returns a nested config map.
func (e *Etcd) Read() (map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	var resp *clientv3.GetResponse
	r, err := e.client.Get(ctx, e.key, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}

	resp = r

	mp := make(map[string]interface{}, len(resp.Kvs))
	for _, r := range resp.Kvs {
		mp[string(r.Key)] = string(r.Value)
	}

	return mp, nil
}

func (e *Etcd) Watch(cb func(event interface{}, err error)) error {
	var w clientv3.WatchChan

	go func() {
		w = e.client.Watch(context.Background(), e.key)

		for wresp := range w {
			for _, ev := range wresp.Events {
				cb(ev, nil)
			}
		}
	}()

	return nil
}
