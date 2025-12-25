package config

import (
	z "github.com/Oudwins/zog"
)

const (
	// Sampler constants
	SamplerAlways       = "always"
	SamplerNever        = "never"
	SamplerTraceIDRatio = "traceidratio"

	// Exporter constants
	ExporterOTLP = "otlp"
	ExporterNone = "none"

	// Default values
	DefaultServiceName  = "portal"
	DefaultOTLPEndpoint = "localhost:4317"
	DefaultMetricsPath  = "/metrics"
)

var (
	_ ConfigSchemaProvider = (*ObservabilityConfig)(nil)
	_ Defaults             = (*ObservabilityConfig)(nil)
	_ ConfigSchemaProvider = (*TracingConfig)(nil)
	_ Defaults             = (*TracingConfig)(nil)
	_ ConfigSchemaProvider = (*MetricsConfig)(nil)
	_ Defaults             = (*MetricsConfig)(nil)
	_ ConfigSchemaProvider = (*LoggingConfig)(nil)
	_ Defaults             = (*LoggingConfig)(nil)
)

type ObservabilityConfig struct {
	Enabled bool          `config:"enabled"`
	Tracing TracingConfig `config:"tracing"`
	Metrics MetricsConfig `config:"metrics"`
	Logging LoggingConfig `config:"logging"`
}

type TracingConfig struct {
	Enabled      bool    `config:"enabled"`
	ServiceName  string  `config:"service_name"`
	Sampler      string  `config:"sampler"`       // always, never, traceidratio
	SamplerRatio float64 `config:"sampler_ratio"` // 0.0-1.0, only for traceidratio
	Exporter     string  `config:"exporter"`      // otlp, stdout, none
	OTLPEndpoint string  `config:"otlp_endpoint"`
}

func (t TracingConfig) Schema() z.ZogSchema {
	return z.Struct(z.Shape{
		"Enabled":     z.Bool(),
		"ServiceName": z.String(),
		"Sampler": z.String().
			OneOf([]string{SamplerAlways, SamplerNever, SamplerTraceIDRatio}, z.Message("sampler must be one of: always, never, traceidratio")),
		"SamplerRatio": z.Float64().
			GTE(0.0, z.Message("sampler_ratio must be >= 0.0")).
			LTE(1.0, z.Message("sampler_ratio must be <= 1.0")),
		"Exporter": z.String().
			OneOf([]string{ExporterOTLP, ExporterNone}, z.Message("exporter must be one of: otlp, none")),
		"OTLPEndpoint": z.String(),
	}).TestFunc(func(data any, ctx z.Ctx) bool {
		c, ok := data.(*TracingConfig)
		if !ok {
			return true
		}

		// Validate sampler_ratio is only used with traceidratio sampler
		if c.Sampler != SamplerTraceIDRatio && c.SamplerRatio != 1.0 {
			ctx.AddIssue(ctx.Issue().SetMessage("sampler_ratio only applies when sampler is 'traceidratio'"))
			return false
		}

		// Validate otlp_endpoint is required when exporter is otlp
		if c.Exporter == ExporterOTLP && c.OTLPEndpoint == "" {
			ctx.AddIssue(ctx.Issue().SetMessage("otlp_endpoint is required when exporter is 'otlp'"))
			return false
		}

		return true
	})
}

func (t TracingConfig) Defaults() map[string]any {
	return map[string]any{
		"Enabled":      true,
		"ServiceName":  DefaultServiceName,
		"Sampler":      SamplerAlways,
		"SamplerRatio": 1.0,
		"Exporter":     ExporterOTLP,
		"OTLPEndpoint": DefaultOTLPEndpoint,
	}
}

type MetricsConfig struct {
	Enabled         bool   `config:"enabled"`
	Path            string `config:"path"`
	RefreshInterval uint   `config:"refresh_interval"`
}

func (m MetricsConfig) Schema() z.ZogSchema {
	return z.Struct(z.Shape{
		"Enabled": z.Bool(),
		"Path":    z.String(),
		"RefreshInterval": z.Uint().
			GT(0, z.Message("refresh_interval must be greater than 0")),
	})
}

func (m MetricsConfig) Defaults() map[string]any {
	return map[string]any{
		"Enabled":         true,
		"Path":            DefaultMetricsPath,
		"RefreshInterval": uint32(15),
	}
}

type LoggingConfig struct {
	Enabled bool   `config:"enabled"`
	Level   string `config:"level"`
}

func (l LoggingConfig) Schema() z.ZogSchema {
	return z.Struct(z.Shape{
		"Enabled": z.Bool(),
		"Level": z.String().
			OneOf([]string{"debug", "info", "warn", "error"}, z.Message("level must be one of: debug, info, warn, error")).
			Default("info"),
	})
}

func (l LoggingConfig) Defaults() map[string]any {
	return map[string]any{
		"Enabled": true,
		"Level":   "info",
	}
}

func (o ObservabilityConfig) Schema() z.ZogSchema {
	var tracingCfg TracingConfig
	var metricsCfg MetricsConfig
	var loggingCfg LoggingConfig

	return z.Struct(z.Shape{
		"Enabled": z.Bool().Default(false),
		"Tracing": tracingCfg.Schema(),
		"Metrics": metricsCfg.Schema(),
		"Logging": loggingCfg.Schema(),
	})
}

func (o ObservabilityConfig) Defaults() map[string]any {
	return map[string]any{
		"Enabled": false,
	}
}

// IsTracingEnabled returns true if observability and tracing are both enabled.
func (o ObservabilityConfig) IsTracingEnabled() bool {
	return o.Enabled && o.Tracing.Enabled
}

// IsMetricsEnabled returns true if observability and metrics are both enabled.
func (o ObservabilityConfig) IsMetricsEnabled() bool {
	return o.Enabled && o.Metrics.Enabled
}

// IsLoggingEnabled returns true if observability and logging are both enabled.
func (o ObservabilityConfig) IsLoggingEnabled() bool {
	return o.Enabled && o.Logging.Enabled
}
