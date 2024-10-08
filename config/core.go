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
