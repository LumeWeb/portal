package redis

/*
import (
	"github.com/adjust/rmq/v5"
	"github.com/google/uuid"
	"time"
)

type RedisTransport struct {
	conn  rmq.Connection
	queue rmq.Queue
}

func NewRedisTransport(conn rmq.Connection) *RedisTransport {
	queue, _ := conn.OpenQueue("cron_triggers")
	return &RedisTransport{conn: conn, queue: queue}
}

func (r *RedisTransport) Publish(jobID uuid.UUID) error {
	return r.queue.Publish(jobID.String())
}

func (r *RedisTransport) Subscribe(handler func(jobID uuid.UUID)) error {
	if err := r.queue.StartConsuming(10, 100*time.Millisecond); err != nil {
		return err
	}
	_, err := r.queue.AddConsumer("cron_transport", r)
	return err
}

func (r *RedisTransport) Consume(delivery rmq.Delivery) {
	jobID, err := uuid.Parse(delivery.Payload())
	if err != nil {
		delivery.Reject()
		return
	}
	handler(jobID)
	delivery.Ack()
}

func (r *RedisTransport) Close() error {
	return r.conn.Close()
}
*/
