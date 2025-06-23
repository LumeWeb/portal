package cron

/*
import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/db/types"
	"go.uber.org/zap"
)

type ClusterCoordinator struct {
	transport        core.CronJobTriggerTransport
	queueService     core.QueueService
	heartbeatService core.CronHeartbeatService
	db               *gorm.DB
	logger           *zap.Logger
}

func NewClusterCoordinator(
	transport JobTriggerTransport,
	queueService QueueService,
	heartbeatService HeartbeatService,
	db *gorm.DB,
	logger *zap.Logger,
) *ClusterCoordinator {
	return &ClusterCoordinator{
		transport:        transport,
		queueService:     queueService,
		heartbeatService: heartbeatService,
		db:               db,
		logger:           logger,
	}
}

func (c *ClusterCoordinator) SetHeartbeat(jobID uuid.UUID) error {
	return c.heartbeatService.SetHeartbeat(jobID)
}

func (c *ClusterCoordinator) CheckHeartbeat(jobID uuid.UUID) (bool, error) {
	return c.heartbeatService.CheckHeartbeat(jobID)
}

func (c *ClusterCoordinator) EnqueueJob(jobID uuid.UUID) error {
	var job models.CronJob
	if err := c.db.Where("uuid = ?", types.FromUUID(jobID)).First(&job).Error; err != nil {
		return fmt.Errorf("failed to find job: %w", err)
	}

	msg := internal.JobMessage{
		ID:       jobID,
		JobType:  job.JobType,
		Name:     job.Name,
		Args:     job.Args,
		SchedDef: job.SchedDef,
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	return c.queueService.Enqueue("cron_jobs", msgBytes)
}

func (c *ClusterCoordinator) Start() error {
	if err := c.transport.Subscribe(c.handleTrigger); err != nil {
		return fmt.Errorf("failed to subscribe to triggers: %w", err)
	}
	return nil
}

func (c *ClusterCoordinator) Close() error {
	err1 := c.transport.Close()
	err2 := c.queueService.Close()
	err3 := c.heartbeatService.Close()
	return errors.Join(err1, err2, err3)
}

func (c *ClusterCoordinator) handleTrigger(jobID uuid.UUID) {
	if _, err := c.scheduler.GetJob(jobID.String()); err != nil {
		c.logger.Debug("CronJob not scheduled on this node",
			zap.String("jobID", jobID.String()))
		return
	}

	if err := c.runJob(jobID); err != nil {
		c.logger.Error("Failed to run triggered job",
			zap.String("jobID", jobID.String()),
			zap.Error(err))
	}
}
*/
