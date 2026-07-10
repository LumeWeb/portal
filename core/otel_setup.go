package core

import (
	"context"
	"errors"
	"time"

	"go.lumeweb.com/portal/build"
	"go.lumeweb.com/portal/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

func buildResource(ctx context.Context, cfg config.ObservabilityConfig) (*resource.Resource, error) {
	opts := []resource.Option{
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
			semconv.ServiceVersionKey.String(build.GetInfo().Version),
		),
		resource.WithContainer(),
		resource.WithProcess(),
		resource.WithOS(),
		resource.WithHost(),
	}

	res, err := resource.New(ctx, opts...)
	if err != nil && res != nil {
		return res, nil
	}
	return res, err
}

func buildTraceExporterOptions(cfg config.OTLPConfig) []otlptracegrpc.Option {
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(cfg.Endpoint),
	}

	if cfg.AuthToken != "" {
		opts = append(opts, otlptracegrpc.WithHeaders(map[string]string{
			"Authorization": "Bearer " + cfg.AuthToken,
		}))
	}

	if cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	return opts
}

func buildLogExporterOptions(cfg config.OTLPConfig) []otlploggrpc.Option {
	opts := []otlploggrpc.Option{
		otlploggrpc.WithEndpoint(cfg.Endpoint),
	}

	if cfg.AuthToken != "" {
		opts = append(opts, otlploggrpc.WithHeaders(map[string]string{
			"Authorization": "Bearer " + cfg.AuthToken,
		}))
	}

	if cfg.Insecure {
		opts = append(opts, otlploggrpc.WithInsecure())
	}

	return opts
}

func buildSampler(cfg config.TracingConfig) sdktrace.Sampler {
	switch cfg.Sampler {
	case config.SamplerAlways:
		return sdktrace.AlwaysSample()
	case config.SamplerNever:
		return sdktrace.NeverSample()
	case config.SamplerTraceIDRatio:
		return sdktrace.TraceIDRatioBased(cfg.SamplerRatio)
	default:
		return sdktrace.AlwaysSample()
	}
}

func ContextWithTelemetry() ContextBuilderOption {
	return func(ctx Context) (Context, error) {
		// Create a TracerProvider with a DeferredSpanProcessor immediately so
		// that spans created during early boot (e.g. portal.boot) are captured
		// on a real provider. The OTLP exporter is attached later in the
		// startup func once config is available; until then, ended spans are
		// buffered in memory and flushed when the exporter is wired up.
		//
		// Always use AlwaysSample at builder time. Config may not be loaded
		// yet (NewContext is called before config.Init()), so reading
		// cfg.Enabled here would get defaults (Enabled=false). If we used
		// NeverSample based on that, the sampler would be frozen and no
		// spans would ever be recorded — even after the startup func reads
		// the real config and attaches an exporter. The sampler is immutable
		// after TracerProvider creation.
		//
		// If tracing is ultimately disabled, the startup func simply doesn't
		// attach an exporter. Buffered spans are then dropped on Shutdown,
		// which is the correct behavior.
		deferred := NewDeferredSpanProcessor()

		// Builder-time TP: used until the startup func replaces it with a
		// resource-corrected one. AlwaysSample so spans are recorded. No
		// resource yet (config not loaded).
		bootTP := sdktrace.NewTracerProvider(
			sdktrace.WithSpanProcessor(deferred),
			sdktrace.WithSampler(sdktrace.AlwaysSample()),
		)
		otel.SetTracerProvider(bootTP)

		// finalTP is the provider that OnExit should shut down. It starts as
		// bootTP and is replaced by the startup func if a new provider is
		// created with the correct resource.
		finalTP := bootTP

		// lp is set by the startup func if logging is enabled. It's declared
		// here so the OnExit closure (registered at builder time) can reference
		// it regardless of whether the startup func reaches the assignment.
		var lp *sdklog.LoggerProvider

		// Register OnExit at builder time, not inside the startup func.
		// If the startup func returns early (tracing disabled, no OTLP endpoint),
		// the TracerProvider still needs to be shut down to release resources
		// and flush any buffered spans in the DeferredSpanProcessor.
		ctx.OnExit(func(exitCtx Context) error {
			var errs []error
			errs = append(errs, finalTP.Shutdown(exitCtx.GetContext()))
			if lp != nil {
				errs = append(errs, lp.Shutdown(exitCtx.GetContext()))
			}
			return errors.Join(errs...)
		})

		ctx.OnStartup(func(ctx Context) error {
			cfg := ctx.Config().Config().Core.Observability

			if !cfg.Enabled {
				return nil
			}

			if cfg.OTLP.Endpoint == "" {
				return nil
			}

			res, err := buildResource(ctx.GetContext(), cfg)
			if err != nil {
				return err
			}

			if cfg.IsTracingEnabled() {
				// Recreate the TracerProvider with the correct resource.
				// The builder-time TP (bootTP) had no resource because config
				// wasn't loaded yet. The deferred processor is reused so
				// buffered boot spans are preserved. bootTP is NOT shut down
				// (that would shut down the shared deferred processor and lose
				// buffered spans). It will be GC'd.
				runtimeTP := sdktrace.NewTracerProvider(
					sdktrace.WithSpanProcessor(deferred),
					sdktrace.WithResource(res),
					sdktrace.WithSampler(buildSampler(cfg.Tracing)),
				)
				otel.SetTracerProvider(runtimeTP)
				finalTP = runtimeTP

				traceExporter, err := otlptracegrpc.New(ctx.GetContext(), buildTraceExporterOptions(cfg.OTLP)...)
				if err != nil {
					return err
				}

				// Attach the real exporter to the deferred processor. This
				// flushes any buffered spans (including portal.boot) to Tempo.
				deferred.SetExporter(traceExporter,
					sdktrace.WithBatchTimeout(time.Duration(cfg.Tracing.BatchTimeout)*time.Second),
					sdktrace.WithMaxExportBatchSize(int(cfg.Tracing.MaxExportBatchSize)),
				)
			}

			if cfg.IsLoggingEnabled() {
				logExporter, err := otlploggrpc.New(ctx.GetContext(), buildLogExporterOptions(cfg.OTLP)...)
				if err != nil {
					return errors.Join(err, finalTP.Shutdown(context.Background()))
				}

				lp = sdklog.NewLoggerProvider(
					sdklog.WithResource(res),
					sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
				)
				global.SetLoggerProvider(lp)
			}

			return nil
		})

		return ctx, nil
	}
}
