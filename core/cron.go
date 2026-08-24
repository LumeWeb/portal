package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/adjust/rmq/v5"
	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
	"github.com/looplab/fsm"
	"go.lumeweb.com/portal/db/models"
	"go.uber.org/zap"
)

const CRON_SERVICE = "cron"

// CronScheduleType defines the type of schedule for a cron job.
type CronScheduleType string

const (
	// CronScheduleTypeDaily indicates a job should run every day.
	CronScheduleTypeDaily CronScheduleType = "daily"
	// CronScheduleTypeWeekly indicates a job should run every week.
	CronScheduleTypeWeekly CronScheduleType = "weekly"
	// CronScheduleTypeMonthly indicates a job should run every month.
	CronScheduleTypeMonthly CronScheduleType = "monthly"
	// CronScheduleTypeHourly indicates a job should run every hour.
	CronScheduleTypeHourly CronScheduleType = "hourly"
	// CronScheduleTypeCron indicates a job should run based on a cron expression.
	CronScheduleTypeCron CronScheduleType = "cron"
	// CronScheduleTypeOnce indicates a job should run only once.
	CronScheduleTypeOnce CronScheduleType = "once"
	// CronScheduleTypeDuration indicates a job should run on a fixed duration interval (in minutes).
	CronScheduleTypeDuration CronScheduleType = "duration"
)

// Cronable is an interface for entities that can register and schedule cron jobs.
type Cronable interface {
	// RegisterTasks registers cron tasks with the CronService.
	RegisterTasks(ctx context.Context, cron CronService) error
	// ScheduleJobs schedules cron jobs with the CronService.
	ScheduleJobs(ctx context.Context, cron CronService) error
}

type RetryPolicy struct {
	MaxRetries    int           `json:"max_retries"`    // Maximum number of retry attempts (0 means no retries)
	InitialDelay  time.Duration `json:"initial_delay"`  // Initial delay between retries
	BackoffFactor float64       `json:"backoff_factor"` // Multiplier for exponential backoff (1.0 for constant delay)
}

// DefaultRetryPolicy provides sensible defaults for job retries
var DefaultRetryPolicy = &RetryPolicy{
	MaxRetries:    3,               // Retry up to 3 times
	InitialDelay:  5 * time.Minute, // Start with 5 minute delay
	BackoffFactor: 1.5,             // Exponential backoff factor
}

// CronScheduleDefinition defines a serializable schedule definition for cron jobs.
// The meaning of fields depends on the CronScheduleType:
//   - Daily: Runs every X days at specified time
//   - Weekly: Runs every X weeks on specified day/time
//   - Monthly: Runs every X months on specified day/time
//   - Cron: Uses cron expression for scheduling
//   - Once: Runs once at specified time
type CronScheduleDefinition struct {
	Type           CronScheduleType `json:"type"`                      // Type of schedule (daily, weekly, etc)
	Interval       uint             `json:"interval,omitempty"`        // Frequency multiplier based on type
	AtTime         time.Time        `json:"at_time,omitempty"`         // Execution time
	DayOfWeek      string           `json:"day_of_week,omitempty"`     // Day of week for weekly schedules
	DayOfMonth     int              `json:"day_of_month,omitempty"`    // Day of month for monthly schedules
	CronExpression string           `json:"cron_expression,omitempty"` // Cron expression for CronScheduleTypeCron
	RetryPolicy    *RetryPolicy     `json:"retry_policy,omitempty"`    // Retry behavior configuration
}

// NewCronScheduleDefinition creates a new CronScheduleDefinition with defaults:
//   - Type: Set to provided scheduleType
//   - Interval: Defaults to 1 (run every period)
//   - Other fields: Zero values
func NewCronScheduleDefinition(schedType CronScheduleType) *CronScheduleDefinition {
	return &CronScheduleDefinition{
		Type:     schedType,
		Interval: 1, // Default to running every period (daily/weekly/monthly)
	}
}

// WithInterval sets the frequency multiplier for the schedule.
// The interval's meaning depends on the CronScheduleType:
//   - Daily: Days between runs (e.g. 2 = every 2 days)
//   - Weekly: Weeks between runs (e.g. 2 = biweekly)
//   - Monthly: Months between runs (e.g. 3 = quarterly)
//   - Cron/Once: Interval has no effect
//
// Must be a positive integer (uint). Returns the CronScheduleDefinition for chaining.
func (s *CronScheduleDefinition) WithInterval(interval uint) *CronScheduleDefinition {
	s.Interval = interval
	return s
}

