package service

import (
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	event "go.lumeweb.com/portal/event"
	"go.uber.org/zap"
	"reflect"
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
			return nil
		}),
	)

	return cs, opts, nil
}

func (cs *ConfigServiceDefault) RegisterPropertyHandler(scope config.Scope, handler core.ConfigPropertyUpdateHandler) {
	fullKey := scope.Key()

	if fullKey == "" {
		cs.logger.Warn("Empty key provided to RegisterPropertyHandler", zap.String("category", string(scope.Category())), zap.String("entity", scope.Entity()), zap.String("subEntity", scope.SubEntity()), zap.String("property", scope.Property()))
		return
	}

	cs.handlersMutex.Lock()
	defer cs.handlersMutex.Unlock()

	cs.handlers[fullKey] = handler
}

func (cs *ConfigServiceDefault) handleConfigChange(key string, value any) error {
	_scope := config.NewScopeFromKey(key)
	category := _scope.Category()
	entity := _scope.Entity()
	subEntity := _scope.SubEntity()
	property := _scope.Property()

	if category == "" {
		return nil
	}

	cs.handlersMutex.RLock()
	defer cs.handlersMutex.RUnlock()

	handlerKey := _scope.Key()
	if handler, ok := cs.handlers[handlerKey]; ok {
		if err := handler(property, value); err != nil {
			return err
		}
	}

	switch category {
	case event.CONFIG_PROPERTY_UPDATE_EVENT_CATEGORY_CORE:
		err := cs.config.FieldProcessor(cs.config.Config(), "", cs.configUpdatesProcessor)
		if err != nil {
			return err
		}
	case event.CONFIG_PROPERTY_UPDATE_EVENT_CATEGORY_SERVICE:
		err := cs.config.FieldProcessor(cs.config.GetService(entity), config.GetServiceSectionSpecifier(entity, subEntity), cs.configUpdatesProcessor)
		if err != nil {
			return err
		}

	case event.CONFIG_PROPERTY_UPDATE_EVENT_CATEGORY_PROTOCOL:
		err := cs.config.FieldProcessor(cs.config.GetProtocol(entity), config.GetProtoSectionSpecifier(entity), cs.configUpdatesProcessor)
		if err != nil {
			return err
		}

	case event.CONFIG_PROPERTY_UPDATE_EVENT_CATEGORY_API:
		err := cs.config.FieldProcessor(cs.config.GetAPI(entity), config.GetAPISectionSpecifier(entity), cs.configUpdatesProcessor)
		if err != nil {
			return err
		}
	}

	return event.FireConfigPropertyUpdateEvent(cs.ctx, property, value, category, entity, subEntity)
}

func (m *ConfigServiceDefault) configUpdatesProcessor(_ reflect.StructField, value reflect.Value, prefix string) error {
	_scope := config.NewScopeFromKey(prefix)

	category := _scope.Category()
	subEntity := _scope.SubEntity()

	switch category {
	case event.CONFIG_PROPERTY_UPDATE_EVENT_CATEGORY_SERVICE:
		if core.ServiceExists(m.ctx, subEntity) {
			svc := core.GetService[core.Service](m.ctx, subEntity)
			if reconfig, ok := svc.(config.Reconfigurable); ok {
				if err := reconfig.Reconfigure(_scope, value.Interface()); err != nil {
					return err
				}
			}
		}
	case event.CONFIG_PROPERTY_UPDATE_EVENT_CATEGORY_PROTOCOL:
		if core.ProtocolExists(subEntity) {
			proto := core.GetProtocol(subEntity)
			if reconfig, ok := proto.(config.Reconfigurable); ok {
				if err := reconfig.Reconfigure(_scope, value.Interface()); err != nil {
					return err
				}
			}
		}

	case event.CONFIG_PROPERTY_UPDATE_EVENT_CATEGORY_API:
		if core.APIExists(subEntity) {
			api := core.GetAPI(subEntity)
			if reconfig, ok := api.(config.Reconfigurable); ok {
				if err := reconfig.Reconfigure(_scope, value.Interface()); err != nil {
					return err
				}
			}
		}

	}

	return nil
}
