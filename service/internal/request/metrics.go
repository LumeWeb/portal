package request

import (
	"github.com/prometheus/client_golang/prometheus"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
)

// Metric name constants for request service metrics
const (
	MetricCreated            = "created_total"
	MetricCompleted          = "completed_total"
	MetricFailed             = "failed_total"
	MetricDeleted            = "deleted_total"
	MetricUpdated            = "updated_total"
	MetricDuplicate          = "duplicate_total"
	MetricValidationFailed   = "validation_failed_total"
	MetricQueryTotal         = "query_total"
	MetricDuration           = "duration_seconds"
	MetricByStatus          = "by_status"
	MetricByOperation       = "by_operation"
	MetricByProtocol        = "by_protocol"
)

// Metric label values
const (
	LabelStatusPending    = models.RequestStatusPending
	LabelStatusProcessing = models.RequestStatusProcessing
	LabelStatusCompleted  = models.RequestStatusCompleted
	LabelStatusFailed     = models.RequestStatusFailed
	LabelStatusDuplicate  = models.RequestStatusDuplicate

	LabelProtocolUnknown = "unknown"

	LabelOperationCreate   = "create"
	LabelOperationUpdate   = "update"
	LabelOperationDelete   = "delete"
	LabelOperationQuery    = "query"
	LabelOperationExecute  = "execute"
	LabelOperationUnknown  = "unknown"

	LabelQueryTypeGet        = "get"
	LabelQueryTypeQuery      = "query"
	LabelQueryTypeList       = "list"
	LabelQueryTypeByHash     = "by_hash"
	LabelQueryTypeByUser     = "by_user"
	LabelQueryTypeByStatus   = "by_status"
	LabelQueryTypeGetStatus  = "get_status"
)

// Global metric instances
var (
	RequestsCreated         prometheus.CounterVec
	RequestsCompleted       prometheus.CounterVec
	RequestsFailed          prometheus.CounterVec
	RequestsDeleted         prometheus.CounterVec
	RequestsUpdated         prometheus.CounterVec
	RequestsDuplicate       prometheus.CounterVec
	RequestsValidationFailed prometheus.CounterVec
	RequestsQueryTotal      prometheus.CounterVec
	RequestDuration        prometheus.HistogramVec
	RequestsByStatus       prometheus.GaugeVec
	RequestsByOperation    prometheus.GaugeVec
	RequestsByProtocol     prometheus.GaugeVec
)

// init initializes all request metrics.
func init() {
	RequestsCreated = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricCreated,
			Subsystem: core.REQUEST_SERVICE,
			Help:      "Total number of requests created",
		},
		[]string{"protocol", "operation"},
	)

	RequestsCompleted = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricCompleted,
			Subsystem: core.REQUEST_SERVICE,
			Help:      "Total number of requests completed successfully",
		},
		[]string{"protocol", "operation"},
	)

	RequestsFailed = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricFailed,
			Subsystem: core.REQUEST_SERVICE,
			Help:      "Total number of requests that failed",
		},
		[]string{"protocol", "operation"},
	)

	RequestsDeleted = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricDeleted,
			Subsystem: core.REQUEST_SERVICE,
			Help:      "Total number of requests deleted",
		},
		[]string{"protocol", "operation"},
	)

	RequestsUpdated = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricUpdated,
			Subsystem: core.REQUEST_SERVICE,
			Help:      "Total number of requests updated",
		},
		[]string{"protocol", "operation"},
	)

	RequestsDuplicate = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricDuplicate,
			Subsystem: core.REQUEST_SERVICE,
			Help:      "Total number of duplicate requests detected",
		},
		[]string{"protocol", "operation"},
	)

	RequestsValidationFailed = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricValidationFailed,
			Subsystem: core.REQUEST_SERVICE,
			Help:      "Total number of requests that failed validation",
		},
		[]string{"protocol", "operation"},
	)

	RequestsQueryTotal = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricQueryTotal,
			Subsystem: core.REQUEST_SERVICE,
			Help:      "Total number of request queries",
		},
		[]string{"query_type"},
	)

	RequestDuration = *prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:      MetricDuration,
			Subsystem: core.REQUEST_SERVICE,
			Help:      "Time spent processing requests",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60, 300, 600},
		},
		[]string{"operation"},
	)

	RequestsByStatus = *prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:      MetricByStatus,
			Subsystem: core.REQUEST_SERVICE,
			Help:      "Current number of requests by status",
		},
		[]string{"status"},
	)

	RequestsByOperation = *prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:      MetricByOperation,
			Subsystem: core.REQUEST_SERVICE,
			Help:      "Current number of requests by operation type",
		},
		[]string{"operation"},
	)

	RequestsByProtocol = *prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:      MetricByProtocol,
			Subsystem: core.REQUEST_SERVICE,
			Help:      "Current number of requests by protocol",
		},
		[]string{"protocol"},
	)
}

// GetCollectors returns all metrics as collectors for registration.
func GetCollectors() []prometheus.Collector {
	return []prometheus.Collector{
		RequestsCreated,
		RequestsCompleted,
		RequestsFailed,
		RequestsDeleted,
		RequestsUpdated,
		RequestsDuplicate,
		RequestsValidationFailed,
		RequestsQueryTotal,
		RequestDuration,
		RequestsByStatus,
		RequestsByOperation,
		RequestsByProtocol,
	}
}
