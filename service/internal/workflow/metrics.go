package workflow

import (
	"github.com/prometheus/client_golang/prometheus"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
)

// Metric name constants for workflow service metrics
const (
	MetricStarted           = "started_total"
	MetricCompleted         = "completed_total"
	MetricFailed            = "failed_total"
	MetricStepCompleted     = "step_completed_total"
	MetricStepFailed        = "step_failed_total"
	MetricStepExecuted      = "step_executed_total"
	MetricStepRetried       = "step_retried_total"
	MetricWorkflowsActive   = "workflows_active"
	MetricWorkflowsTotal    = "workflows_total"
	MetricDuration          = "duration_seconds"
	MetricStepDuration      = "step_duration_seconds"
)

// Metric label values
const (
	LabelStatusPending    = models.RequestStatusPending
	LabelStatusProcessing = models.RequestStatusProcessing
	LabelStatusCompleted  = models.RequestStatusCompleted
	LabelStatusFailed     = models.RequestStatusFailed
	LabelStatusDuplicate  = models.RequestStatusDuplicate

	LabelOperationStart        = "start"
	LabelOperationComplete     = "complete"
	LabelOperationFail         = "fail"
	LabelOperationExecute      = "execute"
	LabelOperationGetStatus    = "get_status"
	LabelOperationGetInstance  = "get_instance"
	LabelOperationList         = "list"
	LabelOperationFind         = "find"
	LabelOperationConvert       = "convert"
	LabelOperationCleanup      = "cleanup"
	LabelOperationUpdateData   = "update_data"
	LabelOperationUnknown      = "unknown"

	LabelFailureBehaviorFail     = "fail_workflow"
	LabelFailureBehaviorContinue = "continue_workflow"
	LabelFailureBehaviorRetry    = "retry_step"

	LabelStepExecutionForeground = "foreground"
	LabelStepExecutionBackground = "background"

	LabelWorkflowUnknown = "unknown"
)

// Global metric instances
var (
	WorkflowsStarted         prometheus.CounterVec
	WorkflowsCompleted       prometheus.CounterVec
	WorkflowsFailed          prometheus.CounterVec
	WorkflowStepsCompleted   prometheus.CounterVec
	WorkflowStepsFailed      prometheus.CounterVec
	WorkflowStepsExecuted    prometheus.CounterVec
	WorkflowStepsRetried     prometheus.CounterVec
	WorkflowsActive          prometheus.GaugeVec
	WorkflowsTotal           prometheus.GaugeVec
	WorkflowDuration         prometheus.HistogramVec
	WorkflowStepDuration     prometheus.HistogramVec
)

// init initializes all workflow metrics.
func init() {
	WorkflowsStarted = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricStarted,
			Subsystem: core.WORKFLOW_SERVICE,
			Help:      "Total number of workflows started",
		},
		[]string{"workflow", "protocol"},
	)

	WorkflowsCompleted = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricCompleted,
			Subsystem: core.WORKFLOW_SERVICE,
			Help:      "Total number of workflows completed successfully",
		},
		[]string{"workflow", "protocol"},
	)

	WorkflowsFailed = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricFailed,
			Subsystem: core.WORKFLOW_SERVICE,
			Help:      "Total number of workflows that failed",
		},
		[]string{"workflow", "protocol", "failure_behavior"},
	)

	WorkflowStepsCompleted = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricStepCompleted,
			Subsystem: core.WORKFLOW_SERVICE,
			Help:      "Total number of workflow steps completed",
		},
		[]string{"workflow", "operation"},
	)

	WorkflowStepsFailed = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricStepFailed,
			Subsystem: core.WORKFLOW_SERVICE,
			Help:      "Total number of workflow steps that failed",
		},
		[]string{"workflow", "operation", "failure_behavior"},
	)

	WorkflowStepsExecuted = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricStepExecuted,
			Subsystem: core.WORKFLOW_SERVICE,
			Help:      "Total number of workflow steps executed",
		},
		[]string{"workflow", "operation", "execution_type"},
	)

	WorkflowStepsRetried = *prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:      MetricStepRetried,
			Subsystem: core.WORKFLOW_SERVICE,
			Help:      "Total number of workflow steps retried",
		},
		[]string{"workflow", "operation"},
	)

	WorkflowsActive = *prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:      MetricWorkflowsActive,
			Subsystem: core.WORKFLOW_SERVICE,
			Help:      "Current number of active workflows by status",
		},
		[]string{"status"},
	)

	WorkflowsTotal = *prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:      MetricWorkflowsTotal,
			Subsystem: core.WORKFLOW_SERVICE,
			Help:      "Total number of registered workflows",
		},
		[]string{},
	)

	WorkflowDuration = *prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:      MetricDuration,
			Subsystem: core.WORKFLOW_SERVICE,
			Help:      "Time spent processing workflow operations",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60, 300, 600},
		},
		[]string{"operation"},
	)

	WorkflowStepDuration = *prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:      MetricStepDuration,
			Subsystem: core.WORKFLOW_SERVICE,
			Help:      "Time spent executing workflow steps",
			Buckets:   []float64{0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30, 60, 300, 600},
		},
		[]string{"workflow", "operation"},
	)
}

// GetCollectors returns all metrics as collectors for registration.
func GetCollectors() []prometheus.Collector {
	return []prometheus.Collector{
		WorkflowsStarted,
		WorkflowsCompleted,
		WorkflowsFailed,
		WorkflowStepsCompleted,
		WorkflowStepsFailed,
		WorkflowStepsExecuted,
		WorkflowStepsRetried,
		WorkflowsActive,
		WorkflowsTotal,
		WorkflowDuration,
		WorkflowStepDuration,
	}
}