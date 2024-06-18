package config

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
	Config() *Config
	Save() error
	ConfigFile() string
	ConfigDir() string
}
