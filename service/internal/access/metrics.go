package access

import (
	"github.com/prometheus/client_golang/prometheus"
	"go.lumeweb.com/portal/core"
)

// Metric name constants for access service metrics
const (
	MetricRouteRegistered  = "route_registered_total"
	MetricRoleAssigned     = "role_assigned_total"
	MetricAccessChecked    = "access_checked_total"
	MetricPolicyExported   = "policy_exported_total"
	MetricDuration         = "duration_seconds"
	MetricFailed           = "failed_total"
)

// Metric label values
const (
	LabelOpRegisterRoute = "register_route"
	LabelOpAssignRole    = "assign_role"
	LabelOpCheckAccess   = "check_access"
	LabelOpExportPolicy  = "export_policy"
	LabelOpUnknown       = "unknown"

	LabelStatusError = "error"
)

// Global metric instances
var (
	RoutesRegistered  prometheus.CounterVec
	RolesAssigned     prometheus.CounterVec
	AccessChecked     prometheus.CounterVec
	PolicyExported    prometheus.CounterVec
	AccessDuration    prometheus.HistogramVec
	AccessFailed      prometheus.CounterVec
)

// init initializes all access metrics.
func init() {
	RoutesRegistered = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricRouteRegistered,
			Subsystem: core.ACCESS_SERVICE,
			Help:      "Total number of routes registered",
		},
		[]string{"operation"},
	)

	RolesAssigned = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricRoleAssigned,
			Subsystem: core.ACCESS_SERVICE,
			Help:      "Total number of roles assigned to users",
		},
		[]string{"operation"},
	)

	AccessChecked = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricAccessChecked,
			Subsystem: core.ACCESS_SERVICE,
			Help:      "Total number of access checks performed",
		},
		[]string{"operation"},
	)

	PolicyExported = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricPolicyExported,
			Subsystem: core.ACCESS_SERVICE,
			Help:      "Total number of user policy exports",
		},
		[]string{"operation"},
	)

	AccessDuration = *prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:      MetricDuration,
			Subsystem: core.ACCESS_SERVICE,
			Help:      "Time spent processing access service operations",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60, 300, 600},
		},
		[]string{"operation"},
	)

	AccessFailed = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricFailed,
			Subsystem: core.ACCESS_SERVICE,
			Help:      "Total number of access service operations that failed",
		},
		[]string{"operation"},
	)
}

// GetCollectors returns all metrics as collectors for registration.
func GetCollectors() []prometheus.Collector {
	return []prometheus.Collector{
		RoutesRegistered,
		RolesAssigned,
		AccessChecked,
		PolicyExported,
		AccessDuration,
		AccessFailed,
	}
}
