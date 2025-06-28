package redis

/*
// CronRedisQueueService implementation
type RedisQueueService struct {
	conn             rmq.Connection
	queues           map[string]rmq.Queue
	logger           *zap.Logger
	jobFactory       core.CronJobFactory
	stateUpdater     func(ctx context.Context, jobID uuid.UUID, state models.CronJobState) error
	heartbeatUpdater func(ctx context.Context, jobID uuid.UUID) error
}

func NewRedisQueueService(
	ctx core.Context,
	jobFactory core.CronJobFactory,
	stateUpdater func(ctx context.Context, jobID uuid.UUID, state models.CronJobState) error,
	heartbeatUpdater func(ctx context.Context, jobID uuid.UUID) error,
) (*RedisQueueService, error) {
	redisConfig := ctx.Config().Config().Redis
	conn, err := rmq.OpenConnection("portal", "tcp", fmt.Sprintf("%s:%d", redisConfig.Host, redisConfig.Port), 1, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &RedisQueueService{
		conn:             conn,
		queues:           make(map[string]rmq.Queue),
		logger:           ctx.ServiceLogger(nil).Desugar(),
		jobFactory:       jobFactory,
		stateUpdater:     stateUpdater,
		heartbeatUpdater: heartbeatUpdater,
	}, nil
}

func (r *RedisQueueService) Enqueue(queueName string, message []byte) error {
	queue, err := r.getOrCreateQueue(queueName)
	if err != nil {
		return err
	}
	return queue.Publish(message)
}

func (r *RedisQueueService) getOrCreateQueue(name string) (rmq.Queue, error) {
	if queue, ok := r.queues[name]; ok {
		return queue, nil
	}

	queue, err := r.conn.OpenQueue(name)
	if err != nil {
		return nil, fmt.Errorf("failed to open queue %s: %w", name, err)
	}

	// Start consuming with prefetch
	if err := queue.StartConsuming(10, 100*time.Millisecond); err != nil {
		return nil, fmt.Errorf("failed to start consuming: %w", err)
	}

	// Add consumer
	if _, err := queue.AddConsumer("cron-consumer", r); err != nil {
		return nil, fmt.Errorf("failed to add consumer: %w", err)
	}

	r.queues[name] = queue
	return queue, nil
}

func (r *RedisQueueService) SetHeartbeat(jobID uuid.UUID) error {
	key := fmt.Sprintf("cron:heartbeat:%s", jobID.String())
	expiration := 2 * time.Minute // Heartbeat timeout

	client := r.conn.GetRedisClient()
	_, err := client.Set(context.Background(), key, "1", expiration).Result()
	if err != nil {
		return fmt.Errorf("failed to set heartbeat: %w", err)
	}
	return nil
}

func (r *RedisQueueService) CheckHeartbeat(jobID uuid.UUID) (bool, error) {
	key := fmt.Sprintf("cron:heartbeat:%s", jobID.String())

	client := r.conn.GetRedisClient()
	exists, err := client.Exists(context.Background(), key).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check heartbeat: %w", err)
	}
	return exists > 0, nil
}

type JobMessage struct {
	ID       uuid.UUID `json:"id"`
	JobType  string    `json:"job_type"`
	Name     string    `json:"name"`
	Args     string    `json:"args"`
	SchedDef string    `json:"sched_def"`
}

func (r *RedisQueueService) Consume(delivery rmq.Delivery) {
	var msg JobMessage
	if err := json.Unmarshal([]byte(delivery.Payload()), &msg); err != nil {
		r.logger.Error("Failed to parse job message from queue",
			zap.Error(err),
			zap.String("payload", delivery.Payload()))
		_ = delivery.Reject()
		return
	}

	// Handle maintenance jobs differently
	if strings.HasPrefix(msg.JobType, "maintenance.") {
		maintenanceType := strings.TrimPrefix(msg.JobType, "maintenance.")
		if err := r.cron.handleMaintenanceJob(maintenanceType); err != nil {
			r.logger.Error("Failed to handle maintenance job",
				zap.String("type", maintenanceType),
				zap.Error(err))
			_ = delivery.Reject()
			return
		}
		_ = delivery.Ack()
		return
	}

	// Get the cron service from context
	cronService := r.cron.ctx.Service(core.CRON_SERVICE).(core.CronService)

	// Execute the job through the coordinator
	if err := cronService.RunJob(msg.ID); err != nil {
		r.logger.Error("Failed to run job",
			zap.String("jobID", msg.ID.String()),
			zap.Error(err))
		_ = delivery.Reject()
		return
	}

	// Handle job completion
	if err != nil {
		r.logger.Error("CronJob execution failed",
			zap.String("jobID", msg.ID.String()),
			zap.Error(err))

		// Get current failure count from DB
		var cronJob models.CronJob
		if err := r.cron.db.Where("uuid = ?", types.FromUUID(msg.ID)).First(&cronJob).Error; err != nil {
			r.logger.Error("Failed to get job failure count",
				zap.String("jobID", msg.ID.String()),
				zap.Error(err))
			_ = delivery.Reject()
			return
		}

		// Use shared failure handler
		if err := r.cron.handleFailedJob(msg.ID, cronJob.Failures+1); err != nil {
			r.logger.Error("Failed to handle failed job",
				zap.String("jobID", msg.ID.String()),
				zap.Error(err))
		}

		// Reject the message to retry later
		_ = delivery.Reject()
		return
	}

	// CronJob completed successfully
	r.logger.Info("CronJob completed successfully",
		zap.String("jobID", msg.ID.String()))

	// Update state to Completed
	if updateErr := r.cron.updateJobState(
		context.Background(),
		msg.ID,
		models.CronJobStateCompleted,
	); updateErr != nil {
		r.logger.Error("Failed to update job status after completion",
			zap.String("jobID", msg.ID.String()),
			zap.Error(updateErr))
	}

	// Acknowledge the message
	if err := delivery.Ack(); err != nil {
		r.logger.Error("Failed to acknowledge message",
			zap.String("jobID", jobID.String()),
			zap.Error(err))
	}
}
*/
