package service

import (
	"fmt"
	"github.com/google/uuid"
	"go.lumeweb.com/portal/core"
	"go.uber.org/zap"
)

const (
	workflowStepExecutorJobType = "core.workflow.step_executor"
)

type workflowStepExecutorJob struct {
	*core.BaseCronJob
}

func newWorkflowStepExecutorJob() *workflowStepExecutorJob {
	return &workflowStepExecutorJob{
		BaseCronJob: core.NewBaseCronJob(
			uuid.New(),
			core.JobOriginCore,
			core.GetCronJobSourceID(workflowStepExecutorJobType),
			"Workflow Step Executor",
			core.NewCronScheduleDefinition(core.CronScheduleTypeOnce),
			nil,
		),
	}
}

func (j *workflowStepExecutorJob) Run(ctx core.Context) error {
	workflowSvc := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
	if workflowSvc == nil {
		return fmt.Errorf("%s", "failed to get workflow service")
	}

	var args uint

	rargs := j.Args()

	// args represents the workflow step requestID to execute
	switch v := rargs.(type) {
	case uint:
		args = v
	case float64:
		args = uint(v)
	default:
		return fmt.Errorf("invalid job arguments type, expected uint got %T", j.Args())
	}

	// Check if the step can be transitioned
	canTransition, err := workflowSvc.CanTransition(ctx, args)
	if err != nil {
		return fmt.Errorf("failed to check transition status: %w", err)
	}

	if !canTransition {
		// Log and exit if the step cannot be transitioned
		ctx.Logger().Info("Skipping workflow step execution",
			zap.Uint("requestID", args))
		return nil
	}

	// Execute the workflow step
	err = workflowSvc.ExecuteWorkflowStep(ctx, args)
	if err != nil {
		// Check if this was a retried error (which is expected behavior)
		if core.IsWorkflowErrorType(err, core.ErrKeyWorkflowStepRetried) {
			// Log the retry but don't fail the cron job
			ctx.Logger().Info("Workflow step retried",
				zap.Uint("requestID", args),
				zap.Error(err))
			return nil
		}
		
		// Log the error key before wrapping to preserve context
		if workflowErr := core.AsWorkflowError(err); workflowErr != nil {
			ctx.Logger().Error("Workflow step execution failed",
				zap.Uint("requestID", args),
				zap.String("errorKey", string(workflowErr.Key)),
				zap.Error(err))
		}
		return err
	}

	// Check if the step can still be transitioned (may have been completed by ExecuteWorkflowStep)
	canTransition, err = workflowSvc.CanTransition(ctx, args)
	if err != nil {
		return fmt.Errorf("failed to check transition status after execution: %w", err)
	}

	// Only complete the step if it's still pending and can be transitioned
	if canTransition {
		// If execution was successful, advance to the next step
		err = workflowSvc.CompleteWorkflowStep(ctx, args)
		if err != nil {
			ctx.Logger().Error("Failed to complete workflow step after successful execution",
				zap.Uint("requestID", args),
				zap.Error(err))
			return fmt.Errorf("failed to complete workflow step: %w", err)
		}
	}

	return nil
}
