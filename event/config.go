package event

import (
	"go.lumeweb.com/portal/core"
)

type ConfigPropertyUpdateCategory string

const (
	CONFIG_PROPERTY_UPDATE_EVENT_CATEGORY_CORE     ConfigPropertyUpdateCategory = "core"
	CONFIG_PROPERTY_UPDATE_EVENT_CATEGORY_PROTOCOL ConfigPropertyUpdateCategory = "protocol"
	CONFIG_PROPERTY_UPDATE_EVENT_CATEGORY_API      ConfigPropertyUpdateCategory = "api"
	CONFIG_PROPERTY_UPDATE_EVENT_CATEGORY_SERVICE  ConfigPropertyUpdateCategory = "service"
)

const (
	EVENT_CONFIG_PROPERTY_UPDATE = "config.property.update"
)

func init() {
	core.RegisterEvent(EVENT_CONFIG_PROPERTY_UPDATE, &ConfigPropertyUpdateEvent{})
}

type ConfigPropertyUpdateEvent struct {
	core.Event
}

func (e *ConfigPropertyUpdateEvent) SetCategory(category ConfigPropertyUpdateCategory) {
	e.Set("category", category)
}

func (e ConfigPropertyUpdateEvent) SetEntity(entity string) {
	e.Set("entity", entity)
}

func (e ConfigPropertyUpdateEvent) Entity() string {
	return e.Get("entity").(string)
}

func (e ConfigPropertyUpdateEvent) Category() ConfigPropertyUpdateCategory {
	return e.Get("category").(ConfigPropertyUpdateCategory)
}

func (e *ConfigPropertyUpdateEvent) SetProperty(key string, value interface{}) {
	e.Set("property_key", key)
	e.Set("property_value", value)
}

func (e ConfigPropertyUpdateEvent) PropertyKey() string {
	return e.Get("property_key").(string)
}

func (e ConfigPropertyUpdateEvent) PropertyValue() interface{} {
	return e.Get("property_value")
}

func FireConfigPropertyUpdateEvent(ctx core.Context, key string, value interface{}, category ConfigPropertyUpdateCategory, entity string) error {
	return Fire[*ConfigPropertyUpdateEvent](ctx, EVENT_CONFIG_PROPERTY_UPDATE, func(evt *ConfigPropertyUpdateEvent) error {
		evt.SetProperty(key, value)
		evt.SetCategory(category)
		evt.SetEntity(entity)
		return nil
	})
}
