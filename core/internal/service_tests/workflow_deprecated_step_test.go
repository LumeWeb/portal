package service_tests

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/service"
	"gorm.io/datatypes"
)

// updateRequestMetadata is a helper that patches the metadata of a request
// in the database, simulating the state of a stranded request after a
// workflow refactoring.
func updateRequestMetadata(tb coreTesting.TB, ctx coreTesting.TestContext, reqID uint, metadata service.WorkflowMetadata) {
	updated, err := json.Marshal(metadata)
	require.NoError(tb, err)
	err = ctx.DB().Model(&models.Request{}).Where("id = ?", reqID).
		Update("metadata", datatypes.JSON(updated)).Error
	require.NoError(tb, err)
}

// setRequestStatus is a helper that updates a request's status in the DB.
func setRequestStatus(tb coreTesting.TB, ctx coreTesting.TestContext, reqID uint, status models.RequestStatusType) {
	err := ctx.DB().Model(&models.Request{}).Where("id = ?", reqID).
		Update("status", status).Error
	require.NoError(tb, err)
}

// TestGetWorkflowStatus_DeprecatedStepID_ReturnsLastStepIndex verifies that a
// stranded request (step ID no longer in workflow definition) returns a 0-based
// current step index pointing at the last step (TotalSteps - 1), not a 1-based
// count that exceeds valid indices.
func TestGetWorkflowStatus_DeprecatedStepID_ReturnsLastStepIndex(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		workflowName := "deprecatedStepStatusWorkflow"
		operationName := "test.operation.deprecated.status"

		steps := []core.OperationStep{
			{Operation: operationName, Foreground: true},
		}

		err := workflowService.RegisterWorkflow(workflowName, steps, false)
		require.NoError(tb, err)

		req, err := workflowService.StartWorkflow(context.Background(), workflowName,
			core.WithWorkflowData(map[string]interface{}{"test": "data"}))
		require.NoError(tb, err)
		require.NotNil(tb, req)

		var metadata service.WorkflowMetadata
		err = json.Unmarshal(req.Metadata, &metadata)
		require.NoError(tb, err)
		metadata.CurrentStepID = "step.that.no.longer.exists"
		updateRequestMetadata(tb, ctx, req.ID, metadata)

		status, err := workflowService.GetWorkflowStatus(context.Background(), req.ID)
		assert.NoError(tb, err)
		require.NotNil(tb, status)
		assert.Equal(tb, workflowName, status.WorkflowName)
		assert.Equal(tb, metadata.TotalSteps-1, status.CurrentStep,
			"CurrentStep should be TotalSteps-1 (0-based last step index)")
		assert.Equal(tb, metadata.TotalSteps, status.TotalSteps)
		assert.Less(t, status.CurrentStep, status.TotalSteps,
			"CurrentStep must never equal or exceed TotalSteps (0-based invariant)")
	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, service.NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, service.NewWorkflowCoordinator),
		withTestProtocol("test.operation.deprecated.status"),
	)
}

// TestGetWorkflowStatus_DeprecatedStepID_MultiStepRefactor verifies that when
// a multi-step workflow is refactored to have fewer steps, a stranded request
// with the old TotalSteps still uses metadata.TotalSteps for both index and
// total, and CurrentStep stays within [0, TotalSteps-1].
func TestGetWorkflowStatus_DeprecatedStepID_MultiStepRefactor(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		workflowName := "multiStepRefactorWorkflow"
		operationName := "test.operation.multistep.refactor"

		steps := []core.OperationStep{
			{Operation: operationName, Foreground: true},
		}

		err := workflowService.RegisterWorkflow(workflowName, steps, false)
		require.NoError(tb, err)

		req, err := workflowService.StartWorkflow(context.Background(), workflowName,
			core.WithWorkflowData(map[string]interface{}{"test": "data"}))
		require.NoError(tb, err)
		require.NotNil(tb, req)

		var metadata service.WorkflowMetadata
		err = json.Unmarshal(req.Metadata, &metadata)
		require.NoError(tb, err)
		metadata.CurrentStepID = "step.removed.in.refactor"
		metadata.TotalSteps = 3
		updateRequestMetadata(tb, ctx, req.ID, metadata)

		status, err := workflowService.GetWorkflowStatus(context.Background(), req.ID)
		assert.NoError(tb, err)
		require.NotNil(tb, status)
		assert.Equal(tb, 2, status.CurrentStep,
			"CurrentStep should be TotalSteps-1 (2), a 0-based index into the last step")
		assert.Equal(tb, 3, status.TotalSteps,
			"TotalSteps should be metadata.TotalSteps (3), not len(wf.Steps) (1)")
		assert.Less(t, status.CurrentStep, status.TotalSteps,
			"0-based invariant: CurrentStep < TotalSteps")
	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, service.NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, service.NewWorkflowCoordinator),
		withTestProtocol("test.operation.multistep.refactor"),
	)
}

