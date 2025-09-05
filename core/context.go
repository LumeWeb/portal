package core

import (
	"context"
	"fmt"
	"go.lumeweb.com/event/v2"
	"go.lumeweb.com/portal/config"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"reflect"
)

var _ Context = (*DefaultContext)(nil)

type LifecycleFunc func(Context) error

// Context interface
type Context interface {
	context.Context
	Service(id string) any
	OnExit(f LifecycleFunc)
	OnStartup(f LifecycleFunc)
	StartupFuncs() []func(Context) error
	ExitFuncs() []func(Context) error
	DB() *gorm.DB
	Logger() *Logger
	ProtocolLogger(protocol Protocol) *Logger
	APILogger(api API) *Logger
	ServiceLogger(service Service) *Logger
	NamedLogger(name string) *Logger
	WithLoggerOptions(opts ...zap.Option) *Logger
	WithLoggerLazy(opts ...zap.Field) *Logger
	WithLogger(opts ...zap.Field) *Logger
	Config() config.Manager
	Cancel()
	ExitCode() int
	Event() event.EventManager[any]
	SetExitCode(code int)
	GetContext() context.Context

	// Event helpers
	Fire(eventName string, payload any) error
	MustFire(eventName string, payload any)
	FireAsync(eventName string, payload any)
	ResetEvents()
}

// DefaultContext struct implementing the Context interface
type DefaultContext struct {
	context.Context
	services     map[string]any
	cfg          config.Manager
	logger       *Logger
	exitFuncs    []func(Context) error
	exitCode     int
	startupFuncs []func(Context) error
	db           *gorm.DB
	cancel       context.CancelFunc
	event        event.EventManager[any]
}

// NewContext creates a new Context
func NewContext(config config.Manager, logger *Logger, options ...ContextBuilderOption) (Context, error) {
	// Create a new context with cancel
	baseCtx, cancel := context.WithCancel(context.Background())

	newCtx := &DefaultContext{
		Context:  baseCtx,
		services: make(map[string]any),
		cfg:      config,
		logger:   logger,
		event:    event.NewManager[any](""),
		cancel:   cancel,
	}

	options = append(options, ContextWithExitFunc(func(ctx Context) error {
		return ctx.Event().CloseWait()
	}))

	return ProcessCtxOptions(newCtx, options...)
}

func ProcessCtxOptions(ctx Context, options ...ContextBuilderOption) (Context, error) {
	var err error
	currentCtx := ctx
	newCtx := currentCtx

	for _, opt := range options {
		currentCtx, err = opt(currentCtx)
		if err != nil {
			return currentCtx, err
		}
		// Type assert back to *defaultContext if needed
		if dc, ok := currentCtx.(*DefaultContext); ok {
			newCtx = dc
		} else {
			return currentCtx, fmt.Errorf("context type changed unexpectedly")
		}
	}

	return newCtx, nil
}

// Implement the Context interface methods for defaultContext

func (ctx *DefaultContext) Service(id string) any {
	if svc, ok := ctx.services[id]; ok {
		return svc
	}
	return nil
}

func (ctx *DefaultContext) OnExit(f LifecycleFunc) {
	ctx.exitFuncs = append(ctx.exitFuncs, f)
}

func (ctx *DefaultContext) OnStartup(f LifecycleFunc) {
	ctx.startupFuncs = append(ctx.startupFuncs, f)
}

func (ctx *DefaultContext) StartupFuncs() []func(Context) error {
	return ctx.startupFuncs
}

func (ctx *DefaultContext) ExitFuncs() []func(Context) error {
	return ctx.exitFuncs
}

func (ctx *DefaultContext) DB() *gorm.DB {
	return ctx.db.WithContext(ctx)
}

func (ctx *DefaultContext) Logger() *Logger {
	return ctx.logger
}

func (ctx *DefaultContext) Config() config.Manager {
	return ctx.cfg
}

func (ctx *DefaultContext) Cancel() {
	ctx.cancel()
}

func (ctx *DefaultContext) ExitCode() int {
	return ctx.exitCode
}

func (ctx *DefaultContext) Event() event.EventManager[any] {
	return ctx.event
}

func (ctx *DefaultContext) SetExitCode(code int) {
	ctx.exitCode = code
}

