package config

import (
	"errors"

	"github.com/docker/go-units"
)

var _ Defaults = (*CoreConfig)(nil)
var _ Validator = (*CoreConfig)(nil)

type CoreConfig struct {
	DB              DatabaseConfig `mapstructure:"db"`
	Domain          string         `mapstructure:"domain"`
	PortalName      string         `mapstructure:"portal_name"`
	ExternalPort    uint           `mapstructure:"external_port"`
	Identity        string         `mapstructure:"identity"`
	Log             LogConfig      `mapstructure:"log"`
	Port            uint           `mapstructure:"port"`
	PostUploadLimit uint64         `mapstructure:"post_upload_limit"`
	Sia             SiaConfig      `mapstructure:"sia"`
	Storage         StorageConfig  `mapstructure:"storage"`
	Protocols       []string       `mapstructure:"protocols"`
	Mail            MailConfig     `mapstructure:"mail"`
	Clustered       *ClusterConfig `mapstructure:"clustered"`
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
	}
}
