package config

import (
	"errors"
	types2 "github.com/LumeWeb/portal/config/types"
	"github.com/docker/go-units"
	"go.sia.tech/coreutils/wallet"
)

var _ Defaults = (*CoreConfig)(nil)
var _ Validator = (*CoreConfig)(nil)

type CoreConfig struct {
	DB               DatabaseConfig  `mapstructure:"db"`
	Domain           string          `mapstructure:"domain"`
	PortalName       string          `mapstructure:"portal_name"`
	AccountSubdomain string          `mapstructure:"account_subdomain"`
	ExternalPort     uint            `mapstructure:"external_port"`
	Identity         types2.Identity `mapstructure:"identity"`
	Log              LogConfig       `mapstructure:"log"`
	Port             uint            `mapstructure:"port"`
	PostUploadLimit  uint64          `mapstructure:"post_upload_limit"`
	Storage          StorageConfig   `mapstructure:"storage"`
	Protocols        []string        `mapstructure:"protocols"`
	Mail             MailConfig      `mapstructure:"mail"`
	Clustered        *ClusterConfig  `mapstructure:"clustered"`
	NodeID           types2.UUID     `mapstructure:"node_id"`
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

func (c CoreConfig) Defaults() map[string]interface{} {
	return map[string]interface{}{
		"post_upload_limit": units.MiB * 100,
		"node_id":           types2.NewUUID(),
		"account_subdomain": "account",
		"identity":          wallet.NewSeedPhrase(),
		"domain":            "",
		"portal_name":       "",
		"port":              0,
	}
}

func (c CoreConfig) ClusterEnabled() bool {
	return c.Clustered != nil && c.Clustered.Enabled
}
