package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/adjust/rmq/v5"
	redislock "github.com/go-co-op/gocron-redis-lock/v2"
	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/db/types"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"math"
	"math/rand"
	"sync"
	"time"
)

var _ core.CronService = (*CronServiceDefault)(nil)
var _ gocron.Logger = (*cronLogger)(nil)

const failureBackoffBaseDelay = 1 * time.Millisecond
const failureBackoffMaxDelay = 1 * time.Hour
const queuePollDuration = 100 * time.Millisecond

const redisQueueNamespace = "cron"
const consumerTag = "cron-consumer"
const consumerPrefetch = 10
const deadJobCheckInterval = 1 * time.Minute
const maxTaskDuration = 1 * time.Hour
const heartbeatTimeout = 10 * time.Minute

func init() {
	core.RegisterService(core.ServiceInfo{
		ID: core.CRON_SERVICE,
		Factory: func() (core.Service, []core.ContextBuilderOption, error) {
			return NewCronService()
		},
	})
}

type CronServiceDefault struct {
	ctx            core.Context
	db             *gorm.DB
	logger         *core.Logger
	entities       []core.Cronable
	scheduler      gocron.Scheduler
	tasks          sync.Map
	taskArgs       sync.Map
	taskDefs       sync.Map
	taskRecurring  sync.Map
	queueMu        sync.Mutex
	queues         map[string]rmq.Queue
	redisQueueConn rmq.Connection
	cronRunningMap sync.Map
}

type cancelStruct struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func NewCronService() (*CronServiceDefault, []core.ContextBuilderOption, error) {
	cron := &CronServiceDefault{
		queues: make(map[string]rmq.Queue),
	}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			cron.ctx = ctx
			cron.db = ctx.DB()
			cron.logger = ctx.Logger()
			return nil
		}),
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			scheduler, queue, err := newScheduler(ctx.Config(), ctx.Logger())
			if err != nil {
				return err
			}

			cron.scheduler = scheduler
			cron.redisQueueConn = queue

			return nil
		}),
		core.ContextWithExitFunc(func(ctx core.Context) error {
			return cron.stop()
		}),
	)

	return cron, opts, nil
}

type cronLogger struct {
	logger *core.Logger
}

func (c cronLogger) Debug(msg string, args ...any) {
	c.logger.Debug(msg, zap.Any("args", args))
}

func (c cronLogger) Error(msg string, args ...any) {
	c.logger.Error(msg, zap.Any("args", args))
}

func (c cronLogger) Info(msg string, args ...any) {
	c.logger.Info(msg, zap.Any("args", args))
}

func (c cronLogger) Warn(msg string, args ...any) {
	c.logger.Warn(msg, zap.Any("args", args))
}

func NewCronLogger(logger *core.Logger) gocron.Logger {
	return &cronLogger{logger: logger}
}

func newScheduler(cm config.Manager, logger *core.Logger) (gocron.Scheduler, rmq.Connection, error) {
	cfg := cm.Config()
	if cfg.Core.ClusterEnabled() && cfg.Core.Clustered.Redis != nil {
		redisClient, err := cfg.Core.Clustered.Redis.Client()
		if err != nil {
			return nil, nil, err
		}
		locker, err := redislock.NewRedisLocker(redisClient, redislock.WithTries(1), redislock.WithExpiry(heartbeatTimeout))
		if err != nil {
			return nil, nil, err
		}

		errCh := make(chan error)
		go func(errCh chan error) {
			for err := range errCh {
				logger.Error("rmq Background error", zap.Error(err))
			}
		}(errCh)

		client, err := rmq.OpenConnectionWithRedisClient(consumerTag, redisClient, errCh)
		if err != nil {
			return nil, nil, err
		}

		scheduler, err := gocron.NewScheduler(gocron.WithDistributedLocker(locker), gocron.WithLogger(NewCronLogger(logger)))
		if err != nil {
			return nil, nil, err
		}

		return scheduler, client, nil
	}

	scheduler, err := gocron.NewScheduler(gocron.WithLogger(NewCronLogger(logger)))
	if err != nil {
		return nil, nil, err
	}

	return scheduler, nil, nil
}
func (c *CronServiceDefault) Start() error {
	for _, service := range c.entities {
		err := service.RegisterTasks(c)
		if err != nil {
			c.logger.Fatal("Failed to register tasks for service", zap.Error(err))
		}
	}

	var cronJobs []models.CronJob
	result := c.db.Where(&models.CronJob{State: models.CronJobStateQueued}).Find(&cronJobs)
	if result.Error != nil {
		return result.Error
	}

	for _, cronJob := range cronJobs {
		if c.clusterMode() {
			err := c.enqueueJob(&cronJob)
			if err != nil {
				c.logger.Error("Failed to enqueue job", zap.Error(err))
				return err
			}
		} else {
			err := c.kickOffJob(&cronJob, cronJob.Failures)
			if err != nil {
				c.logger.Error("Failed to kick off job", zap.Error(err))
				return err
			}
		}
	}

	for _, service := range c.entities {
		err := service.ScheduleJobs(c)
		if err != nil {
			c.logger.Error("Failed to schedule jobs for service", zap.Error(err))
			return err
		}
	}

	c.scheduler.Start()

	return nil
}

