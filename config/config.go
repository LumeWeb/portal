package config

import "github.com/invopop/jsonschema"

type Defaults interface {
	Defaults() map[string]any
}

type Validator interface {
	Validate() error
}

type Manager interface {
	Init() error
	ConfigureProtocol(pluginName string, cfg ProtocolConfig) error
	ConfigureAPI(pluginName string, cfg APIConfig) error
	ConfigureService(pluginName string, serviceName string, cfg ServiceConfig) error
	GetPlugin(pluginName string) *PluginEntity
	GetService(serviceName string) ServiceConfig
	GetProtocol(pluginName string) ProtocolConfig
	GetAPI(pluginName string) APIConfig
	Config() *Config
	Save() error
	SaveChanged() error
	ConfigFile() string
	ConfigDir() string
	LiveConfig() *jsonschema.Schema
	PropertyLiveEditable(property string) bool
	PropertyLiveReadable(property string) bool
	PropertyLiveExists(property string) bool
	LiveUpdateProperty(key string, value any) error
}