// WithAtTime sets the execution time for the schedule.
func (s *CronScheduleDefinition) WithAtTime(atTime time.Time) *CronScheduleDefinition {
	s.AtTime = atTime
	return s
}

// WithDayOfWeek sets the day of week for weekly schedules.
func (s *CronScheduleDefinition) WithDayOfWeek(day string) *CronScheduleDefinition {
	s.DayOfWeek = day
	return s
}

// WithDayOfMonth sets the day of month for monthly schedules.
func (s *CronScheduleDefinition) WithDayOfMonth(day int) *CronScheduleDefinition {
	s.DayOfMonth = day
	return s
}

// WithCronExpression sets the cron expression for cron schedules.
func (s *CronScheduleDefinition) WithCronExpression(expr string) *CronScheduleDefinition {
	s.CronExpression = expr
	return s
}

// CronScheduleRegistry provides extended schedule management capabilities.
type CronScheduleRegistry interface {
	// Register adds a schedule factory for a given type.
	Register(schedType CronScheduleType, factory ScheduleFactoryFunc)
	// Create creates a gocron.JobDefinition from a CronScheduleDefinition.
	Create(def CronScheduleDefinition) (gocron.JobDefinition, error)
	// Validate checks if a schedule definition is valid.
	Validate(def CronScheduleDefinition) error
	// GetRegisteredTypes returns all registered schedule types.
	GetRegisteredTypes() []CronScheduleType
}

// ScheduleFactoryFunc creates a gocron.JobDefinition from schedule parameters.
type ScheduleFactoryFunc func(def CronScheduleDefinition) (gocron.JobDefinition, error)

const (
	// JobNamespaceCore represents the namespace for core jobs.
	JobNamespaceCore = "core"
	// JobNamespacePlugin represents the namespace for plugin jobs.
	JobNamespacePlugin = "plugin"
)

// ValidateCronJobType validates the format of a cron job type string.
// Job types must be in the format "namespace.format" (e.g., "core.subsystem.job" or "plugin.id.job").
func ValidateCronJobType(jobType string) error {
	parts := strings.Split(jobType, ".")
	if len(parts) < 2 {
		return fmt.Errorf("job type must be in namespace.format (core.subsystem.job or plugin.id.job)")
	}

	switch parts[0] {
	case JobNamespaceCore:
		if len(parts) < 3 {
			return fmt.Errorf("core jobs require subsystem.job format")
		}
	case JobNamespacePlugin:
		if !PluginExists(parts[1]) {
			return fmt.Errorf("plugin '%s' not registered", parts[1])
		}
	default:
		return fmt.Errorf("first part must be 'core' or 'plugin'")
	}
	return nil
}

func ValidateCronJob(job CronJob) error {
	if job == nil {
		return fmt.Errorf("job cannot be nil")
	}

	// Validate origin
	switch job.Origin() {
	case JobOriginCore:
		// Core jobs must have non-empty source ID
		if job.SourceID() == "" {
			return fmt.Errorf("core jobs must specify subsystem source ID")
		}
	case JobOriginPlugin:
		// Plugin jobs must reference an existing plugin
		if job.SourceID() == "" {
			return fmt.Errorf("plugin jobs must specify plugin ID")
		}

		if !PluginExists(job.SourceID()) {
			return fmt.Errorf("plugin %q not found", job.SourceID())
		}
	default:
		return fmt.Errorf("invalid job origin: %s", job.Origin())
	}

	jobType := job.Type()
	if err := ValidateCronJobType(jobType); err != nil {
		return fmt.Errorf("invalid job type: %w", err)
	}

	// Additional validation based on origin
	switch job.Origin() {
	case JobOriginCore:
		if !strings.HasPrefix(jobType, JobNamespaceCore+".") {
			return fmt.Errorf("core jobs must use core.* namespace")
		}
	case JobOriginPlugin:
		if !strings.HasPrefix(jobType, JobNamespacePlugin+".") {
			return fmt.Errorf("plugin jobs must use plugin.* namespace")
		}
	}

	return nil
}

