package config

import (
	z "github.com/Oudwins/zog"
	"github.com/uptrace/uptrace-go/uptrace"
)

const (
	// Sampler constants
	SamplerAlways       = "always"
	SamplerNever        = "never"
	SamplerTraceIDRatio = "traceidratio"

	// Default values
	DefaultServiceName = "portal"
	DefaultMetricsPath = "/metrics"
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
	Enabled     bool          `config:"enabled"`
	ServiceName string        `config:"service_name"`
	DSN         string        `config:"dsn"`
	Tracing     TracingConfig `config:"tracing"`
	Metrics     MetricsConfig `config:"metrics"`
	Logging     LoggingConfig `config:"logging"`
}

type TracingConfig struct {
	Enabled      bool    `config:"enabled"`
	Sampler      string  `config:"sampler"`       // always, never, traceidratio
	SamplerRatio float64 `config:"sampler_ratio"` // 0.0-1.0, only for traceidratio
}

func (t TracingConfig) Schema() z.ZogSchema {
	return z.Struct(z.Shape{
		"Enabled": z.Bool(),
		"Sampler": z.String().
			OneOf([]string{SamplerAlways, SamplerNever, SamplerTraceIDRatio}, z.Message("sampler must be one of: always, never, traceidratio")),
		"SamplerRatio": z.Float64().
			GTE(0.0, z.Message("sampler_ratio must be >= 0.0")).
			LTE(1.0, z.Message("sampler_ratio must be <= 1.0")),
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

		return true
	})
}

func (t TracingConfig) Defaults() map[string]any {
	return map[string]any{
		"Enabled":      true,
		"Sampler":      SamplerAlways,
		"SamplerRatio": 1.0,
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
		"RefreshInterval": uint(15),
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
			OneOf([]string{"debug", "info", "warn", "error"}, z.Message("level must be one of: debug, info, warn, error")),
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
		"Enabled":     z.Bool(),
		"ServiceName": z.String(),
		"DSN":         z.String(),
		"Tracing":     tracingCfg.Schema(),
		"Metrics":     metricsCfg.Schema(),
		"Logging":     loggingCfg.Schema(),
	}).TestFunc(func(data any, ctx z.Ctx) bool {
		c, ok := data.(*ObservabilityConfig)
		if !ok {
			return true
		}

		// Validate DSN format if provided (contains secrets)
		if c.DSN != "" {
			// Use uptrace's ParseDSN to validate format without exposing values
			_, err := uptrace.ParseDSN(c.DSN)
			if err != nil {
				ctx.AddIssue(ctx.Issue().SetMessage("DSN format is invalid"))
				return false
			}
		}

		return true
	})
}

func (o ObservabilityConfig) Defaults() map[string]any {
	return map[string]any{
		"Enabled":     false,
		"ServiceName": DefaultServiceName,
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
