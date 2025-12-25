package core

import (
	"os"
	"time"

	"go.lumeweb.com/portal/build"
	"go.lumeweb.com/portal/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.34.0"
)

// ContextWithTelemetry sets up the OpenTelemetry pipeline based on observability config.
func ContextWithTelemetry() ContextBuilderOption {
	return func(ctx Context) (Context, error) {
		cfg := ctx.Config().Config().Core.Observability

		// Skip all setup if observability is disabled
		if !cfg.Enabled {
			return ctx, nil
		}

		// Set up propagator (TraceContext + Baggage)
		prop := newPropagator()
		otel.SetTextMapPropagator(prop)

		ctx.OnStartup(func(ctx Context) error {
			// Set up trace provider if tracing is enabled
			if cfg.Tracing.Enabled {
				tracerProvider, err := newTracerProvider(ctx)
				if err != nil {
					return err
				}

				otel.SetTracerProvider(tracerProvider)

				ctx.OnExit(func(exitCtx Context) error {
					return tracerProvider.Shutdown(exitCtx.GetContext())
				})
			}

			return nil
		})

		// Set up logger provider if logging is enabled
		if cfg.Logging.Enabled {
			loggerProvider, err := newLoggerProvider(ctx)
			if err != nil {
				return ctx, err
			}
			global.SetLoggerProvider(loggerProvider)

			ctx.ReplaceLogger(NewLogger(ctx.Config(), ctx.Logger()))

			ctx.OnExit(func(exitCtx Context) error {
				return loggerProvider.Shutdown(exitCtx.GetContext())
			})
		}

		return ctx, nil
	}
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

func newTracerProvider(ctx Context) (*trace.TracerProvider, error) {
	cfg := ctx.Config().Config().Core.Observability.Tracing

	var sampler trace.Sampler
	switch cfg.Sampler {
	case config.SamplerAlways:
		sampler = trace.AlwaysSample()
	case config.SamplerNever:
		sampler = trace.NeverSample()
	case config.SamplerTraceIDRatio:
		sampler = trace.TraceIDRatioBased(cfg.SamplerRatio)
	default:
		sampler = trace.AlwaysSample()
	}

	// Check if we should use OTLP exporter
	if cfg.Exporter == config.ExporterOTLP {
		traceExporter, err := otlptracehttp.New(ctx,
			otlptracehttp.WithEndpoint(cfg.OTLPEndpoint),
			otlptracehttp.WithInsecure(),
		)

		if err != nil {
			return nil, err
		}

		tracerProvider := trace.NewTracerProvider(
			trace.WithBatcher(traceExporter,
				trace.WithBatchTimeout(1*time.Second),
			),
			trace.WithSampler(sampler),
			trace.WithResource(newResource(ctx)),
		)
		return tracerProvider, nil
	}

	// No exporter - create provider with no exporters
	tracerProvider := trace.NewTracerProvider(
		trace.WithSampler(sampler),
		trace.WithResource(newResource(ctx)),
	)
	return tracerProvider, nil
}

func newLoggerProvider(ctx Context) (*log.LoggerProvider, error) {
	cfg := ctx.Config().Config().Core.Observability.Logging

	// Create the OTLP HTTP exporter if endpoint is configured
	var logExporter log.Exporter
	if cfg.OTLPEndpoint != "" {
		var err error
		logExporter, err = otlploghttp.New(ctx,
			otlploghttp.WithEndpoint(cfg.OTLPEndpoint),
			otlploghttp.WithInsecure(),
		)
		if err != nil {
			return nil, err
		}
	}

	loggerProvider := log.NewLoggerProvider(
		log.WithProcessor(log.NewBatchProcessor(logExporter)),
		log.WithResource(newResource(ctx)),
	)
	return loggerProvider, nil
}
func newResource(ctx Context) *resource.Resource {
	cfg := ctx.Config().Config().Core.Observability.Tracing
	hostname, _ := os.Hostname()

	serviceName := cfg.ServiceName
	if serviceName == "" {
		serviceName = hostname
	}

	return resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
		semconv.ServiceVersion(build.GetInfo().Version),
		semconv.HostName(hostname),
	)
}
