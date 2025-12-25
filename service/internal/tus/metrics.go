package tus

import (
	"github.com/prometheus/client_golang/prometheus"
	"go.lumeweb.com/portal/core"
)

// Metric name constants for TUS service metrics
const (
	MetricCreated  = "created_total"
	MetricDeleted  = "deleted_total"
	MetricQueried  = "queried_total"
	MetricListed   = "listed_total"
	MetricDuration = "duration_seconds"
	MetricFailed   = "failed_total"
)

// Metric label values
const (
	LabelOpCreate     = "create"
	LabelOpDelete     = "delete"
	LabelOpExists     = "exists"
	LabelOpHashExists = "hash_exists"
	LabelOpList       = "list"
	LabelOpProgress   = "progress"
	LabelOpCompleted  = "completed"
	LabelOpProcessing = "processing"
	LabelOpSetHash    = "set_hash"
	LabelOpUnknown    = "unknown"

	LabelStatusError = "error"
)

// Global metric instances
var (
	UploadsCreated prometheus.CounterVec
	UploadsDeleted prometheus.CounterVec
	UploadsQueried prometheus.CounterVec
	UploadsListed  prometheus.CounterVec
	UploadDuration prometheus.HistogramVec
	UploadFailed   prometheus.CounterVec
)

// init initializes all TUS metrics.
func init() {
	UploadsCreated = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricCreated,
			Subsystem: core.TUS_SERVICE,
			Help:      "Total number of TUS uploads created",
		},
		[]string{"operation"},
	)

	UploadsDeleted = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricDeleted,
			Subsystem: core.TUS_SERVICE,
			Help:      "Total number of TUS uploads deleted",
		},
		[]string{"operation"},
	)

	UploadsQueried = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricQueried,
			Subsystem: core.TUS_SERVICE,
			Help:      "Total number of TUS upload queries",
		},
		[]string{"operation"},
	)

	UploadsListed = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricListed,
			Subsystem: core.TUS_SERVICE,
			Help:      "Total number of TUS upload list operations",
		},
		[]string{"operation"},
	)

	UploadDuration = *prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:      MetricDuration,
			Subsystem: core.TUS_SERVICE,
			Help:      "Time spent processing TUS upload operations",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60, 300, 600},
		},
		[]string{"operation"},
	)

	UploadFailed = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricFailed,
			Subsystem: core.TUS_SERVICE,
			Help:      "Total number of TUS upload operations that failed",
		},
		[]string{"operation"},
	)
}

// GetCollectors returns all metrics as collectors for registration.
func GetCollectors() []prometheus.Collector {
	return []prometheus.Collector{
		UploadsCreated,
		UploadsDeleted,
		UploadsQueried,
		UploadsListed,
		UploadDuration,
		UploadFailed,
	}
}
