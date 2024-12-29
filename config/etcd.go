package config

import (
	"errors"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
	"strings"
	"sync"
)

var _ Defaults = (*EtcdConfig)(nil)

type EtcdConfig struct {
	Endpoints   []string `config:"endpoints"`
	Username    string   `config:"username"`
	Password    string   `config:"password"`
	Prefix      string   `config:"prefix"`
	DialTimeout int      `config:"dial_timeout"`
	manager     *EtcdManager
	managerMu   sync.RWMutex
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

// GetManager returns the EtcdManager, initializing it if necessary
func (r *EtcdConfig) GetManager(logger *zap.Logger) (*EtcdManager, error) {
	r.managerMu.RLock()
	if r.manager != nil {
		defer r.managerMu.RUnlock()
		return r.manager, nil
	}
	r.managerMu.RUnlock()

	r.managerMu.Lock()
	defer r.managerMu.Unlock()

	// Double-check after acquiring write lock
	if r.manager != nil {
		return r.manager, nil
	}

	manager, err := NewEtcdManager(r, logger)
	if err != nil {
		return nil, err
	}

	r.manager = manager
	return r.manager, nil
}

func (r *EtcdConfig) Client() (*clientv3.Client, error) {
	// This method is deprecated - use GetManager().Client() instead
	return nil, errors.New("deprecated: use GetManager().Client() instead")
}

func (r *EtcdConfig) Close() error {
	r.managerMu.Lock()
	defer r.managerMu.Unlock()

	if r.manager != nil {
		return r.manager.Close()
	}
	return nil
}

func (r *EtcdConfig) ComputePrefix(key string) string {
	return r.Prefix + "/" + strings.TrimSuffix(strings.TrimPrefix(key, "/"), "/")
}
