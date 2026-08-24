package service

import (
	"context"
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

func (j *workflowStepExecutorJob) Run(ctx core.Context, eventCtx context.Context) error {
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

	// eventCtx carries the trace span from ExecuteJob. Use it for all
	// workflow service calls so spans connect to the cron execution trace.
	// eventCtx also preserves portal context values (tracer service, etc.)
	// through DetachContext, so TraceMethod inside workflow methods will
	// create child spans of the cron job span.
	execCtx := eventCtx

	// Check if the step can be transitioned
	canTransition, err := workflowSvc.CanTransition(execCtx, args)
	if err != nil {
		return fmt.Errorf("failed to check transition status: %w", err)
	}

	if !canTransition {
		// Log and exit if the step cannot be transitioned
		ctx.Logger().Info("Skipping workflow step execution",
			zap.Uint("requestID", args))
		return nil
	}

	ctx.Logger().Debug("workflow step execution started",
		zap.Uint("requestID", args),
		zap.NamedError("ctxErr", execCtx.Err()))

	// Execute the workflow step
	err = workflowSvc.ExecuteWorkflowStep(execCtx, args)
	if err != nil {
		// Check if this was a retried error (which is expected behavior)
		if core.IsWorkflowErrorType(err, core.ErrKeyWorkflowStepRetried) {
			// Log the retry but don't fail the cron job
			ctx.Logger().Info("Workflow step retried",
				zap.Uint("requestID", args),
				zap.Error(err))
			return nil
		}

		// Check if step is configured to continue on failure
		stepInfo, stepErr := workflowSvc.GetWorkflowStepInfo(execCtx, args)
		if stepErr != nil {
			return fmt.Errorf("failed to get workflow step info: %w", stepErr)
		}

		// If the step is configured to continue on failure, we should still advance to the next step
		if stepInfo.FailureBehavior == core.ContinueWorkflow {
			ctx.Logger().Info("Workflow step failed but configured to continue",
				zap.Uint("requestID", args),
				zap.Error(err))
		} else {
			// Log the error key before wrapping to preserve context
			if workflowErr := core.AsWorkflowError(err); workflowErr != nil {
				ctx.Logger().Error("Workflow step execution failed",
					zap.Uint("requestID", args),
					zap.String("errorKey", string(workflowErr.Key)),
					zap.Error(err))
			}
			return err
		}
	}

	ctx.Logger().Debug("workflow step execution completed",
		zap.Uint("requestID", args),
		zap.NamedError("execErr", err),
		zap.NamedError("ctxErr", execCtx.Err()))

	// Check if the step can still be transitioned (may have been completed by ExecuteWorkflowStep)
	canTransition, err = workflowSvc.CanTransition(execCtx, args)
	if err != nil {
		return fmt.Errorf("failed to check transition status after execution: %w", err)
	}

	// Only complete the step if it's still pending and can be transitioned
	if canTransition {
		ctx.Logger().Debug("completing workflow step",
			zap.Uint("requestID", args))
		// If execution was successful (or we're continuing despite failure), advance to the next step
		err = workflowSvc.CompleteWorkflowStep(execCtx, args)
		if err != nil {
			ctx.Logger().Error("Failed to complete workflow step after execution",
				zap.Uint("requestID", args),
				zap.Error(err))
			return fmt.Errorf("failed to complete workflow step: %w", err)
		}
	}

	return nil
}
