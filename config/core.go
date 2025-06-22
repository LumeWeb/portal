package config

import (
	z "github.com/Oudwins/zog"
	"github.com/docker/go-units"
	"go.lumeweb.com/configmanager"
	"go.lumeweb.com/portal/config/types"
	"go.sia.tech/coreutils/wallet"
)

var (
	_ configmanager.ConfigSchemaProvider = (*CoreConfig)(nil)
	_ Defaults                           = (*CoreConfig)(nil)
)

type CoreConfig struct {
	DB              DatabaseConfig `config:"db"`
	Domain          string         `config:"domain"`
	PortalName      string         `config:"portal_name"`
	ExternalPort    uint           `config:"external_port"`
	Identity        types.Identity `config:"identity"`
	Log             LogConfig      `config:"log"`
	Port            uint           `config:"port"`
	PostUploadLimit uint64         `config:"post_upload_limit"`
	Storage         StorageConfig  `config:"storage"`
	Mail            MailConfig     `config:"mail"`
	Clustered       *ClusterConfig `config:"clustered"`
	NodeID          types.UUID     `config:"node_id"`
	Cron            CronConfig     `config:"cron"`
	Account         AccountConfig  `config:"account"`
}

func (c CoreConfig) Schema() z.ZogSchema {
	return z.Struct(z.Shape{
		"Domain": z.String().
			Required(z.Message("core.domain is required")),
		"PortalName": z.String().
			Required(z.Message("core.portal_name is required")),
		"Port": ZogUInt().
			Required(z.Message("core.port is required")).
			GT(0, z.Message("core.port must be greater than 0")),
		"PostUploadLimit": ZogUInt64(),
	}).TestFunc(func(data any, ctx z.Ctx) bool {
		c, ok := data.(*CoreConfig)
		if !ok {
			return true
		}

		if c.Clustered != nil && c.Clustered.Enabled && c.Clustered.Etcd == nil {
			ctx.AddIssue(ctx.Issue().SetMessage("etcd configuration is required when cluster is enabled"))
			return false
		}
		return true
	})
}

func (c CoreConfig) Defaults() map[string]any {
	return map[string]interface{}{
		"PostUploadLimit": units.MiB * 100,
		"NodeID":          types.NewUUID(),
		"Identity":        wallet.NewSeedPhrase(),
		"Domain":          "",
		"PortalName":      "",
		"Port":            8080,
	}
}

func (c CoreConfig) ClusterEnabled() bool {
	return c.Clustered != nil && c.Clustered.Enabled
}
