package service

import (
	"fmt"
	"github.com/google/uuid"
	"go.lumeweb.com/portal/core"
	"go.uber.org/zap"
)

const workflowStepExecutorJobType = "core.workflow.step_executor"

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

	// args represents the workflow step requestID to execute
	args, ok := j.Args().(uint)
	if !ok {
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
	return workflowSvc.ExecuteWorkflowStep(ctx, args)
}
