package config

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/mitchellh/mapstructure"

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
	Storage         StorageConfig  `mapstructure:"storage"`
	Protocols       []string       `mapstructure:"protocols"`
	Mail            MailConfig     `mapstructure:"mail"`
	Clustered       *ClusterConfig `mapstructure:"clustered"`
	NodeID          UUID           `mapstructure:"node_id"`
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
		"node_id":           NewUUID(),
	}
}

func coreConfigHook() mapstructure.DecodeHookFuncType {
	return func(f reflect.Type, t reflect.Type, data interface{}) (interface{}, error) {
		if f.Kind() != reflect.Map || t != reflect.TypeOf(CoreConfig{}) {
			return data, nil
		}

		var coreConfig CoreConfig
		if err := mapstructure.Decode(data, &coreConfig); err != nil {
			return nil, err
		}

		// Check if the input data map includes "node_id" configuration
		if nodeID, ok := data.(map[string]interface{})["node_id"]; ok {
			switch v := nodeID.(type) {
			case string:
				if v != "" {
					parsed, err := ParseUUID(v)
					if err != nil {
						return nil, err
					}
					coreConfig.NodeID = parsed
				}
			case []byte:
				s := string(v)
				if s != "" {
					parsed, err := ParseUUID(s)
					if err != nil {
						return nil, err
					}
					coreConfig.NodeID = parsed
				}
			default:
				return nil, fmt.Errorf("unsupported type for node_id: %T", v)
			}
		}

		return coreConfig, nil
	}
}