func (c *CronServiceDefault) stop() error {
	err := c.scheduler.Shutdown()
	if err != nil {
		return err
	}
	return nil
}

func (c *CronServiceDefault) clusterMode() bool {
	return c.ctx.Config().Config().Core.ClusterEnabled()
}

func (c *CronServiceDefault) kickOffJob(job *models.CronJob, errors uint) error {
	if c.ctx.Config().Config().Core.ClusterEnabled() {
		return c.enqueueJob(job)
	}

	return c.scheduleJob(job, errors)
}

func (c *CronServiceDefault) enqueueJob(job *models.CronJob) error {
	// Get or create the queue for this job function
	queue, err := c.getOrCreateQueue(job.Function)
	if err != nil {
		return fmt.Errorf("failed to get or create queue: %w", err)
	}

	// Publish the job to the queue
	id := job.UUID[:]
	if err := queue.Publish(string(id)); err != nil {
		return fmt.Errorf("failed to publish job to queue: %w", err)
	}

	c.logger.Debug("Job enqueued successfully",
		zap.String("jobID", job.UUID.String()),
		zap.String("function", job.Function))

	return nil
}

func (c *CronServiceDefault) getOrCreateQueue(name string) (rmq.Queue, error) {
	c.queueMu.Lock()
	defer c.queueMu.Unlock()

	// Check if the queue already exists
	if queue, exists := c.queues[name]; exists {
		return queue, nil
	}

	// Create a new queue
	queue, err := c.redisQueueConn.OpenQueue(redisQueueNamespace + "." + name)
	if err != nil {
		return nil, fmt.Errorf("failed to open queue %s: %w", name, err)
	}

	// Store the queue for future use
	c.queues[name] = queue

	// Start consuming from this queue
	if err := c.startConsuming(queue, name); err != nil {
		return nil, fmt.Errorf("failed to start consuming from queue %s: %w", name, err)
	}

	return queue, nil
}

func (c *CronServiceDefault) startConsuming(queue rmq.Queue, name string) error {
	// Start consuming with a prefetch of 10 and a poll duration of 100ms
	if err := queue.StartConsuming(consumerPrefetch, queuePollDuration); err != nil {
		return fmt.Errorf("failed to start consuming: %w", err)
	}

	// Add a consumer to the queue
	_, err := queue.AddConsumer(consumerTag, NewJobConsumer(c, name))
	if err != nil {
		return fmt.Errorf("failed to add consumer: %w", err)
	}

	return nil
}

