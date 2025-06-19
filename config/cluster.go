package config

import (
	z "github.com/Oudwins/zog"
	"go.lumeweb.com/configmanager"
	"go.lumeweb.com/configmanager/config"
)

var (
	_ configmanager.ConfigSchemaProvider = (*ClusterConfig)(nil)
	_ Defaults                           = (*ClusterConfig)(nil)
)

type ClusterConfig struct {
	Enabled bool               `config:"enabled"`
	Redis   *RedisConfig       `config:"redis"`
	Etcd    *config.EtcdConfig `config:"etcd"`
}

func (c ClusterConfig) Schema() z.ZogSchema {
	return z.Struct(z.Shape{
		"Enabled": z.Bool(),
	}).TestFunc(func(data any, ctx z.Ctx) bool {
		c, ok := data.(*ClusterConfig)
		if !ok {
			return true
		}

		if c.Enabled && c.Etcd == nil {
			ctx.AddIssue(ctx.Issue().SetMessage("etcd configuration is required when cluster is enabled"))
			return false
		}
		return true
	})
}

func (c ClusterConfig) RedisEnabled() bool {
	return c.Redis != nil
}

func (c ClusterConfig) Defaults() map[string]any {
	return map[string]any{
		"Enabled": false,
	}
}

func (c ClusterConfig) EtcdEnabled() bool {
	return c.Etcd != nil
}
