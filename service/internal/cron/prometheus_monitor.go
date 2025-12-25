package cron

import (
	"time"

	"github.com/go-co-op/gocron/v2"
)

// PrometheusMonitor implements gocron.SchedulerMonitor to export cron job metrics.
// It references global metric instances from metrics.go, which are registered
// externally by the cron service.
type PrometheusMonitor struct{}

// NewPrometheusMonitor creates a new PrometheusMonitor.
// Metrics are initialized and registered externally via InitMetrics() and GetCollectors().
func NewPrometheusMonitor() *PrometheusMonitor {
	return &PrometheusMonitor{}
}

// SchedulerStarted is called when Start() is invoked on the scheduler.
func (p *PrometheusMonitor) SchedulerStarted() {
	SchedulerUp.Set(1)
}

// SchedulerStopped is called when the scheduler's main loop stops,
// but before final cleanup in Shutdown().
func (p *PrometheusMonitor) SchedulerStopped() {
	SchedulerUp.Set(0)
}

// SchedulerShutdown is called when Shutdown() completes successfully.
func (p *PrometheusMonitor) SchedulerShutdown() {
	SchedulerUp.Set(0)
}

// JobRegistered is called when a job is registered with the scheduler.
func (p *PrometheusMonitor) JobRegistered(_ gocron.Job) {
	JobsRegistered.Inc()
}

// JobUnregistered is called when a job is unregistered from the scheduler.
func (p *PrometheusMonitor) JobUnregistered(_ gocron.Job) {
	JobsUnregistered.Inc()
}

// JobStarted is called when a job starts running.
func (p *PrometheusMonitor) JobStarted(_ gocron.Job) {
	JobsRunning.Inc()
}

// JobRunning is called when a job is running.
// This is called periodically while the job executes.
func (p *PrometheusMonitor) JobRunning(_ gocron.Job) {
}

// JobFailed is called when a job fails to complete successfully.
func (p *PrometheusMonitor) JobFailed(_ gocron.Job, err error) {
	JobsRunning.Dec()
	JobsFailed.Inc()
}

// JobCompleted is called when a job has completed running successfully.
func (p *PrometheusMonitor) JobCompleted(_ gocron.Job) {
	JobsRunning.Dec()
	JobsCompleted.Inc()
}

// JobExecutionTime is called after a job completes (success or failure)
// with the time it took to execute.
func (p *PrometheusMonitor) JobExecutionTime(_ gocron.Job, duration time.Duration) {
	JobExecutionDuration.Observe(duration.Seconds())
}

// JobSchedulingDelay is called when a job starts running, providing both
// the scheduled time and actual start time.
func (p *PrometheusMonitor) JobSchedulingDelay(_ gocron.Job, scheduledTime time.Time, actualStartTime time.Time) {
	if delay := actualStartTime.Sub(scheduledTime); delay > 0 {
		JobSchedulingDelay.Observe(delay.Seconds())
	}
}

// ConcurrencyLimitReached is called when a job cannot start immediately
// due to concurrency limits (singleton or limit mode).
// limitType will be "singleton" or "limit".
func (p *PrometheusMonitor) ConcurrencyLimitReached(_ string, _ gocron.Job) {
	ConcurrencyLimitReached.Inc()
}