// TestGetWorkflowStepInfo_DeprecatedStepID_ReturnsSyntheticInfo simulates a
// workflow refactoring where a step ID no longer exists. GetWorkflowStepInfo
// should return a synthetic WorkflowStepInfo using the request's own data
// instead of erroring.
func TestGetWorkflowStepInfo_DeprecatedStepID_ReturnsSyntheticInfo(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		workflowName := "deprecatedStepInfoWorkflow"
		operationName := "test.operation.deprecated.info"

		steps := []core.OperationStep{
			{Operation: operationName, FailureBehavior: core.RetryStep, Foreground: true},
		}

		err := workflowService.RegisterWorkflow(workflowName, steps, false)
		require.NoError(tb, err)

		req, err := workflowService.StartWorkflow(context.Background(), workflowName,
			core.WithWorkflowData(map[string]interface{}{"test": "data"}))
		require.NoError(tb, err)
		require.NotNil(tb, req)

		var metadata service.WorkflowMetadata
		err = json.Unmarshal(req.Metadata, &metadata)
		require.NoError(tb, err)
		metadata.CurrentStepID = "step.that.was.removed"
		updateRequestMetadata(tb, ctx, req.ID, metadata)

		info, err := workflowService.GetWorkflowStepInfo(context.Background(), req.ID)
		assert.NoError(tb, err)
		require.NotNil(tb, info)
		assert.Equal(tb, req.Operation, info.Operation)
		// FailureBehavior should be the zero value (FailWorkflow), not
		// fabricated RetryStep, since the original step's behavior is
		// unknowable from metadata after the step ID was removed.
		assert.Equal(tb, core.FailWorkflow, info.FailureBehavior)
		assert.Equal(tb, req.Status, info.Status)
	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, service.NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, service.NewWorkflowCoordinator),
		withTestProtocol("test.operation.deprecated.info"),
	)
}

// TestGetWorkflowStatus_DeprecatedStepID_ValidStepWorksNormally is a control test
// to ensure the resilience changes don't break normal (non-deprecated) step lookups.
func TestGetWorkflowStatus_DeprecatedStepID_ValidStepWorksNormally(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		workflowName := "validStepWorkflow"
		operationName := "test.operation.valid.step"

		steps := []core.OperationStep{
			{Operation: operationName, Foreground: true},
		}

		err := workflowService.RegisterWorkflow(workflowName, steps, false)
		require.NoError(tb, err)

		req, err := workflowService.StartWorkflow(context.Background(), workflowName,
			core.WithWorkflowData(map[string]interface{}{"test": "data"}))
		require.NoError(tb, err)
		require.NotNil(tb, req)

		status, err := workflowService.GetWorkflowStatus(context.Background(), req.ID)
		assert.NoError(tb, err)
		require.NotNil(tb, status)
		assert.Equal(tb, 0, status.CurrentStep, "valid step should return correct 0-based index")
		assert.Equal(tb, len(steps), status.TotalSteps)

		info, err := workflowService.GetWorkflowStepInfo(context.Background(), req.ID)
		assert.NoError(tb, err)
		require.NotNil(tb, info)
		assert.Equal(tb, operationName, info.Operation)
	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, service.NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, service.NewWorkflowCoordinator),
		withTestProtocol("test.operation.valid.step"),
	)
}

