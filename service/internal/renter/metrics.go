package renter

import (
	"github.com/prometheus/client_golang/prometheus"
	"go.lumeweb.com/portal/core"
)

// Metric name constants for renter service metrics
const (
	MetricBucketOperationsTotal   = "bucket_operations_total"
	MetricBucketOperationDuration = "bucket_operation_duration_seconds"
	MetricObjectOperationsTotal    = "object_operations_total"
	MetricObjectOperationDuration = "object_operation_duration_seconds"
	MetricApiRequestsTotal        = "api_requests_total"
	MetricApiLatency             = "api_latency_seconds"
	MetricGougingCompliance      = "gouging_compliance_status"
	MetricRecommendedFee          = "recommended_fee_hastings"
)

// Metric label values
const (
	LabelClientTypeBus      = "bus"
	LabelClientTypeWorker   = "worker"
	LabelClientTypeAutopilot = "autopilot"

	LabelOperationCreate    = "create"
	LabelOperationCheck     = "check"
	LabelOperationUpload    = "upload"
	LabelOperationDownload  = "download"
	LabelOperationDelete    = "delete"

	LabelStatusSuccess = "success"
	LabelStatusError   = "error"

	LabelEndpointBucket            = "bucket"
	LabelEndpointObject            = "object"
	LabelEndpointObjectMetadata    = "object_metadata"
	LabelEndpointMultipartUpload   = "multipart_upload"
	LabelEndpointHost             = "host"
	LabelEndpointConsensus        = "consensus"
	LabelEndpointFee              = "fee"
	LabelEndpointGouging          = "gouging"
	LabelEndpointUploadSettings   = "upload_settings"
	LabelEndpointMultipartUploadPart = "multipart_upload_part"
)

// Global metric instances
var (
	BucketOperationsTotal   prometheus.CounterVec
	BucketOperationDuration prometheus.HistogramVec
	ObjectOperationsTotal    prometheus.CounterVec
	ObjectOperationDuration  prometheus.HistogramVec
	ApiRequestsTotal        prometheus.CounterVec
	ApiLatency             prometheus.HistogramVec
	GougingCompliance      prometheus.Gauge
	RecommendedFee          prometheus.Gauge
)

// init initializes all renter metrics.
func init() {
	BucketOperationsTotal = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricBucketOperationsTotal,
			Subsystem: core.RENTER_SERVICE,
			Help:      "Total number of bucket operations",
		},
		[]string{"operation", "status"},
	)

	BucketOperationDuration = *prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:      MetricBucketOperationDuration,
			Subsystem: core.RENTER_SERVICE,
			Help:      "Time spent on bucket operations",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30},
		},
		[]string{"operation"},
	)

	ObjectOperationsTotal = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricObjectOperationsTotal,
			Subsystem: core.RENTER_SERVICE,
			Help:      "Total number of object operations",
		},
		[]string{"operation", "status"},
	)

	ObjectOperationDuration = *prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:      MetricObjectOperationDuration,
			Subsystem: core.RENTER_SERVICE,
			Help:      "Time spent on object operations",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60, 300, 600},
		},
		[]string{"operation"},
	)

	ApiRequestsTotal = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricApiRequestsTotal,
			Subsystem: core.RENTER_SERVICE,
			Help:      "Total number of API requests",
		},
		[]string{"client_type", "endpoint", "status"},
	)

	ApiLatency = *prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:      MetricApiLatency,
			Subsystem: core.RENTER_SERVICE,
			Help:      "API request latency",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30},
		},
		[]string{"client_type", "endpoint"},
	)

	GougingCompliance = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:      MetricGougingCompliance,
		Subsystem: core.RENTER_SERVICE,
		Help:      "Gouging compliance status (1 = compliant, 0 = non-compliant)",
	})

	RecommendedFee = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:      MetricRecommendedFee,
		Subsystem: core.RENTER_SERVICE,
		Help:      "Current recommended network fee in hastings",
	})
}

// GetCollectors returns all metrics as collectors for registration.
func GetCollectors() []prometheus.Collector {
	return []prometheus.Collector{
		BucketOperationsTotal,
		BucketOperationDuration,
		ObjectOperationsTotal,
		ObjectOperationDuration,
		ApiRequestsTotal,
		ApiLatency,
		GougingCompliance,
		RecommendedFee,
	}
}

// TrackApiCall wraps an API call function to track request count and latency.
// clientType: bus, worker, or autopilot
// endpoint: specific API endpoint name
// f: API function to call, returning an error
func TrackApiCall(clientType, endpoint string, f func() error) error {
	start := prometheus.NewTimer(ApiLatency.WithLabelValues(clientType, endpoint))
	defer start.ObserveDuration()

	err := f()

	if err != nil {
		ApiRequestsTotal.WithLabelValues(clientType, endpoint, LabelStatusError).Inc()
		return err
	}

	ApiRequestsTotal.WithLabelValues(clientType, endpoint, LabelStatusSuccess).Inc()
	return nil
}

// TrackApiCallResult wraps an API call function that returns (T, error) to track request count and latency.
func TrackApiCallResult[T any](clientType, endpoint string, f func() (T, error)) (T, error) {
	start := prometheus.NewTimer(ApiLatency.WithLabelValues(clientType, endpoint))
	defer start.ObserveDuration()

	result, err := f()

	if err != nil {
		ApiRequestsTotal.WithLabelValues(clientType, endpoint, LabelStatusError).Inc()
		return result, err
	}

	ApiRequestsTotal.WithLabelValues(clientType, endpoint, LabelStatusSuccess).Inc()
	return result, nil
}
