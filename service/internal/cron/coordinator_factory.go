package cron

import (
	"fmt"
	"go.lumeweb.com/portal/core"
)

func NewCoordinatorFromContext(ctx core.Context, cronService core.CronService, registry core.CronJobStateMachineRegistry) (core.CronCoordinator, error) {
	/*if ctx.Config().ClusterEnabled() {
		return NewClusterCoordinatorFromContext(ctx, cronService)
	}*/
	return NewStandaloneCoordinatorFromContext(ctx, cronService, registry)
}

/*
	func NewClusterCoordinatorFromContext(ctx core.Context, cronService core.CronService) (core.CronCoordinator, error) {
		redisConfig := ctx.Config().Config().Redis

		// Create Redis connection
		conn, err := rabbitmq.NewConn(
			fmt.Sprintf("amqp://%s:%d", redisConfig.Host, redisConfig.Port),
			rabbitmq.WithConnectionOptionsLogging,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to redis: %w", err)
		}

		// Create Redis client
		redisClient := redis.NewClient(&redis.Options{
			Addr: fmt.Sprintf("%s:%d", redisConfig.Host, redisConfig.Port),
		})

		return NewClusterCoordinator(
			NewRedisTransport(conn),
			NewRedisQueue(conn, cronService.logger.Desugar()),
			NewRedisHeartbeat(redisClient),
			ctx.DB(),
			cronService.logger.Desugar(),
		), nil
	}
*/
func NewStandaloneCoordinatorFromContext(ctx core.Context, cronService core.CronService, registry core.CronJobStateMachineRegistry) (core.CronCoordinator, error) {
	coordinator, err := NewStandaloneCoordinator(
		ctx,
		cronService,
		registry,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create standalone coordinator: %w", err)
	}
	return coordinator, nil
}
