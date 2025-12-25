package otp

import (
	"github.com/prometheus/client_golang/prometheus"
	"go.lumeweb.com/portal/core"
)

// Metric name constants for OTP service metrics
const (
	MetricGenerated = "generated_total"
	MetricVerified  = "verified_total"
	MetricEnabled   = "enabled_total"
	MetricDisabled  = "disabled_total"
	MetricDuration  = "duration_seconds"
	MetricFailed    = "failed_total"
)

// Metric label values
const (
	LabelOpGenerate = "generate"
	LabelOpVerify   = "verify"
	LabelOpEnable   = "enable"
	LabelOpDisable  = "disable"
	LabelOpUnknown  = "unknown"

	LabelStatusError = "error"
)

// Global metric instances
var (
	OTPsGenerated prometheus.CounterVec
	OTPsVerified  prometheus.CounterVec
	OTPEnabled    prometheus.CounterVec
	OTPDisabled   prometheus.CounterVec
	OTPOperation  prometheus.HistogramVec
	OTPFailed     prometheus.CounterVec
)

// init initializes all OTP metrics.
func init() {
	OTPsGenerated = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricGenerated,
			Subsystem: core.OTP_SERVICE,
			Help:      "Total number of OTPs generated",
		},
		[]string{"operation"},
	)

	OTPsVerified = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricVerified,
			Subsystem: core.OTP_SERVICE,
			Help:      "Total number of OTPs verified",
		},
		[]string{"operation"},
	)

	OTPEnabled = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricEnabled,
			Subsystem: core.OTP_SERVICE,
			Help:      "Total number of OTPs enabled for accounts",
		},
		[]string{"operation"},
	)

	OTPDisabled = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricDisabled,
			Subsystem: core.OTP_SERVICE,
			Help:      "Total number of OTPs disabled for accounts",
		},
		[]string{"operation"},
	)

	OTPOperation = *prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:      MetricDuration,
			Subsystem: core.OTP_SERVICE,
			Help:      "Time spent processing OTP operations",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60, 300, 600},
		},
		[]string{"operation"},
	)

	OTPFailed = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricFailed,
			Subsystem: core.OTP_SERVICE,
			Help:      "Total number of OTP operations that failed",
		},
		[]string{"operation"},
	)
}

// GetCollectors returns all metrics as collectors for registration.
func GetCollectors() []prometheus.Collector {
	return []prometheus.Collector{
		OTPsGenerated,
		OTPsVerified,
		OTPEnabled,
		OTPDisabled,
		OTPOperation,
		OTPFailed,
	}
}
