package config

import (
	z "github.com/Oudwins/zog"
	"go.lumeweb.com/configmanager"
	"math/big"
)

var (
	_ configmanager.ConfigSchemaProvider = (*SiaConfig)(nil)
	_ Defaults                           = (*SiaConfig)(nil)
)

type SiaConfig struct {
	Key     string `config:"key"`
	URL     string `config:"url"`
	Cluster bool   `config:"cluster"`
}

func (s SiaConfig) Schema() z.ZogSchema {
	return z.Struct(z.Shape{
		"Key": z.String(),
		"URL": z.String(),
		"Cluster": z.Bool().
			Default(false),
	}).TestFunc(func(data any, ctx z.Ctx) bool {
		s, ok := data.(*SiaConfig)
		if !ok {
			return true
		}

		if s.Key != "" {
			if rat, ok := new(big.Rat).SetString(s.Key); !ok {
				ctx.AddIssue(ctx.Issue().SetMessage("failed to parse key"))
				return false
			} else if rat.Cmp(new(big.Rat).SetUint64(0)) <= 0 {
				ctx.AddIssue(ctx.Issue().SetMessage("key must be greater than 0"))
				return false
			}
		}

		if s.Cluster && s.URL == "" {
			ctx.AddIssue(ctx.Issue().SetMessage("core.storage.sia.url is required when cluster is enabled"))
			return false
		}
		return true
	})
}

func (s SiaConfig) Defaults() map[string]interface{} {
	return map[string]interface{}{
		"Key":     "",
		"Cluster": false,
		"URL":     "",
	}
}