func (ctx *DefaultContext) Value(key any) any {
	return ctx.Context.Value(key)
}

func (ctx *DefaultContext) GetContext() context.Context {
	return ctx.Context
}

func ensureValue(payload any) any {
	if payload == nil {
		return nil
	}
	val := reflect.ValueOf(payload)
	if val.Kind() == reflect.Ptr {
		// If it's a pointer, check if it's nil before dereferencing
		if val.IsNil() {
			return nil
		}
		// Return the pointed-to value
		return val.Elem().Interface()
	}
	// For non-pointer values, return as-is
	return payload
}

func (ctx *DefaultContext) Fire(eventName string, payload any) error {
	val := ensureValue(payload)
	return Fire[any](ctx, eventName, &val)
}

func (ctx *DefaultContext) MustFire(eventName string, payload any) {
	val := ensureValue(payload)
	MustFire[any](ctx, eventName, &val)
}

func (ctx *DefaultContext) FireAsync(eventName string, payload any) {
	val := ensureValue(payload)
	FireAsync[any](ctx, eventName, &val)
}

func (ctx *DefaultContext) ResetEvents() {
	ctx.Event().Reset()
}

func (ctx *DefaultContext) ProtocolLogger(protocol Protocol) *Logger {
	return ctx.NamedLogger(fmt.Sprintf("protocol-%s", protocol.Name()))
}

func (ctx *DefaultContext) APILogger(api API) *Logger {
	return ctx.NamedLogger(fmt.Sprintf("api-%s", api.Name()))
}

func (ctx *DefaultContext) ServiceLogger(service Service) *Logger {
	return ctx.NamedLogger(fmt.Sprintf("service-%s", service.ID()))
}

func (ctx *DefaultContext) NamedLogger(name string) *Logger {
	return &Logger{
		Logger: ctx.logger.Logger.Named(name),
		level:  ctx.logger.level,
		cm:     ctx.logger.cm,
	}
}

func (ctx *DefaultContext) WithLoggerOptions(opts ...zap.Option) *Logger {
	return &Logger{
		Logger: ctx.logger.Logger.WithOptions(opts...),
		level:  ctx.logger.level,
		cm:     ctx.logger.cm,
	}
}

func (ctx *DefaultContext) WithLoggerLazy(opts ...zap.Field) *Logger {
	return &Logger{
		Logger: ctx.logger.Logger.WithLazy(opts...),
		level:  ctx.logger.level,
		cm:     ctx.logger.cm,
	}
}

func (ctx *DefaultContext) WithLogger(opts ...zap.Field) *Logger {
	return &Logger{
		Logger: ctx.logger.Logger.With(opts...),
		level:  ctx.logger.level,
		cm:     ctx.logger.cm,
	}
}

// ContextBuilderOption and related functions

type ContextBuilderOption func(Context) (Context, error)

func ContextWithService(id string, svc Service) ContextBuilderOption {
	return func(ctx Context) (Context, error) {
		if defaultCtx, ok := ctx.(*DefaultContext); ok {
			defaultCtx.services[id] = svc
		}
		return ctx, nil
	}
}

func ContextWithStartupFunc(f LifecycleFunc) ContextBuilderOption {
	return func(ctx Context) (Context, error) {
		ctx.OnStartup(f)
		return ctx, nil
	}
}

func ContextWithExitFunc(f LifecycleFunc) ContextBuilderOption {
	return func(ctx Context) (Context, error) {
		ctx.OnExit(f)
		return ctx, nil
	}
}

func ContextWithEvents(events ...event.Event[any]) ContextBuilderOption {
	return func(ctx Context) (Context, error) {
		for _, e := range events {
			ctx.Event().AddEvent(e)
		}
		return ctx, nil
	}
}

func ContextWithDB(db *gorm.DB) ContextBuilderOption {
	return func(ctx Context) (Context, error) {
		if defaultCtx, ok := ctx.(*DefaultContext); ok {
			defaultCtx.db = db
		}
		return ctx, nil
	}
}

