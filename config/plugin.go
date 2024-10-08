package config

type PluginEntity struct {
	Protocol ProtocolConfig           `config:"protocol"`
	API      APIConfig                `config:"api"`
	Service  map[string]ServiceConfig `config:"service"`
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
