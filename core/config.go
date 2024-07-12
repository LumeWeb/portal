package core

const CONFIG_SERVICE = "config"

type ConfigPropertyUpdateHandler func(key string, value any) error

type ConfigService interface {
	RegisterPropertyHandler(key string, handler ConfigPropertyUpdateHandler)
}

type ConfigChanger interface {
	RegisterConfigPropertyHandlers(ConfigService)
}