// GetCronJobNamespace extracts the namespace from a cron job type string.
func GetCronJobNamespace(jobType string) string {
	if parts := strings.Split(jobType, "."); len(parts) > 0 {
		return parts[0]
	}
	return ""
}

// GetCronJobOrigin parses the origin (core/plugin) from a cron job type string.
func GetCronJobOrigin(jobType string) string {
	namespace := GetCronJobNamespace(jobType)
	switch namespace {
	case JobNamespaceCore:
		return JobOriginCore
	case JobNamespacePlugin:
		return JobOriginPlugin
	default:
		return ""
	}
}

// GetCronJobSourceID parses the source ID (plugin ID or subsystem) from a cron job type string.
func GetCronJobSourceID(jobType string) string {
	parts := strings.Split(jobType, ".")
	if len(parts) < 2 {
		return ""
	}
	return strings.Join(parts[1:], ".")
}

// IsCoreCronJob checks if a cron job is a core job based on its type.
func IsCoreCronJob(jobType string) bool {
	return GetCronJobNamespace(jobType) == JobNamespaceCore
}

// IsPluginCronJob checks if a cron job is a plugin job based on its type.
func IsPluginCronJob(jobType string) bool {
	return GetCronJobNamespace(jobType) == JobNamespacePlugin
}

// BaseCronJob provides common implementation for CronJob interface.
// CronJobOption represents an optional configuration for BaseCronJob.
// CronJobOption represents an optional configuration for BaseCronJob.
// Use WithExplicitJobType() to provide an explicit job type instead of computing one.
type CronJobOption func(*BaseCronJob)

// WithExplicitJobType sets an explicit job type for the cron job.
// This is useful for plugin jobs that need to override the computed type.
// When not used, the jobType defaults to GetCronJobIdentifier(origin, sourceID).
func WithExplicitJobType(jobType string) CronJobOption {
	return func(b *BaseCronJob) {
		b.jobType = jobType
	}
}

type BaseCronJob struct {
	id                 uuid.UUID
	origin             string
	sourceID           string
	displayName        string
	scheduleDefinition *CronScheduleDefinition
	args               any
	jobType            string
	job                gocron.Job
	done               <-chan struct{}
}

var _ CronJob = (*BaseCronJob)(nil)

// Job returns the underlying gocron.Job instance.
func (b *BaseCronJob) Job() gocron.Job {
	return b.job
}

// SetJob sets the underlying gocron.Job instance.
func (b *BaseCronJob) SetJob(job gocron.Job) {
	b.job = job
}

// NewBaseCronJob creates a new BaseCronJob instance.
// The jobType defaults to being computed from origin and sourceID.
// Use WithExplicitJobType() option to override this behavior.
func NewBaseCronJob(id uuid.UUID, origin string, sourceID string, displayName string, scheduleDef *CronScheduleDefinition, args any, opts ...CronJobOption) *BaseCronJob {
	b := &BaseCronJob{
		id:                 id,
		origin:             origin,
		sourceID:           sourceID,
		displayName:        displayName,
		scheduleDefinition: scheduleDef,
		args:               args,
		jobType:            GetCronJobIdentifier(origin, sourceID), // Default: compute from origin and sourceID
		job:                nil,
		done:               nil,
	}

	for _, opt := range opts {
		opt(b)
	}

	return b
}

// Done returns a channel that will be closed when the job completes execution.
func (b *BaseCronJob) Done() <-chan struct{} {
	return b.done
}

// SetDone sets the done channel for the job.
func (b *BaseCronJob) SetDone(done <-chan struct{}) {
	b.done = done
}

// ID returns a unique identifier for the job.
func (b *BaseCronJob) ID() uuid.UUID {
	return b.id
}

// Origin returns whether job is from core or plugin.
func (b *BaseCronJob) Origin() string {
	return b.origin
}

// SourceID returns plugin_id for plugins or subsystem for core.
func (b *BaseCronJob) SourceID() string {
	return b.sourceID
}

// DisplayName returns localized human-readable name.
func (b *BaseCronJob) DisplayName() string {
	return b.displayName
}

