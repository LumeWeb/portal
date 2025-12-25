package upload

import (
	"github.com/prometheus/client_golang/prometheus"
	"go.lumeweb.com/portal/core"
)

// Metric name constants for upload service metrics
const (
	MetricSaved    = "saved_total"
	MetricDeleted  = "deleted_total"
	MetricQueried  = "queried_total"
	MetricListed   = "listed_total"
	MetricDuration = "duration_seconds"
	MetricFailed   = "failed_total"
)

// Metric label values
const (
	LabelOpSave      = "save"
	LabelOpGet       = "get"
	LabelOpDelete    = "delete"
	LabelOpListAll   = "list_all"
	LabelOpGetByID   = "get_by_id"
	LabelOpUnknown   = "unknown"

	LabelStatusError = "error"
)

// Global metric instances
var (
	UploadsSaved    prometheus.CounterVec
	UploadsDeleted  prometheus.CounterVec
	UploadsQueried  prometheus.CounterVec
	UploadsListed   prometheus.CounterVec
	UploadDuration  prometheus.HistogramVec
	UploadFailed    prometheus.CounterVec
)

// init initializes all upload metrics.
func init() {
	UploadsSaved = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricSaved,
			Subsystem: core.UPLOAD_SERVICE,
			Help:      "Total number of uploads saved",
		},
		[]string{"operation"},
	)

	UploadsDeleted = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricDeleted,
			Subsystem: core.UPLOAD_SERVICE,
			Help:      "Total number of uploads deleted",
		},
		[]string{"operation"},
	)

	UploadsQueried = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricQueried,
			Subsystem: core.UPLOAD_SERVICE,
			Help:      "Total number of uploads queried",
		},
		[]string{"operation"},
	)

	UploadsListed = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricListed,
			Subsystem: core.UPLOAD_SERVICE,
			Help:      "Total number of upload list operations",
		},
		[]string{"operation"},
	)

	UploadDuration = *prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:      MetricDuration,
			Subsystem: core.UPLOAD_SERVICE,
			Help:      "Time spent processing upload operations",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60, 300, 600},
		},
		[]string{"operation"},
	)

	UploadFailed = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricFailed,
			Subsystem: core.UPLOAD_SERVICE,
			Help:      "Total number of upload operations that failed",
		},
		[]string{"operation"},
	)
}

// GetCollectors returns all metrics as collectors for registration.
func GetCollectors() []prometheus.Collector {
	return []prometheus.Collector{
		UploadsSaved,
		UploadsDeleted,
		UploadsQueried,
		UploadsListed,
		UploadDuration,
		UploadFailed,
	}
}
