package config

import "go.uber.org/zap"

type Defaults interface {
	Defaults() map[string]any
}

type Validator interface {
	Validate() error
}

type ConfigChangeCallback func(key string, value any) error

type Manager interface {
	Init() error
	SetLogger(logger *zap.Logger)
	RegisterConfigChangeCallback(callback ConfigChangeCallback)
	ConfigureProtocol(pluginName string, cfg ProtocolConfig) error
	ConfigureAPI(pluginName string, cfg APIConfig) error
	ConfigureService(pluginName string, serviceName string, cfg ServiceConfig) error
	GetPlugin(pluginName string) *PluginEntity
	GetService(serviceName string) ServiceConfig
	GetProtocol(pluginName string) ProtocolConfig
	GetAPI(pluginName string) APIConfig
	Config() *Config
	Save() error
	ConfigFile() string
	ConfigDir() string
}
