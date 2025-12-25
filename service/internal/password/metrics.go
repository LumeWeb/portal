package password

import (
	"github.com/prometheus/client_golang/prometheus"
	"go.lumeweb.com/portal/core"
)

// Metric name constants for password reset service metrics
const (
	MetricResetSent   = "reset_sent_total"
	MetricPasswordReset = "password_reset_total"
	MetricDuration    = "duration_seconds"
	MetricFailed      = "failed_total"
)

// Metric label values
const (
	LabelOpSendReset = "send_reset"
	LabelOpReset     = "reset"
	LabelOpUnknown   = "unknown"
)

// Global metric instances
var (
	ResetSent       prometheus.CounterVec
	PasswordReset   prometheus.CounterVec
	ResetDuration   prometheus.HistogramVec
	ResetFailed     prometheus.CounterVec
)

// init initializes all password reset metrics.
func init() {
	ResetSent = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricResetSent,
			Subsystem: core.PASSWORD_RESET_SERVICE,
			Help:      "Total number of password reset emails sent",
		},
		[]string{"operation"},
	)

	PasswordReset = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricPasswordReset,
			Subsystem: core.PASSWORD_RESET_SERVICE,
			Help:      "Total number of password resets completed",
		},
		[]string{"operation"},
	)

	ResetDuration = *prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:      MetricDuration,
			Subsystem: core.PASSWORD_RESET_SERVICE,
			Help:      "Time spent processing password reset operations",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60, 300, 600},
		},
		[]string{"operation"},
	)

	ResetFailed = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricFailed,
			Subsystem: core.PASSWORD_RESET_SERVICE,
			Help:      "Total number of password reset operations that failed",
		},
		[]string{"operation"},
	)
}

// GetCollectors returns all metrics as collectors for registration.
func GetCollectors() []prometheus.Collector {
	return []prometheus.Collector{
		ResetSent,
		PasswordReset,
		ResetDuration,
		ResetFailed,
	}
}
