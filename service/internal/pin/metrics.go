package pin

import (
	"github.com/prometheus/client_golang/prometheus"
	"go.lumeweb.com/portal/core"
)

// Metric name constants for pin service metrics
const (
	MetricCreated       = "created_total"
	MetricDeleted       = "deleted_total"
	MetricQueried       = "queried_total"
	MetricUpdated       = "updated_total"
	MetricProtocolQueried = "protocol_queried_total"
	MetricDataQueried   = "data_queried_total"
	MetricDataUpdated   = "data_updated_total"
	MetricListed        = "listed_total"
	MetricChecked       = "checked_total"
	MetricDuration      = "duration_seconds"
	MetricFailed        = "failed_total"
)

// Metric label values
const (
	LabelOpCreate          = "create"
	LabelOpDelete          = "delete"
	LabelOpGet             = "get"
	LabelOpQuery           = "query"
	LabelOpUpdate          = "update"
	LabelOpGetProtocol     = "get_protocol"
	LabelOpQueryProtocol   = "query_protocol"
	LabelOpGetData         = "get_data"
	LabelOpUpdateData      = "update_data"
	LabelOpListAccount     = "list_account"
	LabelOpListUpload      = "list_upload"
	LabelOpCheckGlobal     = "check_global"
	LabelOpCheckUser       = "check_user"
	LabelOpUnknown         = "unknown"

	LabelStatusError       = "error"
)

// Global metric instances
var (
	PinsCreated         prometheus.CounterVec
	PinsDeleted         prometheus.CounterVec
	PinsQueried         prometheus.CounterVec
	PinsUpdated         prometheus.CounterVec
	ProtocolPinsQueried prometheus.CounterVec
	PinDataQueried      prometheus.CounterVec
	PinDataUpdated      prometheus.CounterVec
	PinsListed          prometheus.CounterVec
	PinsChecked         prometheus.CounterVec
	PinDuration         prometheus.HistogramVec
	PinFailed           prometheus.CounterVec
)

// init initializes all pin metrics.
func init() {
	PinsCreated = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricCreated,
			Subsystem: core.PIN_SERVICE,
			Help:      "Total number of pins created",
		},
		[]string{"operation"},
	)

	PinsDeleted = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricDeleted,
			Subsystem: core.PIN_SERVICE,
			Help:      "Total number of pins deleted",
		},
		[]string{"operation"},
	)

	PinsQueried = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricQueried,
			Subsystem: core.PIN_SERVICE,
			Help:      "Total number of pins queried",
		},
		[]string{"operation"},
	)

	PinsUpdated = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricUpdated,
			Subsystem: core.PIN_SERVICE,
			Help:      "Total number of pins updated",
		},
		[]string{"operation"},
	)

	ProtocolPinsQueried = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricProtocolQueried,
			Subsystem: core.PIN_SERVICE,
			Help:      "Total number of protocol pins queried",
		},
		[]string{"operation"},
	)

	PinDataQueried = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricDataQueried,
			Subsystem: core.PIN_SERVICE,
			Help:      "Total number of pin data queries",
		},
		[]string{"operation"},
	)

	PinDataUpdated = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricDataUpdated,
			Subsystem: core.PIN_SERVICE,
			Help:      "Total number of pin data updates",
		},
		[]string{"operation"},
	)

	PinsListed = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricListed,
			Subsystem: core.PIN_SERVICE,
			Help:      "Total number of pin list operations",
		},
		[]string{"operation"},
	)

	PinsChecked = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricChecked,
			Subsystem: core.PIN_SERVICE,
			Help:      "Total number of pin status checks",
		},
		[]string{"operation"},
	)

	PinDuration = *prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:      MetricDuration,
			Subsystem: core.PIN_SERVICE,
			Help:      "Time spent processing pin operations",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60, 300, 600},
		},
		[]string{"operation"},
	)

	PinFailed = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricFailed,
			Subsystem: core.PIN_SERVICE,
			Help:      "Total number of pin operations that failed",
		},
		[]string{"operation"},
	)
}

// GetCollectors returns all metrics as collectors for registration.
func GetCollectors() []prometheus.Collector {
	return []prometheus.Collector{
		PinsCreated,
		PinsDeleted,
		PinsQueried,
		PinsUpdated,
		ProtocolPinsQueried,
		PinDataQueried,
		PinDataUpdated,
		PinsListed,
		PinsChecked,
		PinDuration,
		PinFailed,
	}
}
