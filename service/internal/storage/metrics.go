package storage

import (
	"github.com/prometheus/client_golang/prometheus"
	"go.lumeweb.com/portal/core"
)

// Metric name constants for storage service metrics
const (
	MetricUploadDuration       = "upload_duration_seconds"
	MetricUploadBytes          = "upload_bytes_total"
	MetricUploadErrors         = "upload_errors_total"
	MetricDownloadDuration     = "download_duration_seconds"
	MetricDownloadBytes        = "download_bytes_total"
	MetricDownloadErrors       = "download_errors_total"
	MetricDeleteDuration       = "delete_duration_seconds"
	MetricDeleteErrors         = "delete_errors_total"
	MetricMultipartUploadParts = "multipart_upload_parts_total"
	MetricMultipartUploadErrors = "multipart_upload_errors_total"
	MetricS3UploadDuration   = "s3_upload_duration_seconds"
	MetricS3UploadBytes      = "s3_upload_bytes_total"
	MetricS3UploadErrors     = "s3_upload_errors_total"
	MetricS3DownloadDuration = "s3_download_duration_seconds"
	MetricS3DownloadBytes    = "s3_download_bytes_total"
	MetricS3DownloadErrors   = "s3_download_errors_total"
	MetricS3DeleteDuration   = "s3_delete_duration_seconds"
	MetricS3DeleteErrors     = "s3_delete_errors_total"
	MetricSiaUploadDuration  = "sia_upload_duration_seconds"
	MetricSiaUploadBytes     = "sia_upload_bytes_total"
	MetricSiaUploadErrors    = "sia_upload_errors_total"
	MetricSiaDownloadDuration = "sia_download_duration_seconds"
	MetricSiaDownloadBytes   = "sia_download_bytes_total"
	MetricSiaDownloadErrors  = "sia_download_errors_total"
	MetricSiaDeleteDuration  = "sia_delete_duration_seconds"
	MetricSiaDeleteErrors    = "sia_delete_errors_total"
	MetricActiveUploads       = "active_uploads"
	MetricStorageCacheHits    = "storage_cache_hits_total"
	MetricStorageCacheMisses  = "storage_cache_misses_total"
)

// Global metric instances (created once, reused everywhere)
var (
	UploadDuration       prometheus.Histogram
	UploadBytes          prometheus.Counter
	UploadErrors         prometheus.Counter
	DownloadDuration     prometheus.Histogram
	DownloadBytes        prometheus.Counter
	DownloadErrors       prometheus.Counter
	DeleteDuration       prometheus.Histogram
	DeleteErrors         prometheus.Counter
	MultipartUploadParts prometheus.Counter
	MultipartUploadErrors prometheus.Counter
	S3UploadDuration    prometheus.Histogram
	S3UploadBytes       prometheus.Counter
	S3UploadErrors      prometheus.Counter
	S3DownloadDuration  prometheus.Histogram
	S3DownloadBytes     prometheus.Counter
	S3DownloadErrors    prometheus.Counter
	S3DeleteDuration    prometheus.Histogram
	S3DeleteErrors      prometheus.Counter
	SiaUploadDuration   prometheus.Histogram
	SiaUploadBytes      prometheus.Counter
	SiaUploadErrors     prometheus.Counter
	SiaDownloadDuration prometheus.Histogram
	SiaDownloadBytes    prometheus.Counter
	SiaDownloadErrors   prometheus.Counter
	SiaDeleteDuration   prometheus.Histogram
	SiaDeleteErrors     prometheus.Counter
	ActiveUploads       prometheus.Gauge
	StorageCacheHits    prometheus.Counter
	StorageCacheMisses  prometheus.Counter
)

