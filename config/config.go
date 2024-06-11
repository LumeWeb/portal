package config

type ProtocolConfig interface {
	Defaults
}

type Defaults interface {
	Defaults() map[string]any
}

type Validator interface {
	Validate() error
}

type Manager interface {
	Init() error
	ConfigureProtocol(name string, cfg ProtocolConfig) error
	Config() *Config
	Save() error
	ConfigFile() string
	ConfigDir() string
}
