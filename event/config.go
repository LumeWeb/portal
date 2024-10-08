package event

import (
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
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

func (e *ConfigPropertyUpdateEvent) SetCategory(category config.ConfigPropertyUpdateCategory) {
	e.Set("category", category)
}

func (e ConfigPropertyUpdateEvent) SetEntity(entity string) {
	e.Set("entity", entity)
}

func (e ConfigPropertyUpdateEvent) Entity() string {
	return e.Get("entity").(string)
}

func (e ConfigPropertyUpdateEvent) SetSubEntity(subEntity string) {
	e.Set("sub_entity", subEntity)
}

func (e ConfigPropertyUpdateEvent) SubEntity() string {
	return e.Get("sub_entity").(string)
}

func (e ConfigPropertyUpdateEvent) Category() config.ConfigPropertyUpdateCategory {
	return e.Get("category").(config.ConfigPropertyUpdateCategory)
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

func FireConfigPropertyUpdateEvent(ctx core.Context, key string, value interface{}, category config.ConfigPropertyUpdateCategory, entity string, subEntity string) error {
	return Fire[*ConfigPropertyUpdateEvent](ctx, EVENT_CONFIG_PROPERTY_UPDATE, func(evt *ConfigPropertyUpdateEvent) error {
		evt.SetProperty(key, value)
		evt.SetCategory(category)
		evt.SetEntity(entity)
		evt.SetSubEntity(subEntity)

		return nil
	})
}
