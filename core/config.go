package core

import "go.lumeweb.com/portal/config"

const CONFIG_SERVICE = "config"

type ConfigPropertyUpdateHandler func(key string, value any) error

type ConfigService interface {
	RegisterPropertyHandler(scope config.Scope, handler ConfigPropertyUpdateHandler)
}

type ConfigPropertyUpdateCategory string

const (
	CONFIG_PROPERTY_UPDATE_EVENT_CATEGORY_CORE     ConfigPropertyUpdateCategory = "core"
	CONFIG_PROPERTY_UPDATE_EVENT_CATEGORY_PROTOCOL ConfigPropertyUpdateCategory = "protocol"
	CONFIG_PROPERTY_UPDATE_EVENT_CATEGORY_API      ConfigPropertyUpdateCategory = "api"
	CONFIG_PROPERTY_UPDATE_EVENT_CATEGORY_SERVICE  ConfigPropertyUpdateCategory = "service"
)