func (c *CronServiceDefault) scheduleJob(job *models.CronJob, errors uint) error {
	task, err := c.prepareTask(job)
	if err != nil {
		return fmt.Errorf("failed to prepare task: %w", err)
	}

	options := []gocron.JobOption{
		gocron.WithName(job.UUID.String()),
		gocron.WithIdentifier(uuid.UUID(job.UUID)),
		gocron.WithEventListeners(
			gocron.AfterJobRuns(c.listenerFuncNoError),
			gocron.AfterJobRunsWithError(c.listenerFuncErr),
		),
	}

	jobDef, err := c.loadTaskDef(job)
	if err != nil {
		return fmt.Errorf("failed to load task definition: %w", err)
	}

	var backoffDelay time.Duration
	if errors > 0 {
		backoffDelay = exponentialBackoff(errors, failureBackoffBaseDelay, failureBackoffMaxDelay)
		jobDef = gocron.OneTimeJob(gocron.OneTimeJobStartDateTime(time.Now().Add(backoffDelay)))
	}

	cronJob, err := c.scheduler.NewJob(jobDef, task, options...)
	if err != nil {
		return fmt.Errorf("failed to schedule job: %w", err)
	}

	ctx := cronJob.Context()

	wrapCtx, cancel := context.WithCancel(ctx)

	c.cronRunningMap.Store(uuid.UUID(job.UUID), cancelStruct{ctx: wrapCtx, cancel: cancel})

	go func() {
		if backoffDelay > 0 {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()

			backoffEnd := time.Now().Add(backoffDelay)

			for {
				select {
				case <-ticker.C:
					if err := c.jobHeartbeat(context.Background(), uuid.UUID(job.UUID)); err != nil {
						c.logger.Error("Failed to update job heartbeat during backoff", zap.Error(err))
					}
				case <-time.After(time.Until(backoffEnd)):
					// Backoff period is over
					goto waitForStart
				}
			}
		}

	waitForStart:
		<-cronJob.Started()
		rlock := cronJob.Lock().(*redislock.RedisLock)

		for {
			until := rlock.Get().Until()
			timeToWait := until.Sub(time.Now()) + 500*time.Millisecond

			select {
			case <-time.After(timeToWait):
				c.logger.Debug("Lock expired, attempting to extend", zap.String("jobID", job.UUID.String()))
				valid, err := rlock.Get().ValidContext(ctx)
				if err != nil {
					c.logger.Error("Failed to check lock validity", zap.Error(err))
					continue
				}

				if valid {
					err = rlock.Get().Lock()
				} else {
					err = rlock.Extend(ctx)
				}

				if err != nil {
					c.logger.Error("Failed to extend lock", zap.Error(err))
					continue
				}

				c.logger.Debug("Lock extended", zap.String("jobID", job.UUID.String()))

				// Call the heartbeat
				if err = c.jobHeartbeat(context.Background(), uuid.UUID(job.UUID)); err != nil {
					c.logger.Error("Failed to update job heartbeat", zap.Error(err))
				}
			case <-ctx.Done():
				c.logger.Debug("Job canceled, exiting heartbeat watch", zap.String("jobID", job.UUID.String()))
				return
			case <-wrapCtx.Done():
				c.logger.Debug("Job done, exiting heartbeat watch", zap.String("jobID", job.UUID.String()))
				return
			}
		}
	}()

	return nil
}

func (c *CronServiceDefault) prepareTask(job *models.CronJob) (gocron.Task, error) {
	taskFunc, ok := c.tasks.Load(job.Function)
	if !ok {
		return nil, fmt.Errorf("function %s not found", job.Function)
	}

	argsFunc, ok := c.taskArgs.Load(job.Function)
	if !ok {
		return nil, fmt.Errorf("arguments factory for function %s not found", job.Function)
	}

	args := argsFunc.(core.CronTaskArgsFactoryFunction)()

	if len(job.Args) > 0 {
		if err := json.Unmarshal([]byte(job.Args), args); err != nil {
			return nil, fmt.Errorf("failed to unmarshal job arguments: %w", err)
		}
	}

	varArgs := []interface{}{
		interface{}(struct{}{}),
		interface{}(c.ctx),
	}

	if args != nil {
		varArgs = []interface{}{
			args,
			interface{}(c.ctx),
		}
	}

	return gocron.NewTask(taskFunc, varArgs...), nil
}

func (c *CronServiceDefault) loadTaskDef(job *models.CronJob) (gocron.JobDefinition, error) {
	taskDefFunc, ok := c.taskDefs.Load(job.Function)
	if !ok {
		return nil, fmt.Errorf("task definition for function %s not found", job.Function)
	}

	return taskDefFunc.(core.CronTaskDefArgsFactoryFunction)(), nil
}

