package config

import (
	"errors"
	"github.com/docker/go-units"
	"go.lumeweb.com/portal/config/types"
	"go.sia.tech/coreutils/wallet"
)

var _ Defaults = (*CoreConfig)(nil)
var _ Validator = (*CoreConfig)(nil)

type CoreConfig struct {
	DB              DatabaseConfig `mapstructure:"db"`
	Domain          string         `mapstructure:"domain"`
	PortalName      string         `mapstructure:"portal_name"`
	ExternalPort    uint           `mapstructure:"external_port"`
	Identity        types.Identity `mapstructure:"identity"`
	Log             LogConfig      `mapstructure:"log"`
	Port            uint           `mapstructure:"port"`
	PostUploadLimit uint64         `mapstructure:"post_upload_limit"`
	Storage         StorageConfig  `mapstructure:"storage"`
	Mail            MailConfig     `mapstructure:"mail"`
	Clustered       *ClusterConfig `mapstructure:"clustered"`
	NodeID          types.UUID     `mapstructure:"node_id" flags:"nosync"`
	Cron            CronConfig     `mapstructure:"cron"`
	Account         AccountConfig  `mapstructure:"account"`
}

func (c CoreConfig) Validate() error {
	if c.Domain == "" {
		return errors.New("core.domain is required")
	}
	if c.PortalName == "" {
		return errors.New("core.portal_name is required")
	}
	if c.Port == 0 {
		return errors.New("core.port is required")
	}

	return nil
}

func (c CoreConfig) Defaults() map[string]any {
	return map[string]interface{}{
		"post_upload_limit": units.MiB * 100,
		"node_id":           types.NewUUID(),
		"identity":          wallet.NewSeedPhrase(),
		"domain":            "",
		"portal_name":       "",
		"port":              0,
	}
}

func (c CoreConfig) ClusterEnabled() bool {
	return c.Clustered != nil && c.Clustered.Enabled
}
