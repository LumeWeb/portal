package content_scan

import (
	"github.com/prometheus/client_golang/prometheus"
	"go.lumeweb.com/portal/core"
)

// Metric name constants for content scanner service metrics
const (
	MetricScanned           = "scanned_total"
	MetricPassed            = "passed_total"
	MetricFailed            = "failed_total"
	MetricResultsQueried    = "results_queried_total"
	MetricScannerRegistered = "scanner_registered_total"
	MetricDuration          = "duration_seconds"
	MetricOperationFailed   = "operation_failed_total"
)

// Metric label values
const (
	LabelOpScan          = "scan"
	LabelOpGetResults    = "get_results"
	LabelOpGetResultById = "get_result_by_id"
	LabelOpRegister      = "register"
	LabelOpUnknown       = "unknown"

	LabelStatusPassed = "passed"
	LabelStatusFailed = "failed"

	LabelScannerUnknown = "unknown"
)

// Global metric instances
var (
	Scanned           prometheus.CounterVec
	ScansPassed       prometheus.CounterVec
	ScansFailed       prometheus.CounterVec
	ResultsQueried    prometheus.CounterVec
	ScannerRegistered prometheus.CounterVec
	ScanDuration      prometheus.HistogramVec
	OperationFailed   prometheus.CounterVec
)

// init initializes all content scanner metrics.
func init() {
	Scanned = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricScanned,
			Subsystem: core.CONTENT_SCANNER_SERVICE,
			Help:      "Total number of content scans performed",
		},
		[]string{"scanner_id"},
	)

	ScansPassed = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricPassed,
			Subsystem: core.CONTENT_SCANNER_SERVICE,
			Help:      "Total number of scans that passed",
		},
		[]string{"scanner_id"},
	)

	ScansFailed = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricFailed,
			Subsystem: core.CONTENT_SCANNER_SERVICE,
			Help:      "Total number of scans that failed",
		},
		[]string{"scanner_id"},
	)

	ResultsQueried = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricResultsQueried,
			Subsystem: core.CONTENT_SCANNER_SERVICE,
			Help:      "Total number of scan result queries",
		},
		[]string{"operation"},
	)

	ScannerRegistered = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricScannerRegistered,
			Subsystem: core.CONTENT_SCANNER_SERVICE,
			Help:      "Total number of scanners registered",
		},
		[]string{"scanner_id"},
	)

	ScanDuration = *prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:      MetricDuration,
			Subsystem: core.CONTENT_SCANNER_SERVICE,
			Help:      "Time spent processing content scanner operations",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60, 300, 600},
		},
		[]string{"operation"},
	)

	OperationFailed = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricOperationFailed,
			Subsystem: core.CONTENT_SCANNER_SERVICE,
			Help:      "Total number of content scanner operations that failed",
		},
		[]string{"operation"},
	)
}

// GetCollectors returns all metrics as collectors for registration.
func GetCollectors() []prometheus.Collector {
	return []prometheus.Collector{
		Scanned,
		ScansPassed,
		ScansFailed,
		ResultsQueried,
		ScannerRegistered,
		ScanDuration,
		OperationFailed,
	}
}
