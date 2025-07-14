package config

import (
	"go.lumeweb.com/configmanager"
	"go.lumeweb.com/configmanager/source"
	"go.uber.org/zap"
)

type Defaults interface {
	source.ConfigDefaults
}

type Validator interface {
	Validate() error
}

type ConfigSchemaProvider = configmanager.ConfigSchemaProvider

type Manager interface {
	configmanager.Manager

	// Custom methods specific to our implementation
	Init() error
	SetLogger(logger *zap.Logger)
	Config() *Config
	EnableSync(opts ...configmanager.ConfigOption) error

	// Plugin configuration methods
	ConfigureProtocol(pluginName string, cfg ProtocolConfig) error
	ConfigureAPI(pluginName string, cfg APIConfig) error
	ConfigureService(pluginName string, serviceName string, cfg ServiceConfig) error
	GetService(pluginName string, serviceName string) ServiceConfig
	GetProtocol(pluginName string) ProtocolConfig
	GetAPI(pluginName string) APIConfig
}

type Config struct {
	Core   CoreConfig              `config:"core"`
	Plugin map[string]PluginEntity `config:"plugin"`
}
