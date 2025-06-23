package redis

/*
import (
	"context"
	"fmt"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"time"
)

type RedisHeartbeat struct {
	client *redis.Client
}

func NewRedisHeartbeat(client *redis.Client) *RedisHeartbeat {
	return &RedisHeartbeat{client: client}
}

func (r *RedisHeartbeat) SetHeartbeat(jobID uuid.UUID) error {
	key := fmt.Sprintf("heartbeat:%s", jobID)
	return r.client.SetEX(context.Background(), key, "1", 2*time.Minute).Err()
}

func (r *RedisHeartbeat) CheckHeartbeat(jobID uuid.UUID) (bool, error) {
	key := fmt.Sprintf("heartbeat:%s", jobID)
	exists, err := r.client.Exists(context.Background(), key).Result()
	return exists > 0, err
}

func (r *RedisHeartbeat) Close() error {
	return r.client.Close()
}
*/
