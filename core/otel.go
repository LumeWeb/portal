package core

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

const (
	// DefaultTracerService is the default service name for tracers
	DefaultTracerService = "portal"

	// Tracer component types
	ComponentTypeProtocol     = "protocol"
	ComponentTypeAPI          = "api"
	ComponentTypeAPIExtension = "api-extension"
	ComponentTypeService      = "service"
)

// tracerServiceKey is the context key for storing the active tracer service name
type tracerServiceKey struct{}

// tracerSubsystemKey is the context key for storing subsystem information
type tracerSubsystemKey struct{}

// GetTracerService extracts the tracer service name from context
// Returns DefaultTracerService as default if no service is set
func GetTracerService(ctx context.Context) string {
	if ctx == nil {
		zap.L().Warn("nil context provided to GetTracerService, using default tracer service")
		return DefaultTracerService
	}
	if service, ok := ctx.Value(tracerServiceKey{}).(string); ok && service != "" {
		return service
	}
	return DefaultTracerService
}

// GetTracerSubsystem extracts the subsystem name from context
// Returns empty string if no subsystem is set
func GetTracerSubsystem(ctx context.Context) string {
	if ctx == nil {
		zap.L().Warn("nil context provided to GetTracerSubsystem, using empty subsystem")
		return ""
	}
	if subsystem, ok := ctx.Value(tracerSubsystemKey{}).(string); ok {
		return subsystem
	}
	return ""
}

// WithTracerService sets the tracer service name in context
func WithTracerService(ctx context.Context, service string) context.Context {
	return context.WithValue(ctx, tracerServiceKey{}, service)
}

// WithTracerSubsystem sets the subsystem name in context
func WithTracerSubsystem(ctx context.Context, subsystem string) context.Context {
	return context.WithValue(ctx, tracerSubsystemKey{}, subsystem)
}

// WithTracerInfo sets both service and subsystem in context
func WithTracerInfo(ctx context.Context, service, subsystem string) context.Context {
	ctx = WithTracerService(ctx, service)
	ctx = WithTracerSubsystem(ctx, subsystem)
	return ctx
}

// withComponentTracer sets the tracer for a component given its type and name.
// This is the DRY implementation used by all component-specific helpers.
func withComponentTracer(ctx context.Context, componentType, componentName string) context.Context {
	serviceName := DefaultTracerService + "/" + componentType + "/" + componentName
	return WithTracerService(ctx, serviceName)
}

// withComponentSubcomponent adds a subsystem to an existing component tracer.
// This is the DRY implementation used by all component subcomponent helpers.
func withComponentSubcomponent(ctx context.Context, componentType, componentName, subcomponentName string) context.Context {
	serviceName := DefaultTracerService + "/" + componentType + "/" + componentName
	return WithTracerInfo(ctx, serviceName, subcomponentName)
}

// Dedicated tracer helpers for component types

// WithProtocolTracer sets the tracer for a protocol component
func WithProtocolTracer(ctx context.Context, protocolName string) context.Context {
	return withComponentTracer(ctx, ComponentTypeProtocol, protocolName)
}

// WithAPITracer sets the tracer for an API component
func WithAPITracer(ctx context.Context, apiName string) context.Context {
	return withComponentTracer(ctx, ComponentTypeAPI, apiName)
}

// WithAPIExtensionTracer sets the tracer for an API extension component
func WithAPIExtensionTracer(ctx context.Context, extensionName string) context.Context {
	return withComponentTracer(ctx, ComponentTypeAPIExtension, extensionName)
}

// WithServiceTracer sets the tracer for a service component
func WithServiceTracer(ctx context.Context, serviceName string) context.Context {
	return withComponentTracer(ctx, ComponentTypeService, serviceName)
}

// Subcomponent tracer helpers - these add subsystem to existing service tracer

// WithProtocolSubcomponent adds a subsystem to an existing protocol tracer
func WithProtocolSubcomponent(ctx context.Context, protocolName, subcomponentName string) context.Context {
	return withComponentSubcomponent(ctx, ComponentTypeProtocol, protocolName, subcomponentName)
}

// WithAPISubcomponent adds a subsystem to an existing API tracer
func WithAPISubcomponent(ctx context.Context, apiName, subcomponentName string) context.Context {
	return withComponentSubcomponent(ctx, ComponentTypeAPI, apiName, subcomponentName)
}