// GetScheduledDefinition returns the schedule definition for the job.
func (b *BaseCronJob) GetScheduledDefinition() *CronScheduleDefinition {
	return b.scheduleDefinition
}

// SetScheduledDefinition sets the schedule definition for the job.
func (b *BaseCronJob) SetScheduledDefinition(schedDef *CronScheduleDefinition) {
	b.scheduleDefinition = schedDef
}

// Args returns the arguments for the job.
func (b *BaseCronJob) Args() any {
	return b.args
}

// SetArgs sets the arguments for the job.
func (b *BaseCronJob) SetArgs(args any) {
	b.args = args
}

// Schedule returns the schedule definition for the job.
func (b *BaseCronJob) Schedule() *CronScheduleDefinition {
	return b.scheduleDefinition
}

func (b *BaseCronJob) Run(_ Context, _ context.Context) error {
	return nil
}

// GetCronJobIdentifier returns the full job type identifier based on origin and source ID.
// Returns an identifier string and an error if validation fails. The identifier will be
// empty if validation fails.
func GetCronJobIdentifier(origin string, sourceID string) string {
	if sourceID == "" {
		return ""
	}

	switch origin {
	case JobOriginCore:
		return fmt.Sprintf("%s.%s", JobNamespaceCore, sourceID)
	case JobOriginPlugin:
		return fmt.Sprintf("%s.%s", JobNamespacePlugin, sourceID)
	default:
		return ""
	}
}

// Type returns the full job type identifier.
// Returns the stored jobType value, which may be explicitly set via WithExplicitJobType()
// or computed from origin and sourceID by default.
func (b *BaseCronJob) Type() string {
	return b.jobType
}

// CronJob defines the interface for a cron job.
type CronJob interface {
	// ID returns a unique identifier for the job.
	ID() uuid.UUID

	// Origin returns whether job is from core or plugin
	Origin() string // 'core' or 'plugin'

	// SourceID returns plugin_id for plugins or subsystem for core
	SourceID() string

	// DisplayName returns localized human-readable name
	DisplayName() string

	// Run executes the job logic.
	// ctx provides access to the DI container (services, DB, logger).
	// eventCtx provides request-level context for cancellation and tracing.
	Run(ctx Context, eventCtx context.Context) error

	// Schedule returns the schedule definition for the job.
	Schedule() *CronScheduleDefinition

	// Args returns the arguments for the job.
	Args() any
	// SetArgs sets the arguments for the job.
	SetArgs(args any)

	// Type returns the full job type identifier based on origin and source
	Type() string

	// Job returns the underlying gocron.Job instance
	Job() gocron.Job

	// SetJob sets the underlying gocron.Job instance
	SetJob(job gocron.Job)

	// Done returns a channel that will be closed when the job completes execution.
	// This can be used to wait for job completion or detect when a one-time job finishes.
	// For recurring jobs, the channel is closed after each execution.
	Done() <-chan struct{}
	// SetDone sets the done channel for the job
	SetDone(done <-chan struct{})

	// SetScheduledDefinition sets the schedule definition for the job.
	SetScheduledDefinition(schedDef *CronScheduleDefinition)
}

const (
	// JobOriginCore indicates that the job originated from the core system.
	JobOriginCore = "core"
	// JobOriginPlugin indicates that the job originated from a plugin.
	JobOriginPlugin = "plugin"
)

// CronTaskFunc defines the function signature for cron tasks.
type CronTaskFunc func(ctx Context, jobID uuid.UUID) error

// JobLogField returns a standardized zap.Field for job logging.
func JobLogField(job CronJob) zap.Field {
	return zap.String("job", fmt.Sprintf("%s/%s", job.Origin(), job.ID()))
}

