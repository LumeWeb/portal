package config

import (
	"errors"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

var _ Defaults = (*EtcdConfig)(nil)

type EtcdConfig struct {
	Endpoints   []string `mapstructure:"endpoints"`
	DialTimeout int      `mapstructure:"dial_timeout"`
	client      *clientv3.Client
}

func (r *EtcdConfig) Validate() error {
	if len(r.Endpoints) == 0 {
		return errors.New("endpoints is required")
	}
	return nil
}

func (r *EtcdConfig) Defaults() map[string]interface{} {
	return map[string]interface{}{
		"dial_timeout": 5,
	}
}

func (r *EtcdConfig) Client() (*clientv3.Client, error) {
	if r.client == nil {
		client, err := clientv3.New(clientv3.Config{
			Endpoints:   r.Endpoints,
			DialTimeout: time.Duration(r.DialTimeout) * time.Second,
		})
		if err != nil {
			return nil, err
		}

		r.client = client
	}

	return r.client, nil
}
