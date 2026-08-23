package config

import (
	"time"

	z "github.com/Oudwins/zog"
	"go.lumeweb.com/configmanager"
)

var (
	_ configmanager.ConfigSchemaProvider = (*CronConfig)(nil)
	_ Defaults                           = (*CronConfig)(nil)
	_ configmanager.ConfigSchemaProvider = (*WorkflowConfig)(nil)
	_ Defaults                           = (*WorkflowConfig)(nil)
)

type WorkflowConfig struct {
	MaxRetries         int           `config:"max_retries"`
	InitialRetryDelay  time.Duration `config:"initial_retry_delay"`
	RetryBackoffFactor float64       `config:"retry_backoff_factor"`
}

type CronConfig struct {
	Enabled  bool           `config:"enabled"`
	MaxQueue uint           `config:"queue_limit"`
	Workflow WorkflowConfig `config:"workflow"`
}

func (w WorkflowConfig) Schema() z.ZogSchema {
	return z.Struct(z.Shape{
		"MaxRetries": z.Int().
			Default(5).
			GTE(0, z.Message("max_retries must be >= 0")),
		"InitialRetryDelay": z.String().
			Default("30s").
			TestFunc(func(v *string, ctx z.Ctx) bool {
				if _, err := time.ParseDuration(*v); err != nil {
					ctx.AddIssue(ctx.Issue().SetMessage("initial_retry_delay must be a valid duration"))
					return false
				}
				return true
			}),
		"RetryBackoffFactor": z.Float64().
			Default(2.0).
			GTE(1.0, z.Message("retry_backoff_factor must be >= 1.0")),
	})
}

func (w WorkflowConfig) Defaults() map[string]any {
	return map[string]any{
		"max_retries":          5,
		"initial_retry_delay": "30s",
		"retry_backoff_factor": 2.0,
	}
}

func (c CronConfig) Schema() z.ZogSchema {
	return z.Struct(z.Shape{
		"Enabled": z.Bool().
			Default(true),
		"MaxQueue": z.Int().
			Default(50).
			GT(0, z.Message("queue limit must be greater than 0")),
	})
}

func (c CronConfig) Defaults() map[string]any {
	return map[string]any{
		"enabled":   true,
		"queue_limit": 50,
		"workflow": map[string]any{
			"max_retries":          5,
			"initial_retry_delay":  "30s",
			"retry_backoff_factor": 2.0,
		},
	}
}
