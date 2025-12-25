package cron

import (
	"github.com/prometheus/client_golang/prometheus"
	"go.lumeweb.com/portal/core"
)

// Metric name constants for cron service metrics
const (
	MetricJobsCompleted           = "jobs_completed_total"
	MetricJobsFailed              = "jobs_failed_total"
	MetricJobExecutionDuration    = "job_execution_duration_seconds"
	MetricJobSchedulingDelay      = "job_scheduling_delay_seconds"
	MetricJobsRunning             = "jobs_running"
	MetricJobsRegistered          = "jobs_registered_total"
	MetricJobsUnregistered        = "jobs_unregistered_total"
	MetricConcurrencyLimitReached = "concurrency_limit_reached_total"
	MetricSchedulerUp             = "up"
)

// Global metric instances (created once, reused everywhere)
var (
	JobsCompleted           prometheus.Counter
	JobsFailed              prometheus.Counter
	JobExecutionDuration    prometheus.Histogram
	JobSchedulingDelay      prometheus.Histogram
	JobsRunning             prometheus.Gauge
	JobsRegistered          prometheus.Counter
	JobsUnregistered        prometheus.Counter
	ConcurrencyLimitReached prometheus.Counter
	SchedulerUp             prometheus.Gauge
)

// init initializes all cron metrics.
// Called automatically when the package is imported.
func init() {
	JobsCompleted = prometheus.NewCounter(prometheus.CounterOpts{
		Name:      MetricJobsCompleted,
		Subsystem: core.CRON_SERVICE,
		Help:      "Total number of cron jobs that completed successfully",
	})
	JobsFailed = prometheus.NewCounter(prometheus.CounterOpts{
		Name:      MetricJobsFailed,
		Subsystem: core.CRON_SERVICE,
		Help:      "Total number of cron job failures",
	})
	JobExecutionDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:      MetricJobExecutionDuration,
		Subsystem: core.CRON_SERVICE,
		Help:      "Time spent executing cron jobs",
		Buckets:   []float64{0.1, 0.5, 1, 5, 10, 30, 60, 120, 300},
	})
	JobSchedulingDelay = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:      MetricJobSchedulingDelay,
		Subsystem: core.CRON_SERVICE,
		Help:      "Delay between scheduled and actual start time of cron jobs",
		Buckets:   []float64{0.01, 0.05, 0.1, 0.5, 1, 5, 10, 30},
	})
	JobsRunning = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:      MetricJobsRunning,
		Subsystem: core.CRON_SERVICE,
		Help:      "Number of cron jobs currently running",
	})
	JobsRegistered = prometheus.NewCounter(prometheus.CounterOpts{
		Name:      MetricJobsRegistered,
		Subsystem: core.CRON_SERVICE,
		Help:      "Total number of cron jobs registered",
	})
	JobsUnregistered = prometheus.NewCounter(prometheus.CounterOpts{
		Name:      MetricJobsUnregistered,
		Subsystem: core.CRON_SERVICE,
		Help:      "Total number of cron jobs unregistered",
	})
	ConcurrencyLimitReached = prometheus.NewCounter(prometheus.CounterOpts{
		Name:      MetricConcurrencyLimitReached,
		Subsystem: core.CRON_SERVICE,
		Help:      "Number of times a job could not start due to concurrency limits",
	})
	SchedulerUp = prometheus.NewGauge(prometheus.GaugeOpts{
		Name:      MetricSchedulerUp,
		Subsystem: core.CRON_SERVICE,
		Help:      "Indicates if the scheduler is running (1) or stopped (0)",
	})
}

// GetCollectors returns all metrics as collectors for registration.
func GetCollectors() []prometheus.Collector {
	return []prometheus.Collector{
		JobsCompleted,
		JobsFailed,
		JobExecutionDuration,
		JobSchedulingDelay,
		JobsRunning,
		JobsRegistered,
		JobsUnregistered,
		ConcurrencyLimitReached,
		SchedulerUp,
	}
}
