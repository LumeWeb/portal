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
	Enabled                     bool           `config:"enabled"`
	MaxQueue                    uint           `config:"queue_limit"`
	DeadJobCheckIntervalMinutes uint           `config:"dead_job_check_interval_minutes"`
	Workflow                    WorkflowConfig `config:"workflow"`
}

func (w WorkflowConfig) Schema() z.ZogSchema {
	return z.Struct(z.Shape{
		"MaxRetries": z.Int().
			Default(5).
			GTE(0, z.Message("max_retries must be >= 0")),
		// InitialRetryDelay is time.Duration; mapstructure's
		// StringToTimeDurationHookFunc handles string→Duration conversion,
		// so no zog schema entry is needed (same pattern as
		// DatabaseConfig.ConnMaxLifetime).
		"RetryBackoffFactor": z.Float64().
			Default(2.0).
			GTE(1.0, z.Message("retry_backoff_factor must be >= 1.0")),
	})
}

func (w WorkflowConfig) Defaults() map[string]any {
	return map[string]any{
		"MaxRetries":         5,
		"InitialRetryDelay":  "30s",
		"RetryBackoffFactor": 2.0,
	}
}

func (c CronConfig) Schema() z.ZogSchema {
	return z.Struct(z.Shape{
		"Enabled": z.Bool().
			Default(true),
		"MaxQueue": z.Int().
			Default(50).
			GT(0, z.Message("queue limit must be greater than 0")),
		"DeadJobCheckIntervalMinutes": z.Int().
			Default(30).
			GT(0, z.Message("dead_job_check_interval_minutes must be greater than 0")),
	})
}

func (c CronConfig) Defaults() map[string]any {
	return map[string]any{
		"Enabled":                     true,
		"MaxQueue":                    50,
		"DeadJobCheckIntervalMinutes": 30,
		"Workflow": map[string]any{
			"MaxRetries":         5,
			"InitialRetryDelay":  "30s",
			"RetryBackoffFactor": 2.0,
		},
	}
}
