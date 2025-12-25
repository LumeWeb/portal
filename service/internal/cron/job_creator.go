package cron

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/db/types"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type JobCreator struct {
	db         *gorm.DB
	jobFactory core.CronJobFactory
	logger     *core.Logger
}

func NewJobCreator(db *gorm.DB, jobFactory core.CronJobFactory, logger *core.Logger) *JobCreator {
	return &JobCreator{
		db:         db,
		jobFactory: jobFactory,
		logger:     logger,
	}
}

func (j *JobCreator) CreateFromDB(ctx context.Context, jobID uuid.UUID) (core.CronJob, error) {
	// Retrieve job details from database
	var cronJob models.CronJob
	if err := j.db.Where(&models.CronJob{UUID: types.FromUUID(jobID)}).First(&cronJob).Error; err != nil {
		return nil, fmt.Errorf("failed to find job in database: %w", err)
	}

	// Create job instance
	job, err := j.jobFactory.CreateJob(ctx, cronJob.JobType)
	if err != nil {
		return nil, fmt.Errorf("failed to create job instance: %w", err)
	}

	// Populate job arguments
	if err := j.populateJobArguments(job, string(cronJob.Args), string(cronJob.RetryPolicy)); err != nil {
		return nil, err
	}

	return job, nil
}

func (j *JobCreator) populateJobArguments(job core.CronJob, args string, retryPolicy string) error {
	if len(args) == 0 && len(retryPolicy) == 0 {
		return nil
	}

	if len(retryPolicy) > 0 {
		var policy core.RetryPolicy
		if err := json.Unmarshal([]byte(retryPolicy), &policy); err != nil {
			return fmt.Errorf("failed to unmarshal retry policy: %w", err)
		}
		if jobWithPolicy, ok := job.(interface {
			SetRetryPolicy(policy *core.RetryPolicy)
		}); ok {
			jobWithPolicy.SetRetryPolicy(&policy)
		}
	}

	if len(args) == 0 {
		// No arguments to populate
		return nil
	}

	var argsPtr any
	if err := json.Unmarshal([]byte(args), &argsPtr); err != nil {
		j.logger.Error("Failed to populate job arguments",
			zap.String("jobID", job.ID().String()),
			zap.Error(err))
		return fmt.Errorf("failed to populate job arguments: %w", err)
	}

	job.SetArgs(argsPtr)

	return nil
}