// WithAPIExtensionSubcomponent adds a subsystem to an existing API extension tracer
func WithAPIExtensionSubcomponent(ctx context.Context, extensionName, subcomponentName string) context.Context {
	return withComponentSubcomponent(ctx, ComponentTypeAPIExtension, extensionName, subcomponentName)
}

// WithServiceSubcomponent adds a subsystem to an existing service tracer
func WithServiceSubcomponent(ctx context.Context, serviceName, subcomponentName string) context.Context {
	return withComponentSubcomponent(ctx, ComponentTypeService, serviceName, subcomponentName)
}

// getTracerName generates the appropriate tracer name from context
func getTracerName(ctx context.Context) string {
	if ctx == nil {
		zap.L().Warn("nil context provided to getTracerName, using default tracer name")
		return DefaultTracerService
	}
	service := GetTracerService(ctx)
	subsystem := GetTracerSubsystem(ctx)

	tracerName := service
	if subsystem != "" {
		tracerName = service + "/" + subsystem
	}

	return tracerName
}

// getTracer gets the appropriate tracer based on context
func getTracer(ctx context.Context) trace.Tracer {
	return otel.Tracer(getTracerName(ctx))
}

// EndSpanWithErr finishes the span and records error.
func EndSpanWithErr(
	span trace.Span,
	err error,
	options ...trace.SpanEndOption,
) {
	if err != nil {
		span.SetStatus(codes.Error, "error")
		span.RecordError(err)
	}

	span.End(options...)
}

// SpanConfig holds configuration for span creation
type SpanConfig struct {
	Name       string
	Attributes []attribute.KeyValue
	Kind       trace.SpanKind
	Links      []trace.Link
	NewRoot    bool
}

// SpanOption is a function that configures a SpanConfig
type SpanOption func(*SpanConfig)

// WithAttributes adds attributes to the span
func WithAttributes(attrs ...attribute.KeyValue) SpanOption {
	return func(c *SpanConfig) {
		c.Attributes = append(c.Attributes, attrs...)
	}
}

// WithLinks adds trace links to the span. Links connect independent traces
// (e.g., workflow steps) without forcing them into a single trace tree.
func WithLinks(links ...trace.Link) SpanOption {
	return func(c *SpanConfig) {
		c.Links = append(c.Links, links...)
	}
}

// WithSpanKind sets the span kind
func WithSpanKind(kind trace.SpanKind) SpanOption {
	return func(c *SpanConfig) {
		c.Kind = kind
	}
}

// WithNewRoot forces the span to start a new root trace, ignoring any
// parent span context in ctx. Use for entry points (cron jobs, event
// handlers) that have no meaningful parent but should have their own
// trace root.
func WithNewRoot() SpanOption {
	return func(c *SpanConfig) {
		c.NewRoot = true
	}
}

// StartSpan creates and starts a new span with the given configuration
func StartSpan(ctx context.Context, name string, opts ...SpanOption) (context.Context, trace.Span) {
	if ctx == nil {
		zap.L().Warn("nil context provided to StartSpan, using context.Background()")
		ctx = context.Background()
	}

	config := &SpanConfig{
		Name: name,
		Kind: trace.SpanKindInternal,
	}

	for _, opt := range opts {
		opt(config)
	}

	tracer := getTracer(ctx)
	spanOpts := []trace.SpanStartOption{
		trace.WithAttributes(config.Attributes...),
		trace.WithSpanKind(config.Kind),
	}
	if config.NewRoot {
		spanOpts = append(spanOpts, trace.WithNewRoot())
	}
	if len(config.Links) > 0 {
		spanOpts = append(spanOpts, trace.WithLinks(config.Links...))
	}
	ctx, span := tracer.Start(ctx, config.Name, spanOpts...)
	span.SetAttributes(attribute.String("trace.id", span.SpanContext().TraceID().String()))
	return ctx, span
}

// TraceMethod starts a new trace span for a method.
// Usage:
//
//	func (s *MyService) MyMethod(ctx context.Context, params MyParams) error {
//		ctx, span := TraceMethod(ctx, "my-service.MyMethod")
//		defer EndSpanWithErr(span, err)
//
//		// method implementation
//		return nil
//	}
//
//	func (s *MyService) VoidMethod(ctx context.Context, params MyParams) {
//		ctx, span := TraceMethod(ctx, "my-service.VoidMethod")
//		defer span.End()
//
//		// method implementation
//	}
func TraceMethod(ctx context.Context, name string, opts ...SpanOption) (context.Context, trace.Span) {
	return StartSpan(ctx, name, opts...)
}
