package config

import (
	"errors"
	"strings"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

var _ Defaults = (*EtcdConfig)(nil)

type EtcdConfig struct {
	Endpoints   []string `config:"endpoints"`
	Username    string   `config:"username"`
	Password    string   `config:"password"`
	Prefix      string   `config:"prefix"`
	DialTimeout int      `config:"dial_timeout"`
	manager     *EtcdManager
}

func (r *EtcdConfig) Validate() error {
	if len(r.Endpoints) == 0 {
		return errors.New("endpoints is required")
	}

	if r.DialTimeout <= 0 {
		return errors.New("dial_timeout must be greater than 0")
	}

	if r.Prefix == "" {
		return errors.New("prefix is required")
	}

	if r.Username != "" && r.Password == "" {
		return errors.New("password is required if username is set")
	}

	if r.Username == "" && r.Password != "" {
		return errors.New("username is required if password is set")
	}

	if r.Username != "" && r.Password != "" {
		return errors.New("username and password are required")
	}

	return nil
}

func (r *EtcdConfig) Defaults() map[string]interface{} {
	return map[string]interface{}{
		"dial_timeout": 5,
	}
}

func (r *EtcdConfig) Client() (*clientv3.Client, error) {
    if r.manager == nil {
        return nil, errors.New("etcd manager not initialized")
    }
    return r.manager.Client(), nil
}

func (r *EtcdConfig) InitManager(logger *zap.Logger) error {
    if r.manager != nil {
        return nil
    }
    
    manager, err := NewEtcdManager(r, logger)
    if err != nil {
        return err
    }
    
    r.manager = manager
    return nil
}

func (r *EtcdConfig) Close() error {
    if r.manager != nil {
        return r.manager.Close()
    }
    return nil
}

func (r *EtcdConfig) ComputePrefix(key string) string {
	return r.Prefix + "/" + strings.TrimSuffix(strings.TrimPrefix(key, "/"), "/")
}