func (c *CronServiceDefault) listenerFuncNoError(jobID uuid.UUID, jobName string) {
	c.jobDone(jobID)
	var job models.CronJob
	job.UUID = types.BinaryUUID(jobID)

	// Update last run time and reset failures
	if err := db.RetryOnLock(c.db, func(db *gorm.DB) *gorm.DB {
		return db.Model(&models.CronJob{}).Where(&job).Updates(&models.CronJob{
			LastRun:  timeNow(),
			Failures: 0,
		})
	}); err != nil {
		c.logger.Error("Failed to update job after successful run",
			zap.Error(err),
			zap.String("jobID", jobID.String()),
		)
	}

	if c.isRecurring(job.Function) {
		// For recurring jobs, reschedule
		if err := c.rescheduleJob(&job); err != nil {
			c.logger.Error("Failed to reschedule recurring job",
				zap.Error(err),
				zap.String("jobID", jobID.String()),
			)
		}
	} else {
		err := c.jobStateComplete(context.Background(), jobID)
		if err != nil {
			c.logger.Error("Failed to update job state", zap.Error(err), zap.String("jobID", jobID.String()))
		}
		// For one-time jobs, delete from the database
		if err = c.deleteJob(&job); err != nil {
			c.logger.Error("Failed to delete one-time job after completion",
				zap.Error(err),
				zap.String("jobID", jobID.String()),
			)
		}
	}

	c.logger.Debug("Job completed successfully", zap.String("jobID", jobID.String()), zap.String("function", job.Function))
}

func (c *CronServiceDefault) listenerFuncErr(jobID uuid.UUID, jobName string, err error) {
	c.jobDone(jobID)
	var job models.CronJob
	job.UUID = types.BinaryUUID(jobID)

	c.logger.Error("Job failed",
		zap.Error(err),
		zap.String("jobID", jobID.String()),
	)

	// Update last run time and increment failures
	if err := db.RetryOnLock(c.db, func(db *gorm.DB) *gorm.DB {
		return db.Model(&job).Where(&job).Updates(map[string]interface{}{
			"last_run": time.Now(),
			"failures": gorm.Expr("failures + ?", 1),
		})
	}); err != nil {
		c.logger.Error("Failed to update job after failure",
			zap.Error(err),
			zap.String("jobID", jobID.String()),
		)
	}

	// Fetch the updated job to get the current failure count
	if err := db.RetryOnLock(c.db, func(db *gorm.DB) *gorm.DB {
		return db.Where(&job).First(&job)
	}); err != nil {
		c.logger.Error("Failed to fetch updated job",
			zap.Error(err),
			zap.String("jobID", jobID.String()),
		)
		return
	}

	// Log the failure
	cronLog := &models.CronJobLog{
		CronJobID: job.ID,
		Type:      models.CronJobLogTypeFailure,
		Message:   err.Error(),
	}
	if err := db.RetryOnLock(c.db, func(db *gorm.DB) *gorm.DB {
		return db.Create(cronLog)
	}); err != nil {
		c.logger.Error("Failed to create cron job log",
			zap.Error(err),
			zap.String("jobID", jobID.String()),
		)
	}

	// Reschedule the job with backoff
	if err := c.kickOffJob(&job, job.Failures); err != nil {
		c.logger.Error("Failed to reschedule job with backoff",
			zap.Error(err),
			zap.String("jobID", jobID.String()),
		)
	}
}

func (c *CronServiceDefault) jobDone(jobID uuid.UUID) {
	if val, ok := c.cronRunningMap.Load(jobID); ok {
		cancel := val.(cancelStruct)
		cancel.cancel()
		c.cronRunningMap.Delete(jobID)
	}
}

func (c *CronServiceDefault) isRecurring(funcName string) bool {
	_, ok := c.taskRecurring.Load(funcName)

	return ok
}

