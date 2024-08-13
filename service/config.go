package service

import (
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	event "go.lumeweb.com/portal/event"
	"go.uber.org/zap"
	"strings"
	"sync"
)

var _ core.ConfigService = (*ConfigServiceDefault)(nil)
var _ core.Service = (*ConfigServiceDefault)(nil)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID: core.CONFIG_SERVICE,
		Factory: func() (core.Service, []core.ContextBuilderOption, error) {
			return NewConfigService()
		},
	})
}

type ConfigServiceDefault struct {
	ctx           core.Context
	logger        *core.Logger
	config        config.Manager
	handlers      map[string]core.ConfigPropertyUpdateHandler
	handlersMutex sync.RWMutex
	scope         *scope
}

type scope struct {
	category event.ConfigPropertyUpdateCategory
	entity   string
}

func (cs *ConfigServiceDefault) ID() string {
	return core.CONFIG_SERVICE
}

func NewConfigService() (*ConfigServiceDefault, []core.ContextBuilderOption, error) {
	cs := &ConfigServiceDefault{
		handlers:      make(map[string]core.ConfigPropertyUpdateHandler),
		handlersMutex: sync.RWMutex{},
	}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			cs.ctx = ctx
			cs.logger = ctx.ServiceLogger(cs)
			cs.config = ctx.Config()
			cs.config.RegisterConfigChangeCallback(cs.handleConfigChange)

			event.Listen[*event.BootCompleteEvent](ctx, event.EVENT_BOOT_COMPLETE, func(evt *event.BootCompleteEvent) error {
				cs.registerChangers()
				return nil
			})
			return nil
		}),
	)

	return cs, opts, nil
}

func (cs *ConfigServiceDefault) RegisterPropertyHandler(key string, handler core.ConfigPropertyUpdateHandler) {
	fullKey := getHandlerKey(cs.scope.category, cs.scope.entity, key)

	if fullKey == "" {
		cs.logger.Warn("Empty key provided to RegisterPropertyHandler", zap.String("category", string(cs.scope.category)), zap.String("entity", cs.scope.entity), zap.String("key", key))
		return
	}

	cs.handlers[fullKey] = handler
}

func (cs *ConfigServiceDefault) registerChangers() {
	cs.handlersMutex.Lock()
	defer cs.handlersMutex.Unlock()
	// Register handlers for services
	for _, service := range core.GetServices() {
		if changer, ok := cs.ctx.Service(service.ID).(core.ConfigChanger); ok {
			cs.registerChangerHandlers(event.CONFIG_PROPERTY_UPDATE_EVENT_CATEGORY_SERVICE, service.ID, changer)
		}
	}

	// Register handlers for protocols
	for _, protocol := range core.GetProtocolList() {
		if changer, ok := protocol.(core.ConfigChanger); ok {
			cs.registerChangerHandlers(event.CONFIG_PROPERTY_UPDATE_EVENT_CATEGORY_PROTOCOL, protocol.Name(), changer)
		}
	}

	// Register handlers for APIs
	for _, api := range core.GetAPIList() {
		if changer, ok := api.(core.ConfigChanger); ok {
			cs.registerChangerHandlers(event.CONFIG_PROPERTY_UPDATE_EVENT_CATEGORY_API, api.Name(), changer)
		}
	}
}

func (cs *ConfigServiceDefault) registerChangerHandlers(category event.ConfigPropertyUpdateCategory, entity string, changer core.ConfigChanger) {
	cs.scope = &scope{
		category: category,
		entity:   entity,
	}

	changer.RegisterConfigPropertyHandlers(cs)
}

func (cs *ConfigServiceDefault) handleConfigChange(key string, value any) error {
	category, entity, property := getComponents(key)
	if category == "" {
		return nil
	}

	cs.handlersMutex.RLock()
	defer cs.handlersMutex.RUnlock()

	handlerKey := getHandlerKey(category, entity, property)
	if handler, ok := cs.handlers[handlerKey]; ok {
		if err := handler(property, value); err != nil {
			return err
		}
	}

	return event.FireConfigPropertyUpdateEvent(cs.ctx, property, value, category, entity)
}

func getComponents(key string) (category event.ConfigPropertyUpdateCategory, entity string, property string) {
	parts := strings.SplitN(key, ".", 3)
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
		entity = parts[1]
		switch parts[2] {
		case "protocol":
			category = event.CONFIG_PROPERTY_UPDATE_EVENT_CATEGORY_PROTOCOL
		case "service":
			category = event.CONFIG_PROPERTY_UPDATE_EVENT_CATEGORY_SERVICE
		case "api":
			category = event.CONFIG_PROPERTY_UPDATE_EVENT_CATEGORY_API
		default:
			return
		}
		property = strings.Join(parts[3:], ".")
	}

	return
}

func getHandlerKey(category event.ConfigPropertyUpdateCategory, entity, property string) string {
	switch category {
	case event.CONFIG_PROPERTY_UPDATE_EVENT_CATEGORY_CORE:
		return "core." + property
	case event.CONFIG_PROPERTY_UPDATE_EVENT_CATEGORY_SERVICE:
		return "plugin." + entity + ".service." + property
	case event.CONFIG_PROPERTY_UPDATE_EVENT_CATEGORY_PROTOCOL:
		return "plugin." + entity + ".protocol." + property
	case event.CONFIG_PROPERTY_UPDATE_EVENT_CATEGORY_API:
		return "plugin." + entity + ".api." + property
	default:
		return ""
	}
}
