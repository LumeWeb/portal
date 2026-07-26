package core

import (
	"context"
	"fmt"
	"reflect"

	"go.lumeweb.com/event/v2"
	"go.lumeweb.com/portal/config"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"gorm.io/gorm"
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
	ReplaceLogger(logger *Logger)
	Cancel()
	ExitCode() int
	Event() event.EventManager[any]
	SetExitCode(code int)
	GetContext() context.Context

	// WithRequestContext returns a new Context that wraps the given request
	// context, preserving request-scoped cancellation, deadlines, and trace
	// metadata while retaining access to core services, config, DB, and logger.
	WithRequestContext(ctx context.Context) Context

	// Event helpers
	Fire(eventName string, payload any) error
	MustFire(eventName string, payload any)
	FireAsync(eventName string, payload any)
	ResetEvents()

	// Tracer helpers
	WithTracer(service, subsystem string) Context
	WithTracerService(service string) Context
	WithTracerSubsystem(subsystem string) Context
	TraceMethod(name string, opts ...SpanOption) (context.Context, trace.Span)

	// Dedicated component tracer helpers
	WithProtocolTracer(protocolName string) Context
	WithAPITracer(apiName string) Context
	WithAPIExtensionTracer(extensionName string) Context
	WithServiceTracer(serviceName string) Context

	// Subcomponent tracer helpers
	WithProtocolSubcomponent(protocolName, subcomponentName string) Context
	WithAPISubcomponent(apiName, subcomponentName string) Context
	WithAPIExtensionSubcomponent(extensionName, subcomponentName string) Context
	WithServiceSubcomponent(serviceName, subcomponentName string) Context
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

	// Set up default tracer service
	baseCtx = WithTracerService(baseCtx, DefaultTracerService)

	newCtx := &DefaultContext{
		Context:  baseCtx,
		services: make(map[string]any),
		cfg:      config,
		logger:   logger,
		event:    event.NewManager[any](""),
		cancel:   cancel,
	}

	options = append(options, ContextWithTelemetry(), ContextWithExitFunc(func(ctx Context) error {
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

func (ctx *DefaultContext) ReplaceLogger(logger *Logger) {
	ctx.logger = logger
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
	return ctx.logger.wrap(ctx.logger.Logger.Named(name))
}

func (ctx *DefaultContext) WithLoggerOptions(opts ...zap.Option) *Logger {
	return ctx.logger.wrap(ctx.logger.Logger.WithOptions(opts...))
}

func (ctx *DefaultContext) WithLoggerLazy(opts ...zap.Field) *Logger {
	return ctx.logger.wrap(ctx.logger.Logger.WithLazy(opts...))
}

func (ctx *DefaultContext) WithLogger(opts ...zap.Field) *Logger {
	return ctx.logger.wrap(ctx.logger.Logger.With(opts...))
}

// Tracer helpers implementation
func (ctx *DefaultContext) WithTracer(service, subsystem string) Context {
	return ctx.WithRequestContext(WithTracerInfo(ctx.Context, service, subsystem))
}

func (ctx *DefaultContext) WithTracerService(service string) Context {
	return ctx.WithTracer(service, GetTracerSubsystem(ctx.Context))
}

func (ctx *DefaultContext) WithTracerSubsystem(subsystem string) Context {
	return ctx.WithTracer(GetTracerService(ctx.Context), subsystem)
}

func (ctx *DefaultContext) TraceMethod(name string, opts ...SpanOption) (context.Context, trace.Span) {
	return TraceMethod(ctx.Context, name, opts...)
}

// WithRequestContext returns a new Context that wraps the given request
// context, preserving request-scoped cancellation, deadlines, and trace
// metadata while retaining access to core services, config, DB, and logger.
func (ctx *DefaultContext) WithRequestContext(reqCtx context.Context) Context {
	return &DefaultContext{
		Context:      reqCtx,
		services:     ctx.services,
		cfg:          ctx.cfg,
		db:           ctx.db,
		exitCode:     ctx.exitCode,
		exitFuncs:    ctx.exitFuncs,
		startupFuncs: ctx.startupFuncs,
		event:        ctx.event,
		logger:       ctx.logger,
		cancel:       ctx.cancel,
	}
}

// Dedicated component tracer helpers implementation
func (ctx *DefaultContext) WithProtocolTracer(protocolName string) Context {
	return ctx.WithRequestContext(WithProtocolTracer(ctx.Context, protocolName))
}

func (ctx *DefaultContext) WithAPITracer(apiName string) Context {
	return ctx.WithRequestContext(WithAPITracer(ctx.Context, apiName))
}

func (ctx *DefaultContext) WithAPIExtensionTracer(extensionName string) Context {
	return ctx.WithRequestContext(WithAPIExtensionTracer(ctx.Context, extensionName))
}

func (ctx *DefaultContext) WithServiceTracer(serviceName string) Context {
	return ctx.WithRequestContext(WithServiceTracer(ctx.Context, serviceName))
}

// Subcomponent tracer helpers implementation
func (ctx *DefaultContext) WithProtocolSubcomponent(protocolName, subcomponentName string) Context {
	return ctx.WithRequestContext(WithProtocolSubcomponent(ctx.Context, protocolName, subcomponentName))
}

func (ctx *DefaultContext) WithAPISubcomponent(apiName, subcomponentName string) Context {
	return ctx.WithRequestContext(WithAPISubcomponent(ctx.Context, apiName, subcomponentName))
}

func (ctx *DefaultContext) WithAPIExtensionSubcomponent(extensionName, subcomponentName string) Context {
	return ctx.WithRequestContext(WithAPIExtensionSubcomponent(ctx.Context, extensionName, subcomponentName))
}

func (ctx *DefaultContext) WithServiceSubcomponent(serviceName, subcomponentName string) Context {
	return ctx.WithRequestContext(WithServiceSubcomponent(ctx.Context, serviceName, subcomponentName))
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

// findBaseComponentField searches for a BaseComponent field in the struct hierarchy,
// including embedded structs, using BFS traversal.
func findBaseComponentField(componentElem reflect.Value) reflect.Value {
	var baseComponentField reflect.Value

	queue := []reflect.Value{componentElem}
	for len(queue) > 0 {
		v := queue[0]
		queue = queue[1:]

		if v.Kind() == reflect.Ptr {
			if v.IsNil() {
				continue
			}
			v = v.Elem()
		}

		if v.Kind() != reflect.Struct {
			continue
		}

		f := v.FieldByName("BaseComponent")
		if f.IsValid() {
			baseComponentField = f
			break
		}

		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).Anonymous {
				queue = append(queue, v.Field(i))
			}
		}
	}

	return baseComponentField
}

// ensureBaseComponent initializes the embedded *BaseComponent field on a
// component if it is nil. Returns false if the component does not embed a
// BaseComponent (in which case the caller should skip wiring). Used by both
// ContextWithStartupComponent (during normal boot) and WireService (for CLI
// commands that wire a single service).
func ensureBaseComponent(component Component, ctx Context) bool {
	componentValue := reflect.ValueOf(component)
	if componentValue.Kind() != reflect.Ptr || componentValue.IsNil() {
		return false
	}

	componentElem := componentValue.Elem()
	if componentElem.Kind() != reflect.Struct {
		return false
	}

	baseComponentField := findBaseComponentField(componentElem)
	if !baseComponentField.IsValid() || !baseComponentField.Type().AssignableTo(reflect.TypeOf((*BaseComponent)(nil))) {
		return false
	}

	if baseComponentField.IsNil() {
		baseComponentField.Set(reflect.ValueOf(NewBaseComponent(ctx)))
	}

	return true
}

func ContextWithStartupComponent(component Component) ContextBuilderOption {
	return func(ctx Context) (Context, error) {
		ctx.OnStartup(func(startupCtx Context) error {
			if !ensureBaseComponent(component, startupCtx) {
				return nil
			}

			// Wire up the component with database and config
			component.SetDB(startupCtx.DB())
			component.SetConfig(startupCtx.Config())

			// Wire up the tracer and logger based on component type
			switch c := component.(type) {
			case APIExtension:
				tracedCtx := startupCtx.WithAPIExtensionTracer(c.TargetAPI() + "-" + component.ID())
				component.SetLogger(startupCtx.Logger())
				component.SetContext(tracedCtx)
			case API:
				tracedCtx := startupCtx.WithAPITracer(c.Name())
				component.SetLogger(startupCtx.APILogger(c))
				component.SetContext(tracedCtx)
			case Protocol:
				tracedCtx := startupCtx.WithProtocolTracer(c.Name())
				component.SetLogger(startupCtx.ProtocolLogger(c))
				component.SetContext(tracedCtx)
			case Service:
				tracedCtx := startupCtx.WithServiceTracer(c.ID())
				component.SetLogger(startupCtx.ServiceLogger(c))
				component.SetContext(tracedCtx)
			default:
				tracedCtx := startupCtx.WithTracerSubsystem(component.ID())
				component.SetLogger(startupCtx.Logger())
				component.SetContext(tracedCtx)
			}

			return nil
		})

		return ctx, nil
	}
}

// WireService manually wires a service component with config, DB, logger,
// and tracer — the same setup that ContextWithStartupComponent's OnStartup
// callback performs during startStartupFuncs. CLI commands that need a
// fully configured service without running all startup funcs call this
// after Init(), followed by the service's Init() if it implements ServiceInit.
func WireService(ctx Context, service Service) error {
	if !ensureBaseComponent(service, ctx) {
		return fmt.Errorf("service %s does not embed BaseComponent", service.ID())
	}

	service.SetDB(ctx.DB())
	service.SetConfig(ctx.Config())

	tracedCtx := ctx.WithServiceTracer(service.ID())
	service.SetLogger(ctx.ServiceLogger(service))
	service.SetContext(tracedCtx)

	return nil
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

// ServiceResult encapsulates the result of service retrieval with detailed status
type ServiceResult[T Service] struct {
	Service T
	Found   bool // true if service exists in context
	TypeOK  bool // true if service exists and type matches T
}

// getServiceTyped is the core service retrieval logic shared by all public helpers
func getServiceTyped[T Service](ctx Context, id string) ServiceResult[T] {
	var zero T
	svc := ctx.Service(id)
	if svc == nil {
		return ServiceResult[T]{Service: zero, Found: false, TypeOK: false}
	}

	typedSvc, ok := svc.(T)
	return ServiceResult[T]{
		Service: typedSvc,
		Found:   true,
		TypeOK:  ok,
	}
}

// GetServiceOptional returns a service if available, zero value otherwise
func GetServiceOptional[T Service](ctx Context, id string) T {
	result := getServiceTyped[T](ctx, id)
	if result.Found && !result.TypeOK {
		ctx.Logger().Debug("service type mismatch in optional lookup", zap.String("service", id))
	}
	return result.Service
}

// WithService executes a function with the service if available
func WithService[T Service](ctx Context, id string, fn func(T) error) error {
	result := getServiceTyped[T](ctx, id)
	if !result.TypeOK {
		if result.Found {
			ctx.Logger().Debug("service type mismatch, skipping callback", zap.String("service", id))
		}
		return nil
	}

	return fn(result.Service)
}

func GetService[T Service](ctx Context, id string) T {
	result := getServiceTyped[T](ctx, id)
	if !result.TypeOK {
		var zero T
		if !result.Found {
			ctx.Logger().Fatal("service not found", zap.String("service", id))
		} else {
			ctx.Logger().Fatal("service type mismatch", zap.String("service", id))
		}
		return zero
	}

	return result.Service
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
	ResetMetrics()
	ResetKeyIdentities()

	// Reset plugins
	pluginsMu.Lock()
	plugins = make(map[string]PluginInfo)
	pluginsOrderedMu.Lock()
	pluginsOrdered = nil
	pluginsOrderedMu.Unlock()
	pluginsMu.Unlock()
}

// DetachContext creates a new context that inherits the trace context from the input
// but is not canceled when the input context is canceled.
// This is useful for starting long-running workflows that should outlive HTTP requests
// or other short-lived contexts while preserving OpenTelemetry tracing.
func DetachContext(ctx context.Context) context.Context {
	return context.WithoutCancel(ctx)
}