func (c *CronServiceDefault) rescheduleJob(job *models.CronJob) error {
	err := c.jobStateComplete(context.Background(), uuid.UUID(job.UUID))
	if err != nil {
		return err
	}
	err = c.jobStateReset(context.Background(), uuid.UUID(job.UUID))
	if err != nil {
		return err
	}
	return c.enqueueJob(job)
}

func (c *CronServiceDefault) deleteJob(job *models.CronJob) error {
	return db.RetryOnLock(c.db, func(db *gorm.DB) *gorm.DB {
		return db.Where(&models.CronJob{UUID: job.UUID}).Delete(job)
	})
}

func (c *CronServiceDefault) RegisterEntity(service core.Cronable) {
	c.entities = append(c.entities, service)
}

func (c *CronServiceDefault) RegisterTask(name string, taskFunc core.CronTaskFunction, taskDefFunc core.CronTaskDefArgsFactoryFunction, taskArgFunc core.CronTaskArgsFactoryFunction, recurring bool) {
	c.tasks.Store(name, taskFunc)
	c.taskDefs.Store(name, taskDefFunc)
	c.taskArgs.Store(name, taskArgFunc)
	if recurring {
		c.taskRecurring.Store(name, recurring)
	}
}

func (c *CronServiceDefault) CreateJob(function string, args any) error {
	job, err := c.createJobRecord(function, args)
	if err != nil {
		return err
	}

	return c.kickOffJob(job, job.Failures)
}

func (c *CronServiceDefault) CreateJobScheduled(function string, args any) error {
	job, err := c.createJobRecord(function, args)
	if err != nil {
		return err
	}

	return c.kickOffJob(job, job.Failures)
}

func (c *CronServiceDefault) CreateExistingJobScheduled(uuid uuid.UUID) error {
	var job models.CronJob

	job.UUID = types.BinaryUUID(uuid)

	if err := db.RetryOnLock(c.db, func(db *gorm.DB) *gorm.DB {
		return db.First(&job)
	}); err != nil {
		return err
	}

	return c.kickOffJob(&job, job.Failures)
}

func (c *CronServiceDefault) CreateJobIfNotExists(function string, args any) error {
	exists, _ := c.JobExists(function, args)

	if !exists {
		return c.CreateJob(function, args)
	}

	return nil
}

func (c *CronServiceDefault) JobExists(function string, args any) (bool, *models.CronJob) {
	var job models.CronJob

	job.Function = function

	if args != nil {
		bytes, err := json.Marshal(args)
		if err != nil {
			return false, nil
		}

		job.Args = string(bytes)
	}

	result := c.db.Where(&job).First(&job)

	if result.Error != nil {
		return false, nil
	}

	return true, &job
}

func (c *CronServiceDefault) createJobRecord(function string, args any) (*models.CronJob, error) {
	job := models.CronJob{
		Function: function,
		UUID:     types.BinaryUUID(uuid.New()),
	}

	if args != nil {
		bytes, err := json.Marshal(args)
		if err != nil {
			return nil, err
		}

		job.Args = string(bytes)
	}

	result := c.db.Create(&job)

	if result.Error != nil {
		return nil, result.Error
	}

	return &job, nil
}

func (c *CronServiceDefault) jobHeartbeat(ctx context.Context, jobID uuid.UUID) error {
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var job models.CronJob
		job.UUID = types.BinaryUUID(jobID)

		if err := db.RetryOnLock(c.db, func(db *gorm.DB) *gorm.DB {
			return db.First(&job)
		}); err != nil {
			return err
		}

		job.LastHeartbeat = timeNow()

		return db.RetryOnLock(c.db, func(db *gorm.DB) *gorm.DB {
			return tx.Save(&job)
		})
	})
}

// updateJobState updates the state of a job in the database
func (c *CronServiceDefault) updateJobState(ctx context.Context, jobID uint, newState models.CronJobState) error {
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var job models.CronJob
		if err := tx.First(&job, jobID).Error; err != nil {
			return err
		}

		job.State = newState
		job.LastRun = timeNow()

		if newState == models.CronJobStateProcessing {
			job.Failures = 0 // Reset failures when job starts processing
		} else if newState == models.CronJobStateFailed {
			job.Failures++
		}

		return tx.Save(&job).Error
	})
}

