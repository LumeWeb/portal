package config

import (
	"errors"
	"reflect"

	"github.com/go-viper/mapstructure/v2"
)

var _ Validator = (*ClusterConfig)(nil)

type ClusterConfig struct {
	Enabled bool         `mapstructure:"enabled"`
	Redis   *RedisConfig `mapstructure:"redis"`
	Etcd    *EtcdConfig  `mapstructure:"etcd"`
}

func (c ClusterConfig) Validate() error {
	if c.Enabled {
		if c.Etcd == nil {
			return errors.New("etcd configuration is required in cluster configuration")
		}
	}

	return nil
}

func (c ClusterConfig) RedisEnabled() bool {
	return c.Redis != nil
}

func clusterConfigHook() mapstructure.DecodeHookFuncType {
	return func(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
		if f.Kind() != reflect.Map || t != reflect.TypeOf(&ClusterConfig{}) {
			return data, nil
		}

		var clusterConfig ClusterConfig
		if err := mapstructure.Decode(data, &clusterConfig); err != nil {
			return nil, err
		}

		// Check if the input data map includes "redis" configuration
		if opts, ok := data.(map[string]interface{})["redis"].(map[string]interface{}); ok && opts != nil {
			var redisOptions RedisConfig
			if err := mapstructure.Decode(opts, &redisOptions); err != nil {
				return nil, err
			}

			if err := redisOptions.Validate(); err != nil {
				return nil, err
			}

			clusterConfig.Redis = &redisOptions
		}

		// Check if the input data map includes "etcd" configuration
		if opts, ok := data.(map[string]interface{})["etcd"].(map[string]interface{}); ok && opts != nil {
			var etcdOptions EtcdConfig
			if err := mapstructure.Decode(opts, &etcdOptions); err != nil {
				return nil, err
			}

			if err := etcdOptions.Validate(); err != nil {
				return nil, err
			}

			clusterConfig.Etcd = &etcdOptions
		}

		return &clusterConfig, nil
	}
}
