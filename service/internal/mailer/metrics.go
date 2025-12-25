package mailer

import (
	"github.com/prometheus/client_golang/prometheus"
	"go.lumeweb.com/portal/core"
)

// Metric name constants for mailer service metrics
const (
	MetricSent      = "sent_total"
	MetricTemplates = "templates_total"
	MetricDuration  = "duration_seconds"
	MetricFailed    = "failed_total"
)

// Metric label values
const (
	LabelOpSend       = "send"
	LabelOpRegister   = "register"
	LabelOpUnknown    = "unknown"

	LabelStatusError = "error"
)

// Global metric instances
var (
	EmailsSent       prometheus.CounterVec
	TemplatesTotal   prometheus.CounterVec
	MailerDuration   prometheus.HistogramVec
	MailerFailed     prometheus.CounterVec
)

// init initializes all mailer metrics.
func init() {
	EmailsSent = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricSent,
			Subsystem: core.MAILER_SERVICE,
			Help:      "Total number of emails sent",
		},
		[]string{"operation"},
	)

	TemplatesTotal = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricTemplates,
			Subsystem: core.MAILER_SERVICE,
			Help:      "Total number of email templates registered",
		},
		[]string{"operation"},
	)

	MailerDuration = *prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:      MetricDuration,
			Subsystem: core.MAILER_SERVICE,
			Help:      "Time spent processing mailer operations",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60, 300, 600},
		},
		[]string{"operation"},
	)

	MailerFailed = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricFailed,
			Subsystem: core.MAILER_SERVICE,
			Help:      "Total number of mailer operations that failed",
		},
		[]string{"operation"},
	)
}

// GetCollectors returns all metrics as collectors for registration.
func GetCollectors() []prometheus.Collector {
	return []prometheus.Collector{
		EmailsSent,
		TemplatesTotal,
		MailerDuration,
		MailerFailed,
	}
}
