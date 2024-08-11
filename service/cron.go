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
	"runtime/debug"
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
const heartbeatInterval = 5 * time.Minute
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
	go c.startDeadJobDetection()

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

func (c *CronServiceDefault) kickOffJob(job *models.CronJob, errors uint64) error {
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

func (c *CronServiceDefault) scheduleJob(job *models.CronJob, errors uint64) error {
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
			gocron.AfterJobRunsWithPanic(c.listenerFuncPanic),
		),
	}

	jobDef, err := c.loadTaskDef(job)
	if err != nil {
		return fmt.Errorf("failed to load task definition: %w", err)
	}

	backoffDelay := c.calculateBackoff(job.Failures)
	if errors > 0 {
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
			ticker := time.NewTicker(heartbeatInterval)
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

		go func() {
			ticker := time.NewTicker(heartbeatInterval)
			defer ticker.Stop()

			for {
				select {
				case <-ticker.C:
					if err := c.jobHeartbeat(context.Background(), uuid.UUID(job.UUID)); err != nil {
						c.logger.Error("Failed to update job heartbeat during pre-startup", zap.Error(err))
					}
				case <-ctx.Done():
					c.logger.Debug("Job canceled, exiting pre-heartbeat watch", zap.String("jobID", job.UUID.String()))
					return
				case <-wrapCtx.Done():
					c.logger.Debug("Job done, exiting pre-heartbeat watch", zap.String("jobID", job.UUID.String()))
					return
				case <-cronJob.Started():
					c.logger.Debug("Job started, exiting pre-heartbeat watch", zap.String("jobID", job.UUID.String()))
					return
				}
			}
		}()

	waitForStart:
		<-cronJob.Started()
		rlock := cronJob.Lock().(*redislock.RedisLock)

		for {
			until := rlock.Get().Until()
			timeToWait := until.Sub(time.Now()) - 30*time.Second

			select {
			case <-time.After(timeToWait):
				c.logger.Debug("Lock expired, attempting to extend", zap.String("jobID", job.UUID.String()))

				err = rlock.Extend(ctx)
				if err != nil {
					c.logger.Debug("Failed to extend lock", zap.Error(err))
					err = rlock.Get().Lock()
					if err != nil {
						c.logger.Error("Failed to lock after extending", zap.Error(err))
						continue
					}
				}

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

	// Fetch the job
	if err := c.db.Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.First(&job)
		})
	}); err != nil {
		c.logger.Error("Failed to fetch job",
			zap.Error(err),
			zap.String("jobID", jobID.String()),
		)
		return
	}

	// Update last run time and reset failures
	if err := c.db.Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			job.LastRun = timeNow()
			job.Failures = 0
			return db.Save(&job)
		})
	}); err != nil {
		c.logger.Error("Failed to update job after successful run",
			zap.Error(err),
			zap.String("jobID", jobID.String()),
		)
		return
	}

	if c.isRecurring(job.Function) {
		if err := c.rescheduleJob(&job); err != nil {
			c.logger.Error("Failed to reschedule recurring job",
				zap.Error(err),
				zap.String("jobID", jobID.String()),
			)
		}
	} else {
		err := c.updateJobState(c.ctx, jobID, models.CronJobStateCompleted)
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
	c.handleJobFailure(jobID, jobName, err)
}

func (c *CronServiceDefault) listenerFuncPanic(jobID uuid.UUID, jobName string, recoverData any) {
	err := fmt.Errorf("panic occurred: %v\n%s", recoverData, debug.Stack())
	c.handleJobFailure(jobID, jobName, err)
}

