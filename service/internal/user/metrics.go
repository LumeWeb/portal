package user

import (
	"github.com/prometheus/client_golang/prometheus"
	"go.lumeweb.com/portal/core"
)

// Metric name constants for user service metrics
const (
	MetricCreated               = "created_total"
	MetricUpdated               = "updated_total"
	MetricDeleted               = "deleted_total"
	MetricExistsQueried         = "exists_queried_total"
	MetricEmailVerificationSent = "email_verification_sent_total"
	MetricEmailVerified         = "email_verified_total"
	MetricDeletionRequested     = "deletion_requested_total"
	MetricPublicKeyAdded        = "public_key_added_total"
	MetricDuration              = "duration_seconds"
	MetricFailed                = "failed_total"
)

// Metric label values
const (
	LabelOpCreate           = "create"
	LabelOpUpdate           = "update"
	LabelOpDelete           = "delete"
	LabelOpCheckExists      = "check_exists"
	LabelOpSendVerification = "send_verification"
	LabelOpVerifyEmail      = "verify_email"
	LabelOpRequestDeletion  = "request_deletion"
	LabelOpListPending      = "list_pending"
	LabelOpAddPubkey        = "add_pubkey"
	LabelOpGetAccount       = "get_account"
	LabelOpUnknown          = "unknown"

	LabelStatusError = "error"
)

// Global metric instances
var (
	AccountsCreated          prometheus.CounterVec
	AccountsUpdated          prometheus.CounterVec
	AccountsDeleted          prometheus.CounterVec
	AccountsExistsQueried    prometheus.CounterVec
	EmailVerificationSent    prometheus.CounterVec
	EmailVerified            prometheus.CounterVec
	AccountDeletionRequested prometheus.CounterVec
	PublicKeyAdded           prometheus.CounterVec
	UserOperationDuration    prometheus.HistogramVec
	UserOperationFailed      prometheus.CounterVec
)

// init initializes all user metrics.
func init() {
	AccountsCreated = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricCreated,
			Subsystem: core.USER_SERVICE,
			Help:      "Total number of accounts created",
		},
		[]string{"operation"},
	)

	AccountsUpdated = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricUpdated,
			Subsystem: core.USER_SERVICE,
			Help:      "Total number of accounts updated",
		},
		[]string{"operation"},
	)

	AccountsDeleted = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricDeleted,
			Subsystem: core.USER_SERVICE,
			Help:      "Total number of accounts deleted",
		},
		[]string{"operation"},
	)

	AccountsExistsQueried = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricExistsQueried,
			Subsystem: core.USER_SERVICE,
			Help:      "Total number of account existence checks",
		},
		[]string{"operation"},
	)

	EmailVerificationSent = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricEmailVerificationSent,
			Subsystem: core.USER_SERVICE,
			Help:      "Total number of email verification emails sent",
		},
		[]string{"operation"},
	)

	EmailVerified = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricEmailVerified,
			Subsystem: core.USER_SERVICE,
			Help:      "Total number of email verifications completed",
		},
		[]string{"operation"},
	)

	AccountDeletionRequested = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricDeletionRequested,
			Subsystem: core.USER_SERVICE,
			Help:      "Total number of account deletion requests",
		},
		[]string{"operation"},
	)

	PublicKeyAdded = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricPublicKeyAdded,
			Subsystem: core.USER_SERVICE,
			Help:      "Total number of public keys added to accounts",
		},
		[]string{"operation"},
	)

	UserOperationDuration = *prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:      MetricDuration,
			Subsystem: core.USER_SERVICE,
			Help:      "Time spent processing user service operations",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60, 300, 600},
		},
		[]string{"operation"},
	)

	UserOperationFailed = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricFailed,
			Subsystem: core.USER_SERVICE,
			Help:      "Total number of user service operations that failed",
		},
		[]string{"operation"},
	)
}

// GetCollectors returns all metrics as collectors for registration.
func GetCollectors() []prometheus.Collector {
	return []prometheus.Collector{
		AccountsCreated,
		AccountsUpdated,
		AccountsDeleted,
		AccountsExistsQueried,
		EmailVerificationSent,
		EmailVerified,
		AccountDeletionRequested,
		PublicKeyAdded,
		UserOperationDuration,
		UserOperationFailed,
	}
}