// init initializes all storage metrics.
// Called automatically when the package is imported.
func init() {
	UploadDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:      MetricUploadDuration,
		Subsystem: core.STORAGE_SERVICE,
		Help:      "Time spent uploading objects",
		Buckets:   []float64{0.1, 0.5, 1, 5, 10, 30, 60, 120, 300, 600},
	})
	UploadBytes = prometheus.NewCounter(prometheus.CounterOpts{
		Name:      MetricUploadBytes,
		Subsystem: core.STORAGE_SERVICE,
		Help:      "Total bytes uploaded",
	})
	UploadErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name:      MetricUploadErrors,
		Subsystem: core.STORAGE_SERVICE,
		Help:      "Total number of upload errors",
	})
	DownloadDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:      MetricDownloadDuration,
		Subsystem: core.STORAGE_SERVICE,
		Help:      "Time spent downloading objects",
		Buckets:   []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60},
	})
	DownloadBytes = prometheus.NewCounter(prometheus.CounterOpts{
		Name:      MetricDownloadBytes,
		Subsystem: core.STORAGE_SERVICE,
		Help:      "Total bytes downloaded",
	})
	DownloadErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name:      MetricDownloadErrors,
		Subsystem: core.STORAGE_SERVICE,
		Help:      "Total number of download errors",
	})
	DeleteDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:      MetricDeleteDuration,
		Subsystem: core.STORAGE_SERVICE,
		Help:      "Time spent deleting objects",
		Buckets:   []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 10},
	})
	DeleteErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name:      MetricDeleteErrors,
		Subsystem: core.STORAGE_SERVICE,
		Help:      "Total number of delete errors",
	})
	MultipartUploadParts = prometheus.NewCounter(prometheus.CounterOpts{
		Name:      MetricMultipartUploadParts,
		Subsystem: core.STORAGE_SERVICE,
		Help:      "Total number of multipart upload parts",
	})
	MultipartUploadErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name:      MetricMultipartUploadErrors,
		Subsystem: core.STORAGE_SERVICE,
		Help:      "Total number of multipart upload errors",
	})
	S3UploadDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:      MetricS3UploadDuration,
		Subsystem: core.STORAGE_SERVICE,
		Help:      "Time spent uploading objects to S3",
		Buckets:   []float64{0.1, 0.5, 1, 5, 10, 30, 60, 120, 300},
	})
	S3UploadBytes = prometheus.NewCounter(prometheus.CounterOpts{
		Name:      MetricS3UploadBytes,
		Subsystem: core.STORAGE_SERVICE,
		Help:      "Total bytes uploaded to S3",
	})
	S3UploadErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name:      MetricS3UploadErrors,
		Subsystem: core.STORAGE_SERVICE,
		Help:      "Total number of S3 upload errors",
	})
	S3DownloadDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:      MetricS3DownloadDuration,
		Subsystem: core.STORAGE_SERVICE,
		Help:      "Time spent downloading objects from S3",
		Buckets:   []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60},
	})
	S3DownloadBytes = prometheus.NewCounter(prometheus.CounterOpts{
		Name:      MetricS3DownloadBytes,
		Subsystem: core.STORAGE_SERVICE,
		Help:      "Total bytes downloaded from S3",
	})
	S3DownloadErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name:      MetricS3DownloadErrors,
		Subsystem: core.STORAGE_SERVICE,
		Help:      "Total number of S3 download errors",
	})
	S3DeleteDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:      MetricS3DeleteDuration,
		Subsystem: core.STORAGE_SERVICE,
		Help:      "Time spent deleting objects from S3",
		Buckets:   []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 10},
	})
	S3DeleteErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name:      MetricS3DeleteErrors,
		Subsystem: core.STORAGE_SERVICE,
		Help:      "Total number of S3 delete errors",
	})
	SiaUploadDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:      MetricSiaUploadDuration,
		Subsystem: core.STORAGE_SERVICE,
		Help:      "Time spent uploading objects to Sia",
		Buckets:   []float64{0.1, 0.5, 1, 5, 10, 30, 60, 120, 300, 600},
	})
	SiaUploadBytes = prometheus.NewCounter(prometheus.CounterOpts{
		Name:      MetricSiaUploadBytes,
		Subsystem: core.STORAGE_SERVICE,
		Help:      "Total bytes uploaded to Sia",
	})
	SiaUploadErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name:      MetricSiaUploadErrors,
		Subsystem: core.STORAGE_SERVICE,
		Help:      "Total number of Sia upload errors",
	})
	SiaDownloadDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:      MetricSiaDownloadDuration,
		Subsystem: core.STORAGE_SERVICE,
		Help:      "Time spent downloading objects from Sia",
		Buckets:   []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60},
	})
	SiaDownloadBytes = prometheus.NewCounter(prometheus.CounterOpts{
		Name:      MetricSiaDownloadBytes,
		Subsystem: core.STORAGE_SERVICE,
		Help:      "Total bytes downloaded from Sia",
	})
	SiaDownloadErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name:      MetricSiaDownloadErrors,
		Subsystem: core.STORAGE_SERVICE,
		Help:      "Total number of Sia download errors",
	})
	SiaDeleteDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:      MetricSiaDeleteDuration,
		Subsystem: core.STORAGE_SERVICE,
		Help:      "Time spent deleting objects from Sia",
		Buckets:   []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 10},
	})
	SiaDeleteErrors = prometheus.NewCounter(prometheus.CounterOpts{
		Name:      MetricSiaDeleteErrors,
		Subsystem: core.STORAGE_SERVICE,
		Help:      "Total number of Sia delete errors",
	})
	ActiveUploads = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:      MetricActiveUploads,
		Subsystem: core.STORAGE_SERVICE,
		Help:      "Number of currently active uploads",
	})
	StorageCacheHits = prometheus.NewCounter(prometheus.CounterOpts{
		Name:      MetricStorageCacheHits,
		Subsystem: core.STORAGE_SERVICE,
		Help:      "Total number of storage cache hits",
	})
	StorageCacheMisses = prometheus.NewCounter(prometheus.CounterOpts{
		Name:      MetricStorageCacheMisses,
		Subsystem: core.STORAGE_SERVICE,
		Help:      "Total number of storage cache misses",
	})
}

// GetCollectors returns all metrics as collectors for registration.
func GetCollectors() []prometheus.Collector {
	return []prometheus.Collector{
		UploadDuration,
		UploadBytes,
		UploadErrors,
		DownloadDuration,
		DownloadBytes,
		DownloadErrors,
		DeleteDuration,
		DeleteErrors,
		MultipartUploadParts,
		MultipartUploadErrors,
		S3UploadDuration,
		S3UploadBytes,
		S3UploadErrors,
		S3DownloadDuration,
		S3DownloadBytes,
		S3DownloadErrors,
		S3DeleteDuration,
		S3DeleteErrors,
		SiaUploadDuration,
		SiaUploadBytes,
		SiaUploadErrors,
		SiaDownloadDuration,
		SiaDownloadBytes,
		SiaDownloadErrors,
		SiaDeleteDuration,
		SiaDeleteErrors,
		ActiveUploads,
		StorageCacheHits,
		StorageCacheMisses,
	}
}
