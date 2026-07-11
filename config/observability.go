package config

import (
	"net"

	z "github.com/Oudwins/zog"
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
	_ ConfigSchemaProvider = (*OTLPConfig)(nil)
	_ Defaults             = (*OTLPConfig)(nil)
	_ ConfigSchemaProvider = (*ObservabilityConfig)(nil)
	_ Defaults             = (*ObservabilityConfig)(nil)
	_ ConfigSchemaProvider = (*TracingConfig)(nil)
	_ Defaults             = (*TracingConfig)(nil)
	_ ConfigSchemaProvider = (*MetricsConfig)(nil)
	_ Defaults             = (*MetricsConfig)(nil)
	_ ConfigSchemaProvider = (*MetricsBasicAuthConfig)(nil)
	_ Defaults             = (*MetricsBasicAuthConfig)(nil)
	_ ConfigSchemaProvider = (*LoggingConfig)(nil)
	_ Defaults             = (*LoggingConfig)(nil)
)

type OTLPConfig struct {
	Endpoint  string `config:"endpoint"`
	AuthToken string `config:"auth_token"`
	Insecure  bool   `config:"insecure"`
}

func (o OTLPConfig) Schema() z.ZogSchema {
	return z.Struct(z.Shape{
		"Endpoint":  z.String(),
		"AuthToken": z.String(),
		"Insecure":  z.Bool(),
	}).TestFunc(func(data any, ctx z.Ctx) bool {
		c, ok := data.(*OTLPConfig)
		if !ok {
			return true
		}

		if c.Endpoint != "" {
			endpoint := c.Endpoint
			if _, _, err := net.SplitHostPort(endpoint); err != nil {
				endpoint = endpoint + ":4317"
				if _, _, err2 := net.SplitHostPort(endpoint); err2 != nil {
					ctx.AddIssue(ctx.Issue().SetMessage("endpoint format is invalid"))
					return false
				}
			}
		}

		return true
	})
}

func (o OTLPConfig) Defaults() map[string]any {
	return map[string]any{
		"Endpoint":  "",
		"AuthToken": "",
		"Insecure":  false,
	}
}

type ObservabilityConfig struct {
	Enabled     bool          `config:"enabled"`
	ServiceName string        `config:"service_name"`
	OTLP        OTLPConfig    `config:"otlp"`
	Tracing     TracingConfig `config:"tracing"`
	Metrics     MetricsConfig `config:"metrics"`
	Logging     LoggingConfig `config:"logging"`
}

type TracingConfig struct {
	Enabled            bool    `config:"enabled"`
	Sampler            string  `config:"sampler"`               // always, never, traceidratio
	SamplerRatio       float64 `config:"sampler_ratio"`         // 0.0-1.0, only for traceidratio
	BatchTimeout       uint    `config:"batch_timeout"`         // seconds between BSP flushes
	MaxExportBatchSize uint    `config:"max_export_batch_size"` // max spans per export batch
	MaxQueueSize       uint    `config:"max_queue_size"`        // max spans queued in memory before export
}

func (t TracingConfig) Schema() z.ZogSchema {
	return z.Struct(z.Shape{
		"Enabled": z.Bool(),
		"Sampler": z.String().
			OneOf([]string{SamplerAlways, SamplerNever, SamplerTraceIDRatio}, z.Message("sampler must be one of: always, never, traceidratio")),
		"SamplerRatio": z.Float64().
			GTE(0.0, z.Message("sampler_ratio must be >= 0.0")).
			LTE(1.0, z.Message("sampler_ratio must be <= 1.0")),
		"BatchTimeout": z.Uint().
			GT(0, z.Message("batch_timeout must be greater than 0")),
		"MaxExportBatchSize": z.Uint().
			GT(0, z.Message("max_export_batch_size must be greater than 0")),
		"MaxQueueSize": z.Uint().
			GT(0, z.Message("max_queue_size must be greater than 0")),
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
		"Enabled":            true,
		"Sampler":            SamplerAlways,
		"SamplerRatio":       1.0,
		"BatchTimeout":       uint(5),
		"MaxExportBatchSize": uint(512),
		"MaxQueueSize":       uint(8192),
	}
}

type MetricsConfig struct {
	Enabled         bool                   `config:"enabled"`
	Path            string                 `config:"path"`
	RefreshInterval uint                   `config:"refresh_interval"`
	BasicAuth       MetricsBasicAuthConfig `config:"basic_auth"`
}

type MetricsBasicAuthConfig struct {
	Password string `config:"password"`
}

func (m MetricsBasicAuthConfig) Schema() z.ZogSchema {
	return z.Struct(z.Shape{
		"Password": z.String(),
	})
}

func (m MetricsBasicAuthConfig) Defaults() map[string]any {
	return map[string]any{
		"Password": "",
	}
}

func (m MetricsBasicAuthConfig) IsEnabled() bool {
	return m.Password != ""
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
	return z.Struct(z.Shape{
		"Enabled":     z.Bool(),
		"ServiceName": z.String(),
		"OTLP":        o.OTLP.Schema(),
	}).TestFunc(func(data any, ctx z.Ctx) bool {
		c, ok := data.(*ObservabilityConfig)
		if !ok {
			return true
		}

		if c.Enabled && c.OTLP.Endpoint == "" {
			ctx.AddIssue(ctx.Issue().SetMessage("otlp.endpoint is required when observability is enabled"))
			return false
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
