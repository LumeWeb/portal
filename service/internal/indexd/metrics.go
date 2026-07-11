package indexd

import (
	"github.com/prometheus/client_golang/prometheus"
	"go.lumeweb.com/portal/core"
)

// Metric name constants for indexd SDK operations.
const (
	MetricDownloadDuration = "download_duration_seconds"
	MetricDownloadBytes    = "download_bytes_total"
	MetricDownloadErrors   = "download_errors_total"
	MetricDownloadActive   = "download_active"
	MetricUploadDuration   = "upload_duration_seconds"
	MetricUploadErrors     = "upload_errors_total"
)

// Global metric instances.
var (
	DownloadDuration prometheus.HistogramVec
	DownloadBytes    prometheus.CounterVec
	DownloadErrors   prometheus.CounterVec
	DownloadActive   prometheus.GaugeVec
	UploadDuration   prometheus.HistogramVec
	UploadErrors     prometheus.CounterVec
)

func init() {
	DownloadDuration = *prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: MetricDownloadDuration,
			Subsystem: core.RENTER_SERVICE,
			Help:      "Time spent reading data from the Sia SDK download reader (network I/O boundary)",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60},
		},
		[]string{"operation"},
	)

	DownloadBytes = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricDownloadBytes,
			Subsystem: core.RENTER_SERVICE,
			Help:      "Total bytes downloaded from the Sia SDK",
		},
		[]string{"operation"},
	)

	DownloadErrors = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricDownloadErrors,
			Subsystem: core.RENTER_SERVICE,
			Help:      "Total number of Sia SDK download errors",
		},
		[]string{"operation"},
	)

	DownloadActive = *prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:      MetricDownloadActive,
			Subsystem: core.RENTER_SERVICE,
			Help:      "Number of active Sia SDK downloads (readers not yet closed)",
		},
		[]string{"operation"},
	)

	UploadDuration = *prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:      MetricUploadDuration,
			Subsystem: core.RENTER_SERVICE,
			Help:      "Time spent uploading objects to the Sia SDK",
			Buckets:   prometheus.ExponentialBuckets(0.001, 2, 12),
		},
		[]string{"operation"},
	)

	UploadErrors = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricUploadErrors,
			Subsystem: core.RENTER_SERVICE,
			Help:      "Total number of Sia SDK upload errors",
		},
		[]string{"operation"},
	)
}

// Label values for operation types.
const (
	LabelOpDownload = "download"
	LabelOpUpload   = "upload"
)

// GetCollectors returns all metrics as collectors for registration.
func GetCollectors() []prometheus.Collector {
	return []prometheus.Collector{
		DownloadDuration,
		DownloadBytes,
		DownloadErrors,
		DownloadActive,
		UploadDuration,
		UploadErrors,
	}
}