// transitionJobState handles the state transition logic
func (c *CronServiceDefault) transitionJobState(ctx context.Context, jobID uuid.UUID, fromState, toState models.CronJobState) error {
	return c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var job models.CronJob
		err := db.RetryOnLock(c.db, func(db *gorm.DB) *gorm.DB {
			return db.Model(&job).Where(&models.CronJob{UUID: types.BinaryUUID(jobID)}).First(&job)
		})
		if err != nil {
			return err
		}

		if job.State != fromState {
			return errors.New("invalid state transition")
		}

		job.State = toState
		job.LastRun = timeNow()

		if toState == models.CronJobStateProcessing {
			job.Failures = 0
		} else if toState == models.CronJobStateFailed {
			job.Failures++
		}
		return db.RetryOnLock(c.db, func(db *gorm.DB) *gorm.DB {
			return tx.Save(&job)
		})
	})
}

// StartJob transitions a job from Queued to Processing
func (c *CronServiceDefault) jobStateReset(ctx context.Context, jobID uuid.UUID) error {
	return c.transitionJobState(ctx, jobID, models.CronJobStateCompleted, models.CronJobStateQueued)
}

// StartJob transitions a job from Queued to Processing
func (c *CronServiceDefault) jobStateProcessing(ctx context.Context, jobID uuid.UUID) error {
	return c.transitionJobState(ctx, jobID, models.CronJobStateQueued, models.CronJobStateProcessing)
}

// CompleteJob transitions a job from Processing to Completed
func (c *CronServiceDefault) jobStateComplete(ctx context.Context, jobID uuid.UUID) error {
	return c.transitionJobState(ctx, jobID, models.CronJobStateProcessing, models.CronJobStateCompleted)
}

// FailJob transitions a job from Processing to Failed
func (c *CronServiceDefault) jobStateFailed(ctx context.Context, jobID uuid.UUID) error {
	return c.transitionJobState(ctx, jobID, models.CronJobStateProcessing, models.CronJobStateFailed)
}

// RequeueJob transitions a job from Failed or Completed to Queued
func (c *CronServiceDefault) jobStateRequeue(ctx context.Context, jobID uuid.UUID) error {
	err := c.transitionJobState(ctx, jobID, models.CronJobStateFailed, models.CronJobStateQueued)
	if err != nil {
		// If the job wasn't in Failed state, try transitioning from Completed
		err = c.transitionJobState(ctx, jobID, models.CronJobStateCompleted, models.CronJobStateQueued)
	}
	return err
}

func (c *CronServiceDefault) getJob(id uuid.UUID) (*models.CronJob, error) {
	var job models.CronJob
	job.UUID = types.BinaryUUID(id)

	if err := db.RetryOnLock(c.db, func(db *gorm.DB) *gorm.DB {
		return c.db.Model(&job).Where(&job).First(&job)
	}); err != nil {
		return nil, err
	}

	return &job, nil
}

func (c *CronServiceDefault) startDeadJobDetection(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(deadJobCheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := c.detectAndHandleDeadJobs(ctx); err != nil {
					c.logger.Error("Failed to detect and handle dead tasks", zap.Error(err))
				}
			}
		}
	}()
}

func (c *CronServiceDefault) detectAndHandleDeadJobs(ctx context.Context) error {
	var deadTasks []models.CronJob
	now := time.Now()
	heartbeatDeadline := now.Add(-heartbeatTimeout)
	durationDeadline := now.Add(-maxTaskDuration)

	if err := db.RetryOnLock(c.db, func(db *gorm.DB) *gorm.DB {
		return db.WithContext(ctx).Where("state = ? AND ((last_heartbeat < ? AND last_heartbeat IS NOT NULL) OR (created_at < ? AND last_heartbeat IS NULL))",
			models.CronJobStateProcessing, heartbeatDeadline, durationDeadline).Find(&deadTasks)
	}); err != nil {
		return err
	}

	for _, task := range deadTasks {
		if err := c.handleDeadJob(ctx, &task); err != nil {
			c.logger.Error("Failed to handle dead task", zap.Error(err), zap.String("jobID", task.UUID.String()))
		}
	}

	return nil
}

