package event

import (
	"go.lumeweb.com/portal/config"
)

const EVENT_CONFIG_PROPERTY_UPDATE = "config.property.update"

type ConfigPropertyUpdateEvent struct {
	Category     config.ConfigPropertyUpdateCategory
	Entity       string
	SubEntity    string
	PropertyKey  string
	PropertyValue interface{}
}

func NewConfigPropertyUpdateEvent(
	category config.ConfigPropertyUpdateCategory,
	entity string,
	subEntity string,
	key string,
	value interface{},
) *ConfigPropertyUpdateEvent {
	return &ConfigPropertyUpdateEvent{
		Category:     category,
		Entity:       entity,
		SubEntity:    subEntity,
		PropertyKey:  key,
		PropertyValue: value,
	}
}