func (c *CronServiceDefault) handleJobFailure(jobID uuid.UUID, _ string, jobErr error) {
	c.jobDone(jobID)
	var job models.CronJob
	job.UUID = types.BinaryUUID(jobID)

	c.logger.Error("Job failed",
		zap.Error(jobErr),
		zap.String("jobID", jobID.String()),
	)

	err := c.updateJobState(c.ctx, jobID, models.CronJobStateFailed)
	if err != nil {
		c.logger.Error("Failed to update job state", zap.Error(err), zap.String("jobID", jobID.String()))
		return
	}

	err = c.updateJobState(c.ctx, jobID, models.CronJobStateQueued)
	if err != nil {
		c.logger.Error("Failed to update job state", zap.Error(err), zap.String("jobID", jobID.String()))
		return
	}

	// Fetch the updated job to get the current failure count
	if err = db.RetryOnLock(c.db, func(db *gorm.DB) *gorm.DB {
		return db.Where(&job).First(&job)
	}); err != nil {
		c.logger.Error("Failed to fetch updated job",
			zap.Error(err),
			zap.String("jobID", jobID.String()),
		)
		return
	}

	// Log the failure (including panics) in the cron job logs
	cronLog := &models.CronJobLog{
		CronJobID: job.ID,
		Type:      models.CronJobLogTypeFailure,
		Message:   jobErr.Error(),
	}
	if err = db.RetryOnLock(c.db, func(db *gorm.DB) *gorm.DB {
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
	err := c.updateJobState(c.ctx, uuid.UUID(job.UUID), models.CronJobStateCompleted)
	if err != nil {
		return err
	}

	err = c.updateJobState(c.ctx, uuid.UUID(job.UUID), models.CronJobStateQueued)
	if err != nil {
		return err
	}
	return c.kickOffJob(job, job.Failures)
}

func (c *CronServiceDefault) deleteJob(job *models.CronJob) error {
	return db.RetryOnLock(c.db, func(db *gorm.DB) *gorm.DB {
		return db.Where(&models.CronJob{UUID: job.UUID}).Delete(job)
	})
}

func (c *CronServiceDefault) RegisterEntity(service core.Cronable) {
	c.entities = append(c.entities, service)
}

func (c *CronServiceDefault) RegisterTask(name string, taskFunc core.CronTaskFunction[core.CronTaskArgs], taskDefFunc core.CronTaskDefArgsFactoryFunction, taskArgFunc core.CronTaskArgsFactoryFunction, recurring bool) {
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
	// Fetch the job
	var job models.CronJob
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.Where("uuid = ?", types.BinaryUUID(jobID)).First(&job)
		})
	}); err != nil {
		return fmt.Errorf("failed to fetch job: %w", err)
	}

	oldHeartbeat := job.LastHeartbeat

	// Update the job heartbeat
	var rowsAffected int64
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			ret := db.Model(&job).
				Where("uuid = ? AND version = ?", types.BinaryUUID(jobID), job.Version).
				Updates(models.CronJob{
					LastHeartbeat: timeNow(),
					Version:       job.Version + 1,
				})

			rowsAffected = ret.RowsAffected
			return ret
		})
	}); err != nil {
		return fmt.Errorf("failed to update job heartbeat: %w", err)
	}

	if rowsAffected == 0 {
		c.logger.Warn("Job heartbeat updated by another process")
		return nil
	}

	c.logger.Debug("Job heartbeat updated",
		zap.String("jobID", jobID.String()),
		zap.Time("oldHeartbeat", *oldHeartbeat),
		zap.Time("newHeartbeat", *job.LastHeartbeat))

	return nil
}

func (c *CronServiceDefault) updateJobState(ctx context.Context, jobID uuid.UUID, newState models.CronJobState) error {
	// Fetch the job
	var job models.CronJob
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.Where("uuid = ?", types.BinaryUUID(jobID)).First(&job)
		})
	}); err != nil {
		return fmt.Errorf("failed to fetch job: %w", err)
	}

	oldState := job.State

	// Update the job state
	var rowsAffected int64
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			ret := db.Model(&job).
				Where("uuid = ? AND version = ?", types.BinaryUUID(jobID), job.Version).
				Updates(models.CronJob{
					State:         newState,
					LastRun:       timeNow(),
					Version:       job.Version + 1,
					LastHeartbeat: timeNow(),
				})

			rowsAffected = ret.RowsAffected
			return ret
		})
	}); err != nil {
		return fmt.Errorf("failed to update job state: %w", err)
	}

	if rowsAffected == 0 {
		return errors.New("job was updated by another process")
	}

	c.logger.Debug("Job state updated",
		zap.String("jobID", jobID.String()),
		zap.String("oldState", string(oldState)),
		zap.String("newState", string(newState)))

	return nil
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

func (c *CronServiceDefault) startDeadJobDetection() {
	ticker := time.NewTicker(deadJobCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			c.logger.Info("Stopping dead job detection")
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(c.ctx, 5*time.Minute)
			err := c.detectAndHandleDeadJobs(ctx)
			cancel()
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					c.logger.Error("Failed to detect and handle dead jobs", zap.Error(err))
				} else {
					c.logger.Info("Dead job detection cancelled")
				}
			}
		}
	}
}
func (c *CronServiceDefault) calculateBackoff(failures uint64) time.Duration {
	if failures == 0 {
		return 0
	}
	backoffDelay := c.calculateMaxBackoffWithJitter(failures)
	return backoffDelay
}

