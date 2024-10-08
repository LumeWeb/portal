package config

import (
	"fmt"
	"go.lumeweb.com/portal/event"
	"strings"
)

type Scope struct {
	category  event.ConfigPropertyUpdateCategory
	entity    string
	subEntity string
	property  string
}

func NewScope(category event.ConfigPropertyUpdateCategory, entity, subEntity, property string) Scope {
	return Scope{
		category:  category,
		entity:    entity,
		subEntity: subEntity,
		property:  property,
	}
}

func NewScopeFromKey(key string) Scope {
	category, entity, subEntity, property := getComponents(key)
	return NewScope(category, entity, subEntity, property)
}

func (s Scope) Category() event.ConfigPropertyUpdateCategory {
	return s.category
}

func (s Scope) Entity() string {
	return s.entity
}

func (s Scope) SubEntity() string {
	return s.subEntity
}

func (s Scope) Property() string {
	return s.property
}

func (s Scope) Key() string {
	return getHandlerKey(s.category, s.entity, s.subEntity, s.property)
}

func getComponents(key string) (category event.ConfigPropertyUpdateCategory, entity string, subEntity string, property string) {
	parts := strings.SplitN(key, ".", 4)
	if len(parts) < 2 {
		return
	}

	switch parts[0] {
	case "core":
		property = strings.Join(parts[1:], ".")
		category = event.CONFIG_PROPERTY_UPDATE_EVENT_CATEGORY_CORE
		return
	case "plugin":
		if len(parts) < 4 {
			return
		}
		entity = parts[1] // Plugin name
		switch parts[2] {
		case "protocol":
			category = event.CONFIG_PROPERTY_UPDATE_EVENT_CATEGORY_PROTOCOL
		case "service":
			category = event.CONFIG_PROPERTY_UPDATE_EVENT_CATEGORY_SERVICE
			if len(parts) >= 4 {
				subEntity = parts[3] // Service name
				property = strings.Join(parts[4:], ".")
			}
		case "api":
			category = event.CONFIG_PROPERTY_UPDATE_EVENT_CATEGORY_API
		default:
			return
		}

		if category != event.CONFIG_PROPERTY_UPDATE_EVENT_CATEGORY_SERVICE {
			property = strings.Join(parts[3:], ".")
		}
	}

	return
}

func getHandlerKey(category event.ConfigPropertyUpdateCategory, entity, subEntity, property string) string {
	switch category {
	case event.CONFIG_PROPERTY_UPDATE_EVENT_CATEGORY_CORE:
		return GetCoreSectionSpecifier(property)
	case event.CONFIG_PROPERTY_UPDATE_EVENT_CATEGORY_SERVICE:
		return fmt.Sprintf("%s.%s", GetServiceSectionSpecifier(entity, subEntity), property)
	case event.CONFIG_PROPERTY_UPDATE_EVENT_CATEGORY_PROTOCOL:
		return fmt.Sprintf("%s.%s", GetProtoSectionSpecifier(entity), property)
	case event.CONFIG_PROPERTY_UPDATE_EVENT_CATEGORY_API:
		return fmt.Sprintf("%s.%s", GetAPISectionSpecifier(entity), property)
	default:
		return ""
	}
}