func (c *CronServiceDefault) handleDeadJob(ctx context.Context, job *models.CronJob) error {
	err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Reload the job to ensure we have the latest state
		if err := db.RetryOnLock(c.db, func(db *gorm.DB) *gorm.DB {
			return db.First(job, job.ID)
		}); err != nil {
			return err
		}

		// Double-check that the job is still in a processing state
		if job.State != models.CronJobStateProcessing {
			return nil // Task has already been handled, nothing to do
		}

		job.State = models.CronJobStateQueued
		job.Failures++

		if err := db.RetryOnLock(c.db, func(db *gorm.DB) *gorm.DB {
			return db.Save(job)
		}); err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Log the dead job
	c.logger.Warn("Detected dead job",
		zap.String("jobID", job.UUID.String()),
		zap.String("function", job.Function),
		zap.Time("lastHeartbeat", *job.LastHeartbeat),
		zap.Uint("failures", job.Failures))

	// If it's a recurring job, reschedule it
	if c.isRecurring(job.Function) {
		if err = c.rescheduleJob(job); err != nil {
			c.logger.Error("Failed to reschedule dead recurring job", zap.Error(err), zap.String("jobID", job.UUID.String()))
			return err
		}
	} else if c.clusterMode() {
		// For one-time tasks in clustered mode, re-enqueue the job
		if err = c.enqueueJob(job); err != nil {
			c.logger.Error("Failed to re-enqueue dead one-time job", zap.Error(err), zap.String("jobID", job.UUID.String()))
			return err
		}
	}

	return nil
}

type JobConsumer struct {
	cron     *CronServiceDefault
	function string
}

func NewJobConsumer(cron *CronServiceDefault, function string) *JobConsumer {
	return &JobConsumer{cron: cron, function: function}
}

func (jc *JobConsumer) Consume(delivery rmq.Delivery) {
	sendErr := func(msg string, err error) {
		if err != nil {
			jc.cron.logger.Error(msg,
				zap.Error(err),
				zap.String("payload", delivery.Payload()))
			err = delivery.Reject()
			if err != nil {
				jc.cron.logger.Error("Failed to reject delivery",
					zap.Error(err),
					zap.String("payload", delivery.Payload()))
			}
		}
	}

	ack := func(job *models.CronJob) {
		err := delivery.Ack()
		if err != nil {
			jc.cron.logger.Error("Failed to ack delivery",
				zap.Error(err),
				zap.String("jobID", job.UUID.String()))
		}
	}

	_uuid, err := uuid.FromBytes([]byte(delivery.Payload()))
	if err != nil {
		sendErr("Failed to parse job ID", err)
		return
	}

	job, err := jc.cron.getJob(_uuid)
	if err != nil {
		sendErr("Failed to get job", err)
		return
	}

	jc.cron.logger.Debug("Job consumed", zap.String("jobID", job.UUID.String()), zap.String("function", job.Function), zap.String("args", job.Args))

	if job.State != models.CronJobStateQueued {
		ack(job)
		return
	}

	if err = jc.cron.jobStateProcessing(context.Background(), uuid.UUID(job.UUID)); err != nil {
		sendErr("Failed to update job state", err)
		return
	}

	if err = jc.cron.scheduleJob(job, job.Failures); err != nil {
		sendErr("Failed to kick off job", err)
		return
	}

	ack(job)
}

func TaskDefinitionOneTimeJob() gocron.JobDefinition {
	return gocron.OneTimeJob(gocron.OneTimeJobStartImmediately())
}
func exponentialBackoff(attempt uint, baseDelay, maxDelay time.Duration) time.Duration {
	// Calculate delay
	delay := float64(baseDelay) * math.Pow(2, float64(attempt))

	// Add jitter (randomness)
	jitter := rand.Float64() * 0.5 // 50% jitter
	delay = delay * (1 + jitter)

	// Cap the delay
	if delay > float64(maxDelay) {
		delay = float64(maxDelay)
	}

	return time.Duration(delay)
}

func timeNow() *time.Time {
	t := time.Now()

	return &t
}
