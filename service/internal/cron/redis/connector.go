package redis

/*
import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/adjust/rmq/v5"
	"github.com/google/uuid"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/service"
	"go.uber.org/zap"
	"time"
)

type RedisConnectorImpl struct {
	conn   rmq.Connection
	logger *zap.Logger
}

func NewRedisConnector(ctx core.Context) (*RedisConnectorImpl, error) {
	redisConfig := ctx.GetConfig().GetConfig().Redis
	conn, err := rmq.OpenConnection("portal_cron", "tcp",
		fmt.Sprintf("%s:%d", redisConfig.Host, redisConfig.Port), 1, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}
	return &RedisConnectorImpl{
		conn:   conn,
		logger: ctx.ServiceLogger(nil).Desugar(),
	}, nil
}

func (r *RedisConnectorImpl) GetConnection() rmq.Connection {
	return r.conn
}

func (r *RedisConnectorImpl) Close() error {
	return r.conn.Close()
}

type RedisTriggerService struct {
	connector RedisConnector
	logger    *zap.Logger
}

func (r *RedisTriggerService) SetHeartbeat(jobID uuid.UUID) error {
	key := fmt.Sprintf("cron:heartbeat:%s", jobID.String())
	expiration := 2 * time.Minute

	client := r.connector.GetConnection().GetRedisClient()
	_, err := client.Set(context.Background(), key, "1", expiration).Result()
	return err
}

func (r *RedisTriggerService) CheckHeartbeat(jobID uuid.UUID) (bool, error) {
	key := fmt.Sprintf("cron:heartbeat:%s", jobID.String())

	client := r.connector.GetConnection().GetRedisClient()
	exists, err := client.Exists(context.Background(), key).Result()
	return exists > 0, err
}

func NewRedisTriggerService(connector RedisConnector, logger *zap.Logger) *RedisTriggerService {
	return &RedisTriggerService{
		connector: connector,
		logger:    logger,
	}
}

// Implement CronJobTriggerTransport and CronHeartbeatService methods...

type RedisQueueService struct {
	connector RedisConnector
	logger    *zap.Logger
	queues    map[string]rmq.Queue
}

func NewRedisQueueService(connector RedisConnector, logger *zap.Logger) *RedisQueueService {
	return &RedisQueueService{
		connector: connector,
		logger:    logger,
		queues:    make(map[string]rmq.Queue),
	}
}

// Implement QueueService methods...

// Explicit interface implementation checks
var _ core.CronJobTriggerTransport = (*RedisService)(nil)
var _ core.QueueService = (*RedisService)(nil)
var _ core.CronHeartbeatService = (*RedisService)(nil)

func NewRedisService(ctx core.Context, cronService *core.CronService) (*core.CronRedisQueueService, error) {
	redisConfig := ctx.GetConfig().GetConfig().Redis
	conn, err := rmq.OpenConnection("portal_cron", "tcp",
		fmt.Sprintf("%s:%d", redisConfig.Host, redisConfig.Port), 1, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &RedisService{
		conn:     conn,
		queues:   make(map[string]rmq.Queue),
		logger:   ctx.ServiceLogger(nil).Desugar(),
		handlers: make(map[string]func(jobID uuid.UUID)),
		cron:     cronService,
	}, nil
}

// Implements CronJobTriggerTransport
func (r *RedisService) Publish(jobID uuid.UUID) error {
	return r.publishToQueue("triggers", jobID)
}

// Implements CronJobTriggerTransport
func (r *RedisService) Subscribe(handler func(jobID uuid.UUID)) error {
	return r.startConsuming("triggers", "trigger_consumer", handler)
}

// Implements QueueService
func (r *RedisService) Enqueue(queueName string, message []byte) error {
	queue, err := r.getQueue(queueName)
	if err != nil {
		return err
	}
	return queue.PublishBytes(message)
}

func (r *RedisService) getQueue(name string) (rmq.Queue, error) {
	if queue, ok := r.queues[name]; ok {
		return queue, nil
	}

	queue, err := r.conn.OpenQueue(name)
	if err != nil {
		return nil, fmt.Errorf("failed to open queue %s: %w", name, err)
	}

	r.queues[name] = queue
	return queue, nil
}

func (r *RedisService) startConsuming(queueName, consumerTag string, handler func(jobID uuid.UUID)) error {
	queue, err := r.getQueue(queueName)
	if err != nil {
		return err
	}

	if err := queue.StartConsuming(10, 100*time.Millisecond); err != nil {
		return fmt.Errorf("failed to start consuming: %w", err)
	}

	r.handlers[queueName] = handler

	_, err = queue.AddConsumer(consumerTag, r)
	return err
}

func (r *RedisService) Consume(delivery rmq.Delivery) {
	var msg models.JobMessage
	if err := json.Unmarshal([]byte(delivery.Payload()), &msg); err != nil {
		r.logger.Error("Failed to parse message",
			zap.Error(err),
			zap.String("payload", delivery.Payload()))
		_ = delivery.Reject()
		return
	}

	if handler, ok := r.handlers[delivery.QueueName()]; ok {
		handler(msg.ID)
	}

	_ = delivery.Ack()
}

func (r *RedisService) publishToQueue(queueName string, jobID uuid.UUID) error {
	queue, err := r.getQueue(queueName)
	if err != nil {
		return err
	}
	return queue.Publish(jobID.String())
}

func (r *RedisService) Close() error {
	return r.conn.Close()
}

func (r *RedisService) SetHeartbeat(jobID uuid.UUID) error {
	key := fmt.Sprintf("cron:heartbeat:%s", jobID.String())
	expiration := 2 * time.Minute

	client := r.conn.GetRedisClient()
	_, err := client.Set(context.Background(), key, "1", expiration).Result()
	if err != nil {
		return fmt.Errorf("failed to set heartbeat: %w", err)
	}
	return nil
}

func (r *RedisService) CheckHeartbeat(jobID uuid.UUID) (bool, error) {
	key := fmt.Sprintf("cron:heartbeat:%s", jobID.String())

	client := r.conn.GetRedisClient()
	exists, err := client.Exists(context.Background(), key).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check heartbeat: %w", err)
	}
	return exists > 0, nil
}
*/
