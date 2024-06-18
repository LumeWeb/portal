package config

type PluginEntity struct {
	Protocol ProtocolConfig           `mapstructure:"protocol"`
	API      APIConfig                `mapstructure:"api"`
	Service  map[string]ServiceConfig `mapstructure:"service"`
}

type ProtocolConfig interface {
	Defaults
}

type APIConfig interface {
	Defaults
}

type ServiceConfig interface {
	Defaults
}