// TestDeprecatedStepID_WarnOnlyOnce verifies that repeated calls to
// GetWorkflowStatus and GetWorkflowStepInfo for a stranded request don't
// error and return consistent results. This simulates UI polling behavior.
func TestDeprecatedStepID_WarnOnlyOnce(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		workflowName := "warnOnceWorkflow"
		operationName := "test.operation.warn.once"

		steps := []core.OperationStep{
			{Operation: operationName, Foreground: true},
		}

		err := workflowService.RegisterWorkflow(workflowName, steps, false)
		require.NoError(tb, err)

		req, err := workflowService.StartWorkflow(context.Background(), workflowName,
			core.WithWorkflowData(map[string]interface{}{"test": "data"}))
		require.NoError(tb, err)
		require.NotNil(tb, req)

		var metadata service.WorkflowMetadata
		err = json.Unmarshal(req.Metadata, &metadata)
		require.NoError(tb, err)
		metadata.CurrentStepID = "step.stranded.warn.test"
		updateRequestMetadata(tb, ctx, req.ID, metadata)

		for i := 0; i < 5; i++ {
			status, err := workflowService.GetWorkflowStatus(context.Background(), req.ID)
			assert.NoError(tb, err, "GetWorkflowStatus call %d should not error", i)
			require.NotNil(tb, status)
			assert.Equal(tb, metadata.TotalSteps-1, status.CurrentStep,
				"call %d: CurrentStep should be TotalSteps-1", i)
			assert.Less(t, status.CurrentStep, status.TotalSteps,
				"call %d: 0-based invariant", i)

			info, err := workflowService.GetWorkflowStepInfo(context.Background(), req.ID)
			assert.NoError(tb, err, "GetWorkflowStepInfo call %d should not error", i)
			require.NotNil(tb, info)
			assert.Equal(tb, req.Operation, info.Operation)
		}
	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, service.NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, service.NewWorkflowCoordinator),
		withTestProtocol("test.operation.warn.once"),
	)
}

// ---------------------------------------------------------------------------
// Regression tests for design flaws identified during code review
// ---------------------------------------------------------------------------

// TestStranded_Regression_CurrentStepIsZeroBased is a regression test ensuring
// that a stranded request always returns CurrentStep as a 0-based index that
// never equals or exceeds TotalSteps. A previous version returned a 1-based
// count (TotalSteps), which broke consumers that treated CurrentStep as an
// array index and caused "step X of Y" UI text to show X > Y.
func TestStranded_Regression_CurrentStepIsZeroBased(t *testing.T) {
	for _, totalSteps := range []int{1, 2, 3, 5, 10} {
		totalSteps := totalSteps
		t.Run(fmt.Sprintf("TotalSteps=%d", totalSteps), func(t *testing.T) {
			coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
				workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
				require.NotNil(tb, workflowService)

				workflowName := fmt.Sprintf("zeroBasedWorkflow_%d", totalSteps)
				operationName := "test.operation.zerobased"

				// Register a workflow with only 1 step (post-refactor state)
				steps := make([]core.OperationStep, 1)
				steps[0] = core.OperationStep{Operation: operationName, Foreground: true}

				err := workflowService.RegisterWorkflow(workflowName, steps, false)
				require.NoError(tb, err)

				req, err := workflowService.StartWorkflow(context.Background(), workflowName,
					core.WithWorkflowData(map[string]interface{}{"test": "data"}))
				require.NoError(tb, err)
				require.NotNil(tb, req)

				var metadata service.WorkflowMetadata
				err = json.Unmarshal(req.Metadata, &metadata)
				require.NoError(tb, err)
				metadata.CurrentStepID = "step.removed.in.refactor"
				metadata.TotalSteps = totalSteps
				updateRequestMetadata(tb, ctx, req.ID, metadata)

				status, err := workflowService.GetWorkflowStatus(context.Background(), req.ID)
				assert.NoError(tb, err)
				require.NotNil(tb, status)
				assert.Equal(tb, totalSteps-1, status.CurrentStep,
					"CurrentStep must be TotalSteps-1 (0-based last-step index)")
				assert.Equal(tb, totalSteps, status.TotalSteps)
				assert.Less(t, status.CurrentStep, status.TotalSteps,
					"REGRESSION: CurrentStep must never equal or exceed TotalSteps")
			}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, service.NewRequestService),
				coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, service.NewWorkflowCoordinator),
				withTestProtocol("test.operation.zerobased"),
			)
		})
	}
}