// CronService defines the interface for managing cron jobs.
type CronService interface {
	// Start starts the scheduler.
	Start(ctx context.Context) error
	// Stop stops the scheduler.
	Stop(ctx context.Context) error

	// RegisterEntity registers a Cronable entity (like a service) that has cron jobs to register.
	// The entity must implement both RegisterTasks() and ScheduleJobs() methods.
	RegisterEntity(entity Cronable)

	// RegisterJobType registers a job type with optional default schedule.
	RegisterJobType(ctx context.Context, jobType string, factory CronJobFactoryFunc, defaultSchedule *CronScheduleDefinition) error

	// RegisterJob registers a fully configured job instance.
	RegisterJob(ctx context.Context, job CronJob, retryPolicy *RetryPolicy) error

	// RegisterPluginJobs registers all cron jobs from a plugin.
	RegisterPluginJobs(ctx context.Context, plugin PluginInfo) error

	// RunJob manually triggers a job by its ID.
	RunJob(ctx context.Context, id uuid.UUID) error

	// GetActiveJob returns an active cron job by its UUID if it exists.
	GetActiveJob(ctx context.Context, jobID uuid.UUID) (CronJob, bool, error)

	// GetScheduleRegistry provides access to schedule registry.
	ScheduleRegistry() CronScheduleRegistry

	// GetJobFactory provides access to job factory.
	JobFactory() CronJobFactory

	// StateMachine provides access to the state machine.
	StateMachine() CronJobStateMachine

	// Monitor provides access to the cron job monitor.
	Monitor() CronMonitor

	// Coordinator provides access to the cron coordinator.
	Coordinator() CronCoordinator

	// Service provides access to the base service interface.
	Service
}

// CronCoordinator handles all cron coordination regardless of mode.
type CronCoordinator interface {
	// SetHeartbeat sets the heartbeat for a job.
	SetHeartbeat(ctx context.Context, jobID uuid.UUID) error
	// CheckHeartbeat checks the heartbeat for a job.
	CheckHeartbeat(ctx context.Context, jobID uuid.UUID) (bool, error)

	// EnqueueJob enqueues a job for execution.
	EnqueueJob(ctx context.Context, jobID uuid.UUID) error
	// CreateJobFromDB creates a CronJob instance from the database.
	CreateJobFromDB(ctx context.Context, jobID uuid.UUID) (CronJob, error)
	// HandleFailedJob handles a failed job.
	HandleFailedJob(ctx context.Context, jobID uuid.UUID, failures uint) error

	// SetupJob performs setup tasks for a job.
	SetupJob(ctx context.Context, jobID uuid.UUID) error
	// CleanupJob performs cleanup tasks for a job.
	CleanupJob(ctx context.Context, jobID uuid.UUID) error
	// ExecuteJob executes a job.
	ExecuteJob(ctx context.Context, jobID uuid.UUID) error
	// JobContext returns the context for a job.
	JobContext(ctx context.Context, jobID uuid.UUID) context.Context

	// Jobs returns all scheduled jobs.
	Jobs() []gocron.Job

	// Start starts the coordinator.
	Start() error
	// Close closes the coordinator.
	Close() error

	// RemoveJob removes a job from the scheduler.
	RemoveJob(jobID uuid.UUID) error
}

// CronJobFactoryFunc defines a function signature for creating CronJob instances.
type CronJobFactoryFunc func() (CronJob, error)

// CronJobFactory defines a factory for creating CronJob instances based on their type.
type CronJobFactory interface {
	// CreateJob creates a new CronJob instance of the specified type.
	CreateJob(ctx context.Context, jobType string) (CronJob, error)
	// RegisterFactory registers a factory function for a given job type.
	RegisterFactory(ctx context.Context, jobType string, factory CronJobFactoryFunc, defaultSchedule *CronScheduleDefinition) error
	// GetDefaultSchedule retrieves the default schedule for a given job type.
	GetDefaultSchedule(ctx context.Context, jobType string) (*CronScheduleDefinition, bool)
}

// CronJobTriggerTransport handles distributing job triggers in cluster mode.
type CronJobTriggerTransport interface {
	// Publish publishes a job ID to trigger its execution.
	Publish(jobID uuid.UUID) error
	// Subscribe subscribes to job trigger events and calls the handler function when a job ID is received.
	Subscribe(handler func(jobID uuid.UUID)) error
	// Close closes the trigger transport.
	Close() error
}

// CronRedisConnector provides shared Redis connection management.
type CronRedisConnector interface {
	// GetConnection returns the Redis connection.
	GetConnection() rmq.Connection
	// Close closes the Redis connection.
	Close() error
}

// RedisTriggerService handles job trigger distribution.
type RedisTriggerService interface {
	CronJobTriggerTransport
	CronHeartbeatService
}

