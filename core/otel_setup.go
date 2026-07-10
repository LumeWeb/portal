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

			var tp *sdktrace.TracerProvider
			var lp *sdklog.LoggerProvider

			if cfg.IsTracingEnabled() {
				traceExporter, err := otlptracegrpc.New(ctx.GetContext(), buildTraceExporterOptions(cfg.OTLP)...)
				if err != nil {
					return err
				}

				tp = sdktrace.NewTracerProvider(
					sdktrace.WithResource(res),
					sdktrace.WithBatcher(traceExporter,
						sdktrace.WithBatchTimeout(time.Duration(cfg.Tracing.BatchTimeout)*time.Second),
						sdktrace.WithMaxExportBatchSize(int(cfg.Tracing.MaxExportBatchSize)),
					),
					sdktrace.WithSampler(buildSampler(cfg.Tracing)),
				)
				otel.SetTracerProvider(tp)
			} else {
				tp = sdktrace.NewTracerProvider(sdktrace.WithSampler(sdktrace.NeverSample()))
				otel.SetTracerProvider(tp)
			}

			if cfg.IsLoggingEnabled() {
				logExporter, err := otlploggrpc.New(ctx.GetContext(), buildLogExporterOptions(cfg.OTLP)...)
				if err != nil {
					return errors.Join(err, tp.Shutdown(context.Background()))
				}

				lp = sdklog.NewLoggerProvider(
					sdklog.WithResource(res),
					sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
				)
				global.SetLoggerProvider(lp)
			}

			ctx.OnExit(func(exitCtx Context) error {
				var errs []error
				if tp != nil {
					errs = append(errs, tp.Shutdown(exitCtx.GetContext()))
				}
				if lp != nil {
					errs = append(errs, lp.Shutdown(exitCtx.GetContext()))
				}
				return errors.Join(errs...)
			})

			return nil
		})

		return ctx, nil
	}
}
