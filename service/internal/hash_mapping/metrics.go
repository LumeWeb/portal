package hash_mapping

import (
	"github.com/prometheus/client_golang/prometheus"
	"go.lumeweb.com/portal/core"
)

// Metric name constants for hash mapping service metrics
const (
	MetricStored   = "stored_total"
	MetricQueried  = "queried_total"
	MetricDeleted  = "deleted_total"
	MetricDuration = "duration_seconds"
	MetricFailed   = "failed_total"
)

// Metric label values
const (
	LabelOpStore         = "store"
	LabelOpGet           = "get"
	LabelOpGetReverse    = "get_reverse"
	LabelOpDelete        = "delete"
	LabelOpUnknown       = "unknown"
	LabelProtocolUnknown = "unknown"

	LabelStatusError = "error"
)

// Global metric instances
var (
	MappingsStored  prometheus.CounterVec
	MappingsQueried prometheus.CounterVec
	MappingsDeleted prometheus.CounterVec
	MappingDuration prometheus.HistogramVec
	MappingFailed   prometheus.CounterVec
)

// init initializes all hash mapping metrics.
func init() {
	MappingsStored = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricStored,
			Subsystem: core.HASH_MAPPING_SERVICE,
			Help:      "Total number of hash mappings stored",
		},
		[]string{"operation"},
	)

	MappingsQueried = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricQueried,
			Subsystem: core.HASH_MAPPING_SERVICE,
			Help:      "Total number of hash mappings queried",
		},
		[]string{"operation"},
	)

	MappingsDeleted = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricDeleted,
			Subsystem: core.HASH_MAPPING_SERVICE,
			Help:      "Total number of hash mappings deleted",
		},
		[]string{"operation"},
	)

	MappingDuration = *prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:      MetricDuration,
			Subsystem: core.HASH_MAPPING_SERVICE,
			Help:      "Time spent processing hash mapping operations",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60, 300, 600},
		},
		[]string{"operation"},
	)

	MappingFailed = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricFailed,
			Subsystem: core.HASH_MAPPING_SERVICE,
			Help:      "Total number of hash mapping operations that failed",
		},
		[]string{"operation"},
	)
}

// GetCollectors returns all metrics as collectors for registration.
func GetCollectors() []prometheus.Collector {
	return []prometheus.Collector{
		MappingsStored,
		MappingsQueried,
		MappingsDeleted,
		MappingDuration,
		MappingFailed,
	}
}