// TestStranded_Regression_DoesNotMaskNonTerminalStatusAs100 is a regression
// test ensuring that a stranded request does NOT report 100% progress when its
// actual status is non-terminal (pending/processing). A previous version had
// an early return in CalculateProgress (`if currentStepIndex >= totalSteps`)
// that masked the real status and always rendered 100%, hiding unfinished work.
func TestStranded_Regression_DoesNotMaskNonTerminalStatusAs100(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		workflowName := "noMaskWorkflow"
		operationName := "test.operation.nomask"

		steps := []core.OperationStep{
			{Operation: operationName, Foreground: true},
		}

		err := workflowService.RegisterWorkflow(workflowName, steps, false)
		require.NoError(tb, err)

		req, err := workflowService.StartWorkflow(context.Background(), workflowName,
			core.WithWorkflowData(map[string]interface{}{"test": "data"}))
		require.NoError(tb, err)
		require.NotNil(tb, req)

		var metadata service.WorkflowMetadata
		err = json.Unmarshal(req.Metadata, &metadata)
		require.NoError(tb, err)
		metadata.CurrentStepID = "step.removed.in.refactor"
		metadata.TotalSteps = 3
		updateRequestMetadata(tb, ctx, req.ID, metadata)

		// --- Pending status: should NOT be 100% ---
		setRequestStatus(tb, ctx, req.ID, models.RequestStatusPending)
		status, err := workflowService.GetWorkflowStatus(context.Background(), req.ID)
		assert.NoError(tb, err)
		require.NotNil(tb, status)
		assert.Less(t, status.Progress, 100.0,
			"REGRESSION: pending stranded request must NOT report 100%% progress")

		// --- Processing status: should NOT be 100% ---
		setRequestStatus(tb, ctx, req.ID, models.RequestStatusProcessing)
		status, err = workflowService.GetWorkflowStatus(context.Background(), req.ID)
		assert.NoError(tb, err)
		require.NotNil(tb, status)
		assert.Less(t, status.Progress, 100.0,
			"REGRESSION: processing stranded request must NOT report 100%% progress")

		// --- Completed status: SHOULD be 100% ---
		setRequestStatus(tb, ctx, req.ID, models.RequestStatusCompleted)
		status, err = workflowService.GetWorkflowStatus(context.Background(), req.ID)
		assert.NoError(tb, err)
		require.NotNil(tb, status)
		assert.Equal(tb, 100.0, status.Progress,
			"completed stranded request should report 100%% progress")

		// --- Failed status: should NOT be 100% (failed is not completed) ---
		setRequestStatus(tb, ctx, req.ID, models.RequestStatusFailed)
		status, err = workflowService.GetWorkflowStatus(context.Background(), req.ID)
		assert.NoError(tb, err)
		require.NotNil(tb, status)
		assert.Less(t, status.Progress, 100.0,
			"REGRESSION: failed stranded request must NOT report 100%% progress")
	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, service.NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, service.NewWorkflowCoordinator),
		withTestProtocol("test.operation.nomask"),
	)
}

// TestStranded_Regression_TotalStepsZeroFallsBackToWorkflowSteps is a
// regression test ensuring that when metadata.TotalSteps <= 0 (e.g., older
// metadata that never populated this field), the stranded fallback uses
// len(wf.Steps) and still maintains the 0-based invariant.
func TestStranded_Regression_TotalStepsZeroFallsBackToWorkflowSteps(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		workflowName := "zeroTotalWorkflow"
		operationName := "test.operation.zerofallback"

		steps := []core.OperationStep{
			{Operation: operationName, Foreground: true},
		}

		err := workflowService.RegisterWorkflow(workflowName, steps, false)
		require.NoError(tb, err)

		req, err := workflowService.StartWorkflow(context.Background(), workflowName,
			core.WithWorkflowData(map[string]interface{}{"test": "data"}))
		require.NoError(tb, err)
		require.NotNil(tb, req)

		var metadata service.WorkflowMetadata
		err = json.Unmarshal(req.Metadata, &metadata)
		require.NoError(tb, err)
		metadata.CurrentStepID = "step.removed.in.refactor"
		metadata.TotalSteps = 0
		updateRequestMetadata(tb, ctx, req.ID, metadata)

		status, err := workflowService.GetWorkflowStatus(context.Background(), req.ID)
		assert.NoError(tb, err)
		require.NotNil(tb, status)
		assert.Equal(tb, len(steps)-1, status.CurrentStep,
			"CurrentStep should be len(wf.Steps)-1 when TotalSteps==0")
		assert.Equal(tb, len(steps), status.TotalSteps,
			"TotalSteps should fall back to len(wf.Steps) when metadata.TotalSteps==0")
		assert.Less(t, status.CurrentStep, status.TotalSteps,
			"0-based invariant must hold even with TotalSteps fallback")
	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, service.NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, service.NewWorkflowCoordinator),
		withTestProtocol("test.operation.zerofallback"),
	)
}

