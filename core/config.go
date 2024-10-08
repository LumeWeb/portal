package core

import "go.lumeweb.com/portal/config"

const CONFIG_SERVICE = "config"

type ConfigPropertyUpdateHandler func(key string, value any) error

type ConfigService interface {
	RegisterPropertyHandler(scope config.Scope, handler ConfigPropertyUpdateHandler)
}
