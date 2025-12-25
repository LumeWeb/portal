package core

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
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
	if service, ok := ctx.Value(tracerServiceKey{}).(string); ok && service != "" {
		return service
	}
	return DefaultTracerService
}

// GetTracerSubsystem extracts the subsystem name from context
// Returns empty string if no subsystem is set
func GetTracerSubsystem(ctx context.Context) string {
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

// Dedicated tracer helpers for component types

// WithProtocolTracer sets the tracer for a protocol component
func WithProtocolTracer(ctx context.Context, protocolName string) context.Context {
	serviceName := DefaultTracerService + "/" + ComponentTypeProtocol + "/" + protocolName
	return WithTracerService(ctx, serviceName)
}

// WithAPITracer sets the tracer for an API component
func WithAPITracer(ctx context.Context, apiName string) context.Context {
	serviceName := DefaultTracerService + "/" + ComponentTypeAPI + "/" + apiName
	return WithTracerService(ctx, serviceName)
}

// WithAPIExtensionTracer sets the tracer for an API extension component
func WithAPIExtensionTracer(ctx context.Context, extensionName string) context.Context {
	serviceName := DefaultTracerService + "/" + ComponentTypeAPIExtension + "/" + extensionName
	return WithTracerService(ctx, serviceName)
}

// WithServiceTracer sets the tracer for a service component
func WithServiceTracer(ctx context.Context, serviceName string) context.Context {
	serviceName = DefaultTracerService + "/" + ComponentTypeService + "/" + serviceName
	return WithTracerService(ctx, serviceName)
}

// Subcomponent tracer helpers - these add subsystem to existing service tracer

// WithProtocolSubcomponent adds a subsystem to an existing protocol tracer
func WithProtocolSubcomponent(ctx context.Context, protocolName, subcomponentName string) context.Context {
	serviceName := DefaultTracerService + "/" + ComponentTypeProtocol + "/" + protocolName
	return WithTracerInfo(ctx, serviceName, subcomponentName)
}

// WithAPISubcomponent adds a subsystem to an existing API tracer
func WithAPISubcomponent(ctx context.Context, apiName, subcomponentName string) context.Context {
	serviceName := DefaultTracerService + "/" + ComponentTypeAPI + "/" + apiName
	return WithTracerInfo(ctx, serviceName, subcomponentName)
}

// WithAPIExtensionSubcomponent adds a subsystem to an existing API extension tracer
func WithAPIExtensionSubcomponent(ctx context.Context, extensionName, subcomponentName string) context.Context {
	serviceName := DefaultTracerService + "/" + ComponentTypeAPIExtension + "/" + extensionName
	return WithTracerInfo(ctx, serviceName, subcomponentName)
}

// WithServiceSubcomponent adds a subsystem to an existing service tracer
func WithServiceSubcomponent(ctx context.Context, serviceName, subcomponentName string) context.Context {
	serviceName = DefaultTracerService + "/" + ComponentTypeService + "/" + serviceName
	return WithTracerInfo(ctx, serviceName, subcomponentName)
}

// getTracerName generates the appropriate tracer name from context
func getTracerName(ctx context.Context) string {
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
}

// SpanOption is a function that configures a SpanConfig
type SpanOption func(*SpanConfig)

// WithAttributes adds attributes to the span
func WithAttributes(attrs ...attribute.KeyValue) SpanOption {
	return func(c *SpanConfig) {
		c.Attributes = append(c.Attributes, attrs...)
	}
}

// WithSpanKind sets the span kind
func WithSpanKind(kind trace.SpanKind) SpanOption {
	return func(c *SpanConfig) {
		c.Kind = kind
	}
}

// StartSpan creates and starts a new span with the given configuration
func StartSpan(ctx context.Context, name string, opts ...SpanOption) (context.Context, trace.Span) {
	config := &SpanConfig{
		Name: name,
		Kind: trace.SpanKindInternal,
	}

	for _, opt := range opts {
		opt(config)
	}

	tracer := getTracer(ctx)
	return tracer.Start(ctx, config.Name,
		trace.WithAttributes(config.Attributes...),
		trace.WithSpanKind(config.Kind),
	)
}

// TraceMethod is a minimalist wrapper for auto-injecting spans into service methods with explicit naming
// Usage:
//
//	func (s *MyService) MyMethod(ctx context.Context, params MyParams) error {
//		ctx, span := TraceMethod(ctx, "my-service.MyMethod")
//		defer EndSpanWithErr(span, err)
//
//		// method implementation
//		return nil
//	}
func TraceMethod(ctx context.Context, name string, opts ...SpanOption) (context.Context, trace.Span) {
	return StartSpan(ctx, name, opts...)
}

// TraceMethodVoid is a specialized version for methods without return values with explicit naming
// Usage:
//
//	func (s *MyService) VoidMethod(ctx context.Context, params MyParams) {
//		ctx, end := TraceMethodVoid(ctx, "my-service.VoidMethod")
//		defer end(nil)
//
//		// method implementation
//	}
func TraceMethodVoid(ctx context.Context, name string, opts ...SpanOption) (context.Context, func(err error)) {
	ctx, span := StartSpan(ctx, name, opts...)

	return ctx, func(err error) {
		EndSpanWithErr(span, err)
	}
}