func ContextWithLoggerOptions(opts ...zap.Option) ContextBuilderOption {
	return func(ctx Context) (Context, error) {
		if defaultCtx, ok := ctx.(*DefaultContext); ok {
			defaultCtx.logger = defaultCtx.WithLoggerOptions(opts...)
		}
		return ctx, nil
	}
}

func ContextOptions(options ...ContextBuilderOption) []ContextBuilderOption {
	return options
}

func GetService[T Service](ctx Context, id string) T {
	svc := ctx.Service(id)
	if svc == nil {
		ctx.Logger().Fatal("service not found", zap.String("service", id))
	}

	typedSvc, ok := svc.(T)

	if !ok {
		ctx.Logger().Fatal("service type mismatch", zap.String("service", id))
	}

	return typedSvc
}

func GetServiceConfig[T config.ServiceConfig](ctx Context, id string) T {
	var zero T
	// Find which plugin owns this service
	pluginID := GetPluginForService(id)
	if pluginID == "" {
		ctx.Logger().Fatal("service has no plugin association", zap.String("service", id))
		var zero T
		return zero
	}

	// Verify service belongs to plugin
	plugin := GetPlugin(pluginID)
	if plugin.ID == "" {
		ctx.Logger().Fatal("plugin not found", zap.String("plugin", pluginID))
	}

	if !PluginHasServices(plugin) {
		ctx.Logger().Fatal("plugin has no services", zap.String("plugin", pluginID))
	}

	pluginSvcs, err := plugin.Services()
	if err != nil {
		ctx.Logger().Fatal("failed to get plugin services",
			zap.String("plugin", pluginID),
			zap.Error(err))
	}

	// Check if service exists in plugin
	found := false
	for _, svc := range pluginSvcs {
		if svc.ID == id {
			found = true
			break
		}
	}
	if !found {
		ctx.Logger().Error("service not found in plugin",
			zap.String("service", id),
			zap.String("plugin", pluginID))
		return zero
	}

	// Get the service config from the owning plugin
	cfg := ctx.Config().GetService(pluginID, id)
	if cfg == nil {
		ctx.Logger().Error("service config not found",
			zap.String("service", id),
			zap.String("plugin", pluginID))
		return zero
	}

	typedSvc, ok := cfg.(T)
	if !ok {
		ctx.Logger().Error("service type mismatch",
			zap.String("service", id),
			zap.String("expected", reflect.TypeOf(*new(T)).String()),
			zap.String("actual", reflect.TypeOf(cfg).String()))
		return zero
	}

	return typedSvc
}

func GetAPIConfig[T config.APIConfig](ctx Context, id string) T {
	var zero T
	cfg := ctx.Config().GetAPI(id)
	if cfg == nil {
		ctx.Logger().Error("api not found", zap.String("api", id))
		return zero
	}

	typedSvc, ok := cfg.(T)
	if !ok {
		ctx.Logger().Error("api type mismatch", 
			zap.String("api", id),
			zap.String("expected", reflect.TypeOf(*new(T)).String()),
			zap.String("actual", reflect.TypeOf(cfg).String()))
		return zero
	}

	return typedSvc
}

func GetProtocolConfig[T config.ProtocolConfig](ctx Context, id string) T {
	var zero T
	cfg := ctx.Config().GetProtocol(id)
	if cfg == nil {
		ctx.Logger().Error("protocol not found", zap.String("protocol", id))
		return zero
	}

	typedSvc, ok := cfg.(T)
	if !ok {
		ctx.Logger().Error("protocol type mismatch",
			zap.String("protocol", id),
			zap.String("expected", reflect.TypeOf(*new(T)).String()),
			zap.String("actual", reflect.TypeOf(cfg).String()))
		return zero
	}

	return typedSvc
}

func ServiceExists(ctx Context, id string) bool {
	if ctx.Service(id) == nil {
		return false
	}
	return true
}

// ResetState resets all global state in the core package for testing purposes
func ResetState() {
	ResetProtocols()
	ResetAPIs()
	ResetServices()
	ResetHashAlgorithms()
	ResetErrorRegistry()

	// Reset plugins
	pluginsMu.Lock()
	plugins = make(map[string]PluginInfo)
	pluginsOrderedMu.Lock()
	pluginsOrdered = nil
	pluginsOrderedMu.Unlock()
	pluginsMu.Unlock()
}
