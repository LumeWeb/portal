package config

type ProtocolConfig interface {
	Defaults() map[string]interface{}
}