func (c *CronServiceDefault) calculateMaxBackoffWithJitter(failures uint64) time.Duration {
	if failures == 0 {
		return 0
	}

	// Calculate the maximum possible delay including jitter
	baseDelay := float64(failureBackoffBaseDelay) * math.Pow(2, float64(failures))
	maxDelayWithJitter := baseDelay * 1.5 // 1.5 accounts for maximum 50% jitter

	// Cap the delay
	if maxDelayWithJitter > float64(failureBackoffMaxDelay) {
		maxDelayWithJitter = float64(failureBackoffMaxDelay)
	}

	return time.Duration(maxDelayWithJitter)
}

func (c *CronServiceDefault) detectAndHandleDeadJobs(ctx context.Context) error {
	now := time.Now()
	heartbeatDeadline := now.Add(-heartbeatTimeout)

	var potentialDeadJobs []models.CronJob

	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.Where("state IN (?, ?) AND last_heartbeat < ?",
				models.CronJobStateProcessing, models.CronJobStateQueued, heartbeatDeadline).
				Find(&potentialDeadJobs)
		})
	}); err != nil {
		return fmt.Errorf("error querying for potential dead jobs: %w", err)
	}

	for _, job := range potentialDeadJobs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if c.shouldHandleDeadJob(&job, now) {
				if err := c.handleDeadJob(ctx, uuid.UUID(job.UUID)); err != nil {
					c.logger.Error("Failed to handle dead job",
						zap.Error(err),
						zap.String("jobID", job.UUID.String()),
						zap.String("function", job.Function))
				}
			}
		}
	}

	return nil
}

func (c *CronServiceDefault) shouldHandleDeadJob(job *models.CronJob, now time.Time) bool {
	if job.State == models.CronJobStateProcessing {
		// Always handle processing jobs that missed their heartbeat
		return true
	}

	// For queued jobs, check if the backoff period has elapsed
	backoffDuration := c.calculateBackoff(job.Failures)
	var backoffEndTime time.Time

	if job.LastRun != nil && !job.LastRun.IsZero() {
		backoffEndTime = job.LastRun.Add(backoffDuration)
	} else {
		backoffEndTime = job.UpdatedAt.Add(backoffDuration)
	}

	return now.After(backoffEndTime)
}

func (c *CronServiceDefault) handleDeadJob(ctx context.Context, jobID uuid.UUID) error {
	// Fetch the job
	var job models.CronJob
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.Where("uuid = ?", types.BinaryUUID(jobID)).First(&job)
		})
	}); err != nil {
		return fmt.Errorf("failed to fetch job: %w", err)
	}

	// Check if the job is still in a processing state
	if job.State != models.CronJobStateProcessing {
		return nil // Job has already been handled, nothing to do
	}

	// Update the job state
	var rowsAffected int64
	if err := c.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			ret := db.Model(&job).
				Where("uuid = ? AND version = ? AND state = ?", types.BinaryUUID(jobID), job.Version, models.CronJobStateProcessing).
				Updates(models.CronJob{
					State:    models.CronJobStateQueued,
					Failures: job.Failures + 1,
					Version:  job.Version + 1,
				})
			rowsAffected = ret.RowsAffected
			return ret
		})
	}); err != nil {
		return fmt.Errorf("failed to update job: %w", err)
	}

	if rowsAffected == 0 {
		return errors.New("job was updated by another process or is no longer in processing state")
	}

	c.logger.Warn("Detected and requeued dead job",
		zap.String("jobID", job.UUID.String()),
		zap.String("function", job.Function),
		zap.Time("lastHeartbeat", *job.LastHeartbeat),
		zap.Uint64("failures", job.Failures+1))

	// Handle recurring or one-time job
	if c.isRecurring(job.Function) {
		if err := c.rescheduleJob(&job); err != nil {
			return fmt.Errorf("failed to reschedule dead recurring job: %w", err)
		}
	} else {
		if err := c.kickOffJob(&job, job.Failures); err != nil {
			return fmt.Errorf("failed to re-enqueue dead one-time job: %w", err)
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
		jc.cron.logger.Error("Attempted to run job that does not exist", zap.String("jobID", _uuid.String()))
		ack(job)
		return
	}

	jc.cron.logger.Debug("Job consumed", zap.String("jobID", job.UUID.String()), zap.String("function", job.Function), zap.String("args", job.Args))

	if job.State != models.CronJobStateQueued {
		ack(job)
		return
	}
	if err = jc.cron.updateJobState(jc.cron.ctx, uuid.UUID(job.UUID), models.CronJobStateProcessing); err != nil {
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
func exponentialBackoff(attempt uint64, baseDelay, maxDelay time.Duration) time.Duration {
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