// TestStranded_Regression_EffectiveTotalStepsConsistency verifies that
// CurrentStep and TotalSteps are derived from the same value so they stay
// consistent — no mismatch where CurrentStep is computed from one source
// (metadata.TotalSteps) and TotalSteps from another (len(wf.Steps)).
func TestStranded_Regression_EffectiveTotalStepsConsistency(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		workflowName := "consistencyWorkflow"
		operationName := "test.operation.consistency"

		// Register a workflow with 1 step (post-refactor), but set
		// metadata.TotalSteps = 4 (pre-refactor). The stranded fallback
		// should use 4 for BOTH CurrentStep (3) and TotalSteps (4),
		// not mix 4 for index and 1 for total.
		steps := []core.OperationStep{
			{Operation: operationName, Foreground: true},
		}

		err := workflowService.RegisterWorkflow(workflowName, steps, false)
		require.NoError(tb, err)

		req, err := workflowService.StartWorkflow(context.Background(), workflowName,
			core.WithWorkflowData(map[string]interface{}{"test": "data"}))
		require.NoError(tb, err)
		require.NotNil(tb, req)

		var metadata service.WorkflowMetadata
		err = json.Unmarshal(req.Metadata, &metadata)
		require.NoError(tb, err)
		metadata.CurrentStepID = "step.removed.in.refactor"
		metadata.TotalSteps = 4
		updateRequestMetadata(tb, ctx, req.ID, metadata)

		status, err := workflowService.GetWorkflowStatus(context.Background(), req.ID)
		assert.NoError(tb, err)
		require.NotNil(tb, status)
		assert.Equal(tb, 3, status.CurrentStep,
			"CurrentStep must be metadata.TotalSteps-1 (3), not len(wf.Steps)-1 (0)")
		assert.Equal(tb, 4, status.TotalSteps,
			"TotalSteps must be metadata.TotalSteps (4), not len(wf.Steps) (1)")
		assert.Equal(tb, status.CurrentStep, status.TotalSteps-1,
			"REGRESSION: CurrentStep and TotalSteps must be derived from the same source")
	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, service.NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, service.NewWorkflowCoordinator),
		withTestProtocol("test.operation.consistency"),
	)
}

// TestStranded_Regression_EmptyWorkflowNoNegativeIndex ensures that when a
// workflow has been refactored so ALL steps moved out (len(Steps)==0) and
// metadata.TotalSteps was never populated (0), getCurrentStepIndex does NOT
// return a negative CurrentStep. A previous version could return -1 when
// both metadata.TotalSteps and len(wf.Steps) were 0, violating the 0-based
// index invariant [0, TotalSteps-1].
func TestStranded_Regression_EmptyWorkflowNoNegativeIndex(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		workflowService := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		require.NotNil(tb, workflowService)

		workflowName := "emptyWorkflow"
		operationName := "test.operation.empty"

		// Register a workflow with 1 step so we can start a request.
		steps := []core.OperationStep{
			{Operation: operationName, Foreground: true},
		}

		err := workflowService.RegisterWorkflow(workflowName, steps, false)
		require.NoError(tb, err)

		req, err := workflowService.StartWorkflow(context.Background(), workflowName,
			core.WithWorkflowData(map[string]interface{}{"test": "data"}))
		require.NoError(tb, err)
		require.NotNil(tb, req)

		var metadata service.WorkflowMetadata
		err = json.Unmarshal(req.Metadata, &metadata)
		require.NoError(tb, err)
		metadata.CurrentStepID = "step.removed.in.refactor"
		metadata.TotalSteps = 0
		updateRequestMetadata(tb, ctx, req.ID, metadata)

		// Simulate a full refactoring where all steps were moved out by
		// directly emptying the workflow definition's Steps slice. This
		// bypasses RegisterWorkflow's validation (which rejects 0 steps)
		// to test the defensive guard in getCurrentStepIndex.
		coord, ok := workflowService.(*service.WorkflowCoordinatorDefault)
		require.True(t, ok, "workflowService should be *WorkflowCoordinatorDefault")
		coord.EmptyStepsForTesting(workflowName)

		status, err := workflowService.GetWorkflowStatus(context.Background(), req.ID)
		assert.NoError(tb, err)
		require.NotNil(tb, status)
		assert.GreaterOrEqual(t, status.CurrentStep, 0,
			"REGRESSION: CurrentStep must never be negative")
		assert.GreaterOrEqual(t, status.TotalSteps, 1,
			"TotalSteps must be at least 1 (floored by getCurrentStepIndex)")
		assert.Less(t, status.CurrentStep, status.TotalSteps,
			"0-based invariant: CurrentStep < TotalSteps must hold even for empty workflows")
	}, coreTesting.WithServiceFactory(core.REQUEST_SERVICE, service.NewRequestService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, service.NewWorkflowCoordinator),
		withTestProtocol("test.operation.empty"),
	)
}
