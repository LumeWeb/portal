package redis

/*
import (
	"github.com/adjust/rmq/v5"
)

var _ core.QueueService = (*RedisQueue)(nil)

type RedisQueue struct {
	conn   rmq.Connection
	queues map[string]rmq.Queue
	logger *zap.Logger
}

func NewRedisQueue(conn rmq.Connection, logger *zap.Logger) *RedisQueue {
	return &RedisQueue{
		conn:   conn,
		queues: make(map[string]rmq.Queue),
		logger: logger,
	}
}

func (r *RedisQueue) StartConsuming(handler func([]byte) error) error {
	// Default queue for general consumption
	queue, err := r.getQueue("default")
	if err != nil {
		return err
	}

	if err := queue.StartConsuming(10, 100*time.Millisecond); err != nil {
		return fmt.Errorf("failed to start consuming: %w", err)
	}

	_, err = queue.AddConsumer("cron_consumer", &queueConsumer{
		handler: handler,
		logger:  r.logger,
	})
	return err
}

func (r *RedisQueue) Close() error {
	return r.conn.Close()
}

func (r *RedisQueue) getQueue(name string) (rmq.Queue, error) {
	if q, exists := r.queues[name]; exists {
		return q, nil
	}
	q, err := r.conn.OpenQueue(name)
	if err != nil {
		return nil, err
	}
	r.queues[name] = q
	return q, nil
}

func (r *RedisQueue) Enqueue(queueName string, message []byte) error {
	q, err := r.getQueue(queueName)
	if err != nil {
		return err
	}
	return q.PublishBytes(message)
}

func (r *RedisQueue) StartConsuming(handler func([]byte) error) error {
	// Implementation similar to transport but for job messages
}

func (r *RedisQueue) Close() error {
	return r.conn.Close()
}
type queueConsumer struct {
	handler func([]byte) error
	logger  *zap.Logger
}

func (qc *queueConsumer) Consume(delivery rmq.Delivery) {
	if err := qc.handler([]byte(delivery.Payload())); err != nil {
		qc.logger.Error("Failed to process message",
			zap.Error(err),
			zap.String("payload", delivery.Payload()))
		_ = delivery.Reject()
		return
	}
	_ = delivery.Ack()
}
*/
