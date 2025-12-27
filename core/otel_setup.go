package core

import (
	"os"

	"github.com/uptrace/uptrace-go/uptrace"
	"go.lumeweb.com/portal/build"
	"go.lumeweb.com/portal/config"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.34.0"
)

// ContextWithTelemetry sets up the OpenTelemetry pipeline based on observability config.
func ContextWithTelemetry() ContextBuilderOption {
	return func(ctx Context) (Context, error) {
		ctx.OnStartup(func(ctx Context) error {
			cfg := ctx.Config().Config().Core.Observability

			// Skip all setup if observability is disabled
			if !cfg.Enabled {
				return nil
			}

			// Validate DSN format
			if cfg.DSN == "" {
				return nil
			}

			// Build uptrace configuration options
			options := []uptrace.Option{
				uptrace.WithDSN(cfg.DSN),
				uptrace.WithServiceName(cfg.ServiceName),
				uptrace.WithServiceVersion(build.GetInfo().Version),
			}

			if hostname, err := os.Hostname(); err == nil {
				options = append(options, uptrace.WithResourceAttributes(semconv.HostName(hostname)))
			}

			// Configure logging if enabled
			if cfg.Logging.Enabled {
				options = append(options, uptrace.WithLoggingEnabled(true))
			}

			// Configure tracing if enabled
			var tracingOptions []uptrace.TracingOption
			if cfg.Tracing.Enabled {
				tracingOptions = append(tracingOptions, uptrace.WithTracingEnabled(true))

				// Configure sampler
				var sampler trace.Sampler
				switch cfg.Tracing.Sampler {
				case config.SamplerAlways:
					sampler = trace.AlwaysSample()
				case config.SamplerNever:
					sampler = trace.NeverSample()
				case config.SamplerTraceIDRatio:
					sampler = trace.TraceIDRatioBased(cfg.Tracing.SamplerRatio)
				default:
					sampler = trace.AlwaysSample()
				}
				tracingOptions = append(tracingOptions, uptrace.WithTraceSampler(sampler))
			} else {
				tracingOptions = append(tracingOptions, uptrace.WithTracingEnabled(false))
			}

			// Append tracing options
			for _, opt := range tracingOptions {
				options = append(options, opt)
			}

			// Configure OpenTelemetry using uptrace-go
			uptrace.ConfigureOpentelemetry(options...)

			ctx.OnExit(func(exitCtx Context) error {
				return uptrace.Shutdown(exitCtx.GetContext())
			})

			return nil
		})

		return ctx, nil
	}
}