// CronRedisQueueService handles job execution queuing.
type CronRedisQueueService interface {
	// Enqueue enqueues a message onto a queue.
	Enqueue(queueName string, message []byte) error
	// StartConsuming begins consuming messages from the queue
	StartConsuming(handler func([]byte) error) error
	// Close cleans up queue resources
	Close() error
}

// CronHeartbeatService handles job heartbeat tracking.
type CronHeartbeatService interface {
	// SetHeartbeat sets the heartbeat for a job.
	SetHeartbeat(jobID uuid.UUID) error
	// CheckHeartbeat checks the heartbeat for a job.
	CheckHeartbeat(jobID uuid.UUID) (bool, error)
}

// CronMonitor handles monitoring and maintenance of cron jobs.
type CronStateOption func(*CronStateParams)

// CronStateParams holds parameters for cron job state transitions.
type CronStateParams struct {
	lastRun   bool
	failures  int
	heartbeat bool
}

// LastRun returns whether the job was last run.
func (p *CronStateParams) LastRun() bool {
	return p.lastRun
}

// Failures returns the number of failures for the job.
func (p *CronStateParams) Failures() int {
	return p.failures
}

// Heartbeat returns whether the job has a heartbeat.
func (p *CronStateParams) Heartbeat() bool {
	return p.heartbeat
}

// WithCronLastRun sets the lastRun parameter for a CronStateParams.
func WithCronLastRun() CronStateOption {
	return func(p *CronStateParams) {
		p.lastRun = true
	}
}

// WithCronFailures sets the failures parameter for a CronStateParams.
func WithCronFailures(count int) CronStateOption {
	return func(p *CronStateParams) {
		p.failures = count
	}
}

// WithCronHeartbeat sets the heartbeat parameter for a CronStateParams.
func WithCronHeartbeat() CronStateOption {
	return func(p *CronStateParams) {
		p.heartbeat = true
	}
}

// CronMonitor defines the interface for monitoring cron jobs.
type CronMonitor interface {
	// CleanupOrphanedJobs removes jobs from plugins that no longer exist.
	CleanupOrphanedJobs(ctx context.Context) (int, error)

	// ProcessDeadJobs detects and handles jobs that appear to be dead/stuck.
	RequeueStuckJobs(ctx context.Context) error

	// CleanupCompletedJobs removes old completed one-time jobs.
	CleanupCompletedJobs(ctx context.Context) error

	// StartMonitoring begins monitoring cron jobs.
	StartMonitoring(ctx context.Context) error

	// StopMonitoring stops all monitoring activities.
	StopMonitoring(ctx context.Context) error

	// SignalMaintenance signals the monitor to perform maintenance tasks.
	SignalMaintenance(ctx context.Context)

	// StartHeartbeat starts periodic heartbeats for a job.
	StartHeartbeat(ctx context.Context, jobID uuid.UUID)

	// StopHeartbeat stops heartbeats for a job.
	StopHeartbeat(ctx context.Context, jobID uuid.UUID)

	// CheckHeartbeat verifies if a job's heartbeat is still active.
	CheckHeartbeat(ctx context.Context, jobID uuid.UUID) (bool, error)
}

// CronJobStateMachine handles state transitions for cron jobs.
type CronJobStateMachine interface {
	// Transition validates and performs state transitions for a job.
	Transition(ctx context.Context, jobID uuid.UUID, newState models.CronJobState, opts ...CronStateOption) error

	// IsValidTransition checks if a state transition is valid.
	IsValidTransition(ctx context.Context, current, new models.CronJobState) bool

	// RemoveStateMachine removes the state machine for a job.
	RemoveStateMachine(ctx context.Context, jobID uuid.UUID)
}

// CronJobStateMachineRegistry manages FSM instances for cron jobs.
type CronJobStateMachineRegistry interface {
	// GetOrCreate gets or creates an FSM instance for a job.
	GetOrCreate(ctx context.Context, jobID uuid.UUID) (*models.CronJob, *fsm.FSM, error)

	// Remove removes an FSM instance for a job.
	Remove(ctx context.Context, jobID uuid.UUID)
}

// PluginHasCron checks if a plugin has cron jobs.
func PluginHasCron(plugin PluginInfo) bool {
	return len(plugin.CronJobs) > 0
}
