package core_tests

import (
	"errors"
	"testing"

	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/core/testing/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestDefaultProgressMessageProvider tests the DefaultProgressMessageProvider
func TestDefaultProgressMessageProvider(t *testing.T) {
	provider := core.NewDefaultProgressMessageProvider("TestOperation")

	t.Run("GetStepMessage returns step name directly", func(t *testing.T) {
		stepName := "Acquiring SD blob from LBRY network"
		message := provider.GetStepMessage(stepName, 50.0)
		assert.Equal(t, stepName, message)
	})

	t.Run("GetOverallMessage for initializing phase", func(t *testing.T) {
		message := provider.GetOverallMessage(10.0)
		assert.Equal(t, "Initializing TestOperation", message)
	})

	t.Run("GetOverallMessage for processing phase", func(t *testing.T) {
		message := provider.GetOverallMessage(50.0)
		assert.Equal(t, "Processing TestOperation", message)
	})

	t.Run("GetOverallMessage for finalizing phase", func(t *testing.T) {
		message := provider.GetOverallMessage(80.0)
		assert.Equal(t, "Finalizing TestOperation", message)
	})

	t.Run("GetOverallMessage boundary at 25", func(t *testing.T) {
		message := provider.GetOverallMessage(25.0)
		assert.Equal(t, "Processing TestOperation", message)
	})

	t.Run("GetOverallMessage boundary at 75", func(t *testing.T) {
		message := provider.GetOverallMessage(75.0)
		assert.Equal(t, "Finalizing TestOperation", message)
	})
}

// TestSimpleProgressMessageProvider tests the SimpleProgressMessageProvider
func TestSimpleProgressMessageProvider(t *testing.T) {
	provider := core.NewSimpleProgressMessageProvider("TestOperation")

	t.Run("GetStepMessage for starting phase", func(t *testing.T) {
		message := provider.GetStepMessage("upload", 10.0)
		assert.Equal(t, "Starting upload...", message)
	})

	t.Run("GetStepMessage for processing phase", func(t *testing.T) {
		message := provider.GetStepMessage("upload", 50.0)
		assert.Equal(t, "Processing upload...", message)
	})

	t.Run("GetStepMessage for completing phase", func(t *testing.T) {
		message := provider.GetStepMessage("upload", 90.0)
		assert.Equal(t, "Completing upload...", message)
	})

	t.Run("GetOverallMessage for initializing phase", func(t *testing.T) {
		message := provider.GetOverallMessage(10.0)
		assert.Equal(t, "Initializing TestOperation", message)
	})

	t.Run("GetOverallMessage for processing phase", func(t *testing.T) {
		message := provider.GetOverallMessage(50.0)
		assert.Equal(t, "Processing TestOperation", message)
	})

	t.Run("GetOverallMessage for finalizing phase", func(t *testing.T) {
		message := provider.GetOverallMessage(80.0)
		assert.Equal(t, "Finalizing TestOperation", message)
	})
}

// TestNewProgressTracker tests ProgressTracker creation and validation
func TestNewProgressTracker(t *testing.T) {
	ctx, err := coreTesting.NewTestContext(t)
	require.NoError(t, err)

	mockWorkflowService := mocks.NewMockWorkflowService(t)
	mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil).Maybe()

	t.Run("valid manual mode tracker", func(t *testing.T) {
		config := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeManual,
			RequestID:       123,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		tracker, err := core.NewProgressTracker(config)
		require.NoError(t, err)
		assert.NotNil(t, tracker)
	})

	t.Run("valid work units mode tracker", func(t *testing.T) {
		config := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeWorkUnits,
			TotalWorkUnits:  10,
			RequestID:       123,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		tracker, err := core.NewProgressTracker(config)
		require.NoError(t, err)
		assert.NotNil(t, tracker)
	})

	t.Run("valid weighted mode tracker", func(t *testing.T) {
		config := core.ProgressTrackerConfig{
			Mode: core.ProgressModeWeighted,
			Steps: []core.ProgressStep{
				{Name: "step1", Weight: 1.0},
				{Name: "step2", Weight: 2.0},
			},
			RequestID:       123,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		tracker, err := core.NewProgressTracker(config)
		require.NoError(t, err)
		assert.NotNil(t, tracker)
	})

	t.Run("error missing RequestID", func(t *testing.T) {
		config := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeManual,
			RequestID:       0,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		_, err := core.NewProgressTracker(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "RequestID is required")
	})

	t.Run("error missing WorkflowService", func(t *testing.T) {
		config := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeManual,
			RequestID:       123,
			WorkflowService: nil,
			Logger:          ctx.Logger(),
		}
		_, err := core.NewProgressTracker(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "WorkflowService is required")
	})

	t.Run("error work units mode with zero TotalWorkUnits", func(t *testing.T) {
		config := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeWorkUnits,
			TotalWorkUnits:  0,
			RequestID:       123,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		_, err := core.NewProgressTracker(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "TotalWorkUnits must be > 0")
	})

	t.Run("error weighted mode with no steps", func(t *testing.T) {
		config := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeWeighted,
			Steps:           []core.ProgressStep{},
			RequestID:       123,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		_, err := core.NewProgressTracker(config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Steps must be provided")
	})

	t.Run("default message provider when not set", func(t *testing.T) {
		config := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeManual,
			RequestID:       123,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		tracker, err := core.NewProgressTracker(config)
		require.NoError(t, err)
		assert.NotNil(t, tracker)
	})
}

// TestProgressTrackerManualMode tests manual progress tracking
func TestProgressTrackerManualMode(t *testing.T) {
	ctx, err := coreTesting.NewTestContext(t)
	require.NoError(t, err)

	mockWorkflowService := mocks.NewMockWorkflowService(t)
	mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)

	config := core.ProgressTrackerConfig{
		Mode:            core.ProgressModeManual,
		RequestID:       123,
		WorkflowService: mockWorkflowService,
		Logger:          ctx.Logger(),
		MessageProvider: core.NewDefaultProgressMessageProvider("TestOp"),
	}

	tracker, err := core.NewProgressTracker(config)
	require.NoError(t, err)

	t.Run("SetProgress clamps values to 0-100", func(t *testing.T) {
		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)
		err := tracker.SetProgress(-10.0)
		require.NoError(t, err)
		assert.Equal(t, 0.0, tracker.GetProgress())

		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)
		err = tracker.SetProgress(150.0)
		require.NoError(t, err)
		assert.Equal(t, 100.0, tracker.GetProgress())

		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)
		err = tracker.SetProgress(50.0)
		require.NoError(t, err)
		assert.Equal(t, 50.0, tracker.GetProgress())
	})

	t.Run("SetStep sets current step and progress", func(t *testing.T) {
		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)
		err := tracker.SetStep("uploading", 75.0)
		require.NoError(t, err)

		step, progress := tracker.GetStepProgress()
		assert.Equal(t, "uploading", step)
		assert.Equal(t, 75.0, progress)
	})

	t.Run("SetProgress fails in non-manual mode", func(t *testing.T) {
		config := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeWorkUnits,
			TotalWorkUnits:  10,
			RequestID:       123,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		tracker, _ := core.NewProgressTracker(config)
		err := tracker.SetProgress(50.0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "SetProgress only available in Manual mode")
	})

	t.Run("SetStep fails in non-manual mode", func(t *testing.T) {
		config := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeWorkUnits,
			TotalWorkUnits:  10,
			RequestID:       123,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		tracker, _ := core.NewProgressTracker(config)
		err := tracker.SetStep("step", 50.0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "SetStep only available in Manual mode")
	})
}

// TestProgressTrackerWorkUnitsMode tests work units progress tracking
func TestProgressTrackerWorkUnitsMode(t *testing.T) {
	ctx, err := coreTesting.NewTestContext(t)
	require.NoError(t, err)

	mockWorkflowService := mocks.NewMockWorkflowService(t)
	mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil).Maybe()

	t.Run("CompleteWorkUnit calculates progress correctly", func(t *testing.T) {
		config := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeWorkUnits,
			TotalWorkUnits:  10,
			RequestID:       123,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		tracker, err := core.NewProgressTracker(config)
		require.NoError(t, err)
		require.NoError(t, tracker.Initialize())

		// Complete first unit
		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)
		err = tracker.CompleteWorkUnit(0)
		require.NoError(t, err)
		assert.Equal(t, 10.0, tracker.GetProgress())

		// Complete second unit
		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)
		err = tracker.CompleteWorkUnit(1)
		require.NoError(t, err)
		assert.Equal(t, 20.0, tracker.GetProgress())

		// Complete remaining units
		for i := 2; i < 10; i++ {
			mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)
			err = tracker.CompleteWorkUnit(i)
			require.NoError(t, err)
		}
		assert.Equal(t, 100.0, tracker.GetProgress())
	})

	t.Run("CompleteWorkUnit handles invalid index", func(t *testing.T) {
		config := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeWorkUnits,
			TotalWorkUnits:  10,
			RequestID:       123,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		tracker, err := core.NewProgressTracker(config)
		require.NoError(t, err)
		require.NoError(t, tracker.Initialize())

		err = tracker.CompleteWorkUnit(-1)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid work unit index")

		err = tracker.CompleteWorkUnit(10)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid work unit index")
	})

	t.Run("CompleteWorkUnit is idempotent", func(t *testing.T) {
		config := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeWorkUnits,
			TotalWorkUnits:  5,
			RequestID:       123,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		tracker, err := core.NewProgressTracker(config)
		require.NoError(t, err)
		require.NoError(t, tracker.Initialize())

		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)
		err = tracker.CompleteWorkUnit(0)
		require.NoError(t, err)
		assert.Equal(t, 20.0, tracker.GetProgress())

		// Complete same unit again - should not change progress
		err = tracker.CompleteWorkUnit(0)
		require.NoError(t, err)
		assert.Equal(t, 20.0, tracker.GetProgress())
	})

	t.Run("CompleteWorkUnit fails in non-work-units mode", func(t *testing.T) {
		config := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeManual,
			RequestID:       123,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		tracker, _ := core.NewProgressTracker(config)
		err := tracker.CompleteWorkUnit(0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "CompleteWorkUnit only available in WorkUnits mode")
	})
}

// TestProgressTrackerWeightedMode tests weighted steps progress tracking
func TestProgressTrackerWeightedMode(t *testing.T) {
	ctx, err := coreTesting.NewTestContext(t)
	require.NoError(t, err)

	mockWorkflowService := mocks.NewMockWorkflowService(t)
	mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil).Maybe()

	t.Run("SetStepProgress calculates weighted progress correctly", func(t *testing.T) {
		config := core.ProgressTrackerConfig{
			Mode: core.ProgressModeWeighted,
			Steps: []core.ProgressStep{
				{Name: "step1", Weight: 1.0},
				{Name: "step2", Weight: 2.0},
				{Name: "step3", Weight: 1.0},
			},
			RequestID:       123,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		tracker, err := core.NewProgressTracker(config)
		require.NoError(t, err)

		// Set step1 to 100% - should be 100% (all steps assumed complete)
		// Note: In the current implementation, non-current steps without sub-trackers
		// are assumed to be complete, so setting any step to 100% results in 100% overall
		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)
		err = tracker.SetStepProgress("step1", 100.0)
		require.NoError(t, err)
		assert.InDelta(t, 100.0, tracker.GetProgress(), 0.01)

		// Set step2 to 50% - should be 75% overall (step1=100%, step2=50%, step3=100%)
		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)
		err = tracker.SetStepProgress("step2", 50.0)
		require.NoError(t, err)
		assert.InDelta(t, 75.0, tracker.GetProgress(), 0.01)

		// Set step3 to 50% - should be 87.5% overall (step1=100%, step2=100%, step3=50%)
		// Note: Non-current steps are assumed complete, so step2 is NOT remembered as 50%
		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)
		err = tracker.SetStepProgress("step3", 50.0)
		require.NoError(t, err)
		assert.InDelta(t, 87.5, tracker.GetProgress(), 0.01)

		// Set step2 to 100% - should be 100% overall (step1=100%, step2=100%, step3=100%)
		// Note: step3 is NOT remembered as 50%
		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)
		err = tracker.SetStepProgress("step2", 100.0)
		require.NoError(t, err)
		assert.InDelta(t, 100.0, tracker.GetProgress(), 0.01)
	})

	t.Run("SetStepProgress clamps values to 0-100", func(t *testing.T) {
		config := core.ProgressTrackerConfig{
			Mode: core.ProgressModeWeighted,
			Steps: []core.ProgressStep{
				{Name: "step1", Weight: 1.0},
			},
			RequestID:       123,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		tracker, err := core.NewProgressTracker(config)
		require.NoError(t, err)

		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)
		err = tracker.SetStepProgress("step1", -10.0)
		require.NoError(t, err)
		step, progress := tracker.GetStepProgress()
		assert.Equal(t, "step1", step)
		assert.Equal(t, 0.0, progress)

		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)
		err = tracker.SetStepProgress("step1", 150.0)
		require.NoError(t, err)
		step, progress = tracker.GetStepProgress()
		assert.Equal(t, "step1", step)
		assert.Equal(t, 100.0, progress)
	})

	t.Run("SetStepProgress handles unknown step", func(t *testing.T) {
		config := core.ProgressTrackerConfig{
			Mode: core.ProgressModeWeighted,
			Steps: []core.ProgressStep{
				{Name: "step1", Weight: 1.0},
			},
			RequestID:       123,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		tracker, err := core.NewProgressTracker(config)
		require.NoError(t, err)

		err = tracker.SetStepProgress("unknown_step", 50.0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "step not found")
	})

	t.Run("SetStepProgress fails in non-weighted mode", func(t *testing.T) {
		config := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeManual,
			RequestID:       123,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		tracker, _ := core.NewProgressTracker(config)
		err := tracker.SetStepProgress("step", 50.0)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "SetStepProgress only available in Weighted mode")
	})
}

// TestProgressTrackerPersistence tests progress persistence to workflow data
func TestProgressTrackerPersistence(t *testing.T) {
	ctx, err := coreTesting.NewTestContext(t)
	require.NoError(t, err)

	mockWorkflowService := mocks.NewMockWorkflowService(t)

	t.Run("persistProgress saves correct data", func(t *testing.T) {
		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, uint(123), mock.MatchedBy(func(data map[string]any) bool {
			return data["progress_percent"] == 50.0 && data["step_name"] == ""
		})).Return(nil)

		config := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeManual,
			RequestID:       123,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
			MessageProvider: core.NewDefaultProgressMessageProvider("TestOp"),
		}
		tracker, err := core.NewProgressTracker(config)
		require.NoError(t, err)

		err = tracker.SetProgress(50.0)
		require.NoError(t, err)
	})

	t.Run("persistProgress with step", func(t *testing.T) {
		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, uint(124), mock.MatchedBy(func(data map[string]any) bool {
			return data["step_name"] == "uploading" && data["step_progress"] == 75.0
		})).Return(nil)

		config := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeManual,
			RequestID:       124,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
			MessageProvider: core.NewDefaultProgressMessageProvider("TestOp"),
		}
		tracker, err := core.NewProgressTracker(config)
		require.NoError(t, err)

		err = tracker.SetStep("uploading", 75.0)
		require.NoError(t, err)
	})
}

// TestProgressTrackerCallbacks tests callback functionality
func TestProgressTrackerCallbacks(t *testing.T) {
	ctx, err := coreTesting.NewTestContext(t)
	require.NoError(t, err)

	mockWorkflowService := mocks.NewMockWorkflowService(t)

	t.Run("callback is called on progress update", func(t *testing.T) {
		var capturedUpdate core.ProgressUpdate
		callback := func(update core.ProgressUpdate) {
			capturedUpdate = update
		}

		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)

		config := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeManual,
			RequestID:       123,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
			Callback:        callback,
		}
		tracker, err := core.NewProgressTracker(config)
		require.NoError(t, err)

		err = tracker.SetProgress(50.0)
		require.NoError(t, err)

		assert.Equal(t, 50.0, capturedUpdate.ProgressPercent)
		assert.NotNil(t, capturedUpdate.UpdatedAt)
	})

	t.Run("nil callback is handled gracefully", func(t *testing.T) {
		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)

		config := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeManual,
			RequestID:       123,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
			Callback:        nil,
		}
		tracker, err := core.NewProgressTracker(config)
		require.NoError(t, err)

		err = tracker.SetProgress(50.0)
		require.NoError(t, err)
	})
}

// TestProgressTrackerInitialize tests initialization
func TestProgressTrackerInitialize(t *testing.T) {
	ctx, err := coreTesting.NewTestContext(t)
	require.NoError(t, err)

	mockWorkflowService := mocks.NewMockWorkflowService(t)

	t.Run("initialize is idempotent", func(t *testing.T) {
		config := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeManual,
			RequestID:       123,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		tracker, err := core.NewProgressTracker(config)
		require.NoError(t, err)

		err = tracker.Initialize()
		require.NoError(t, err)

		err = tracker.Initialize()
		require.NoError(t, err)
	})

	t.Run("initialize creates work units for WorkUnits mode", func(t *testing.T) {
		config := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeWorkUnits,
			TotalWorkUnits:  5,
			RequestID:       123,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		tracker, err := core.NewProgressTracker(config)
		require.NoError(t, err)

		err = tracker.Initialize()
		require.NoError(t, err)

		// Verify initialization works by completing a work unit
		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)
		err = tracker.CompleteWorkUnit(0)
		require.NoError(t, err)
		assert.Equal(t, 20.0, tracker.GetProgress())
	})
}

// TestProgressTrackerHelper tests the ProgressTrackerHelper
func TestProgressTrackerHelper(t *testing.T) {

	mockWorkflowService := mocks.NewMockWorkflowService(t)
	ctx, err := coreTesting.NewTestContext(t)
	require.NoError(t, err)

	t.Run("RunWithProgress executes function and updates progress", func(t *testing.T) {
		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)

		config := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeManual,
			RequestID:       123,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		tracker, err := core.NewProgressTracker(config)
		require.NoError(t, err)

		helper := core.NewProgressTrackerHelper(tracker, ctx)
		executed := false

		err = helper.RunWithProgress("step1", 50.0, func() error {
			executed = true
			return nil
		})

		require.NoError(t, err)
		assert.True(t, executed)
		step, progress := tracker.GetStepProgress()
		assert.Equal(t, "step1", step)
		assert.Equal(t, 50.0, progress)
	})

	t.Run("RunWithProgress returns error from function", func(t *testing.T) {
		config := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeManual,
			RequestID:       123,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		tracker, err := core.NewProgressTracker(config)
		require.NoError(t, err)

		helper := core.NewProgressTrackerHelper(tracker, ctx)

		err = helper.RunWithProgress("step1", 50.0, func() error {
			return errors.New("test error")
		})

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "test error")
	})

	t.Run("RunWorkUnit executes function and marks complete", func(t *testing.T) {
		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)

		config := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeWorkUnits,
			TotalWorkUnits:  5,
			RequestID:       123,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		tracker, err := core.NewProgressTracker(config)
		require.NoError(t, err)
		require.NoError(t, tracker.Initialize())

		helper := core.NewProgressTrackerHelper(tracker, ctx)
		executed := false

		err = helper.RunWorkUnit(0, func() error {
			executed = true
			return nil
		})

		require.NoError(t, err)
		assert.True(t, executed)
		assert.Equal(t, 20.0, tracker.GetProgress())
	})

	t.Run("RunStep executes function and updates step progress", func(t *testing.T) {
		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)

		config := core.ProgressTrackerConfig{
			Mode: core.ProgressModeWeighted,
			Steps: []core.ProgressStep{
				{Name: "step1", Weight: 1.0},
			},
			RequestID:       123,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		tracker, err := core.NewProgressTracker(config)
		require.NoError(t, err)

		helper := core.NewProgressTrackerHelper(tracker, ctx)
		executed := false

		err = helper.RunStep("step1", 50.0, func() error {
			executed = true
			return nil
		})

		require.NoError(t, err)
		assert.True(t, executed)
		assert.InDelta(t, 50.0, tracker.GetProgress(), 0.01)
	})
}

// TestCreateSubTracker tests creating sub-trackers
func TestCreateSubTracker(t *testing.T) {
	ctx, err := coreTesting.NewTestContext(t)
	require.NoError(t, err)

	mockWorkflowService := mocks.NewMockWorkflowService(t)

	t.Run("create manual mode sub-tracker", func(t *testing.T) {
		parentConfig := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeManual,
			RequestID:       123,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
			MessageProvider: core.NewDefaultProgressMessageProvider("ParentOp"),
		}

		subTracker, err := core.CreateSubTracker(parentConfig, core.ProgressModeManual, nil)
		require.NoError(t, err)
		assert.NotNil(t, subTracker)

		// Verify the sub-tracker can perform basic operations
		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)
		err = subTracker.SetProgress(50.0)
		require.NoError(t, err)
		assert.Equal(t, 50.0, subTracker.GetProgress())
	})

	t.Run("create work units mode sub-tracker", func(t *testing.T) {
		parentConfig := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeManual,
			RequestID:       124,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
			MessageProvider: core.NewDefaultProgressMessageProvider("ParentOp"),
		}

		subTracker, err := core.CreateSubTracker(parentConfig, core.ProgressModeWorkUnits, func(cfg *core.ProgressTrackerConfig) {
			cfg.TotalWorkUnits = 10
		})
		require.NoError(t, err)
		assert.NotNil(t, subTracker)

		require.NoError(t, subTracker.Initialize())

		// Verify the sub-tracker can complete work units
		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)
		err = subTracker.CompleteWorkUnit(0)
		require.NoError(t, err)
		assert.Equal(t, 10.0, subTracker.GetProgress())

		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)
		err = subTracker.CompleteWorkUnit(1)
		require.NoError(t, err)
		assert.Equal(t, 20.0, subTracker.GetProgress())
	})

	t.Run("create weighted mode sub-tracker", func(t *testing.T) {
		parentConfig := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeManual,
			RequestID:       125,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
			MessageProvider: core.NewDefaultProgressMessageProvider("ParentOp"),
		}

		subTracker, err := core.CreateSubTracker(parentConfig, core.ProgressModeWeighted, func(cfg *core.ProgressTrackerConfig) {
			cfg.Steps = []core.ProgressStep{
				{Name: "sub_step1", Weight: 1.0},
				{Name: "sub_step2", Weight: 2.0},
			}
		})
		require.NoError(t, err)
		assert.NotNil(t, subTracker)

		// Verify the sub-tracker can set step progress
		// Setting sub_step1 to 100% with weights [1.0, 2.0]:
		// Current implementation assumes non-current steps without sub-trackers are complete
		// So: step1=100% (current), step2=100% (assumed complete) = 100% overall
		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)
		err = subTracker.SetStepProgress("sub_step1", 100.0)
		require.NoError(t, err)
		assert.InDelta(t, 100.0, subTracker.GetProgress(), 0.01)

		// Setting sub_step2 to 50% with weights [1.0, 2.0]:
		// Current implementation: step1=100% (assumed complete), step2=50% (current)
		// Total weight = 3.0, completed = 1.0*1.0 + 2.0*0.5 = 2.0
		// Progress = 2.0/3.0 = 66.67%
		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)
		err = subTracker.SetStepProgress("sub_step2", 50.0)
		require.NoError(t, err)
		assert.InDelta(t, 66.67, subTracker.GetProgress(), 0.01)
	})

	t.Run("sub-tracker inherits parent config", func(t *testing.T) {
		var capturedUpdate core.ProgressUpdate
		callback := func(update core.ProgressUpdate) {
			capturedUpdate = update
		}

		parentConfig := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeManual,
			RequestID:       126,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
			MessageProvider: core.NewDefaultProgressMessageProvider("ParentOp"),
			Callback:        callback,
		}

		subTracker, err := core.CreateSubTracker(parentConfig, core.ProgressModeManual, nil)
		require.NoError(t, err)

		// Verify callback is inherited
		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)
		err = subTracker.SetProgress(75.0)
		require.NoError(t, err)

		assert.Equal(t, 75.0, capturedUpdate.ProgressPercent)
	})

	t.Run("sub-tracker error for work units without TotalWorkUnits", func(t *testing.T) {
		parentConfig := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeManual,
			RequestID:       127,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}

		_, err := core.CreateSubTracker(parentConfig, core.ProgressModeWorkUnits, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "TotalWorkUnits must be > 0")
	})

	t.Run("sub-tracker error for weighted mode without steps", func(t *testing.T) {
		parentConfig := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeManual,
			RequestID:       128,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}

		_, err := core.CreateSubTracker(parentConfig, core.ProgressModeWeighted, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Steps must be provided")
	})
}

// TestParentTrackerWithSubTrackers tests parent tracker methods for managing sub-trackers
func TestParentTrackerWithSubTrackers(t *testing.T) {
	ctx, err := coreTesting.NewTestContext(t)
	require.NoError(t, err)

	mockWorkflowService := mocks.NewMockWorkflowService(t)

	t.Run("GetSubTracker returns nil when not set", func(t *testing.T) {
		config := core.ProgressTrackerConfig{
			Mode: core.ProgressModeWeighted,
			Steps: []core.ProgressStep{
				{Name: "step1", Weight: 1.0},
			},
			RequestID:       200,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		tracker, err := core.NewProgressTracker(config)
		require.NoError(t, err)

		subTracker, err := tracker.GetSubTracker("step1")
		require.NoError(t, err)
		assert.Nil(t, subTracker)
	})

	t.Run("SetSubTracker sets and retrieves sub-tracker", func(t *testing.T) {
		config := core.ProgressTrackerConfig{
			Mode: core.ProgressModeWeighted,
			Steps: []core.ProgressStep{
				{Name: "step1", Weight: 1.0},
			},
			RequestID:       201,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		tracker, err := core.NewProgressTracker(config)
		require.NoError(t, err)

		// Create a sub-tracker
		subConfig := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeManual,
			RequestID:       201,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		subTracker, err := core.NewProgressTracker(subConfig)
		require.NoError(t, err)

		// Set it
		err = tracker.SetSubTracker("step1", subTracker)
		require.NoError(t, err)

		// Retrieve it
		retrieved, err := tracker.GetSubTracker("step1")
		require.NoError(t, err)
		assert.NotNil(t, retrieved)
		assert.Equal(t, subTracker, retrieved)
	})

	t.Run("SetSubTracker returns error for unknown step", func(t *testing.T) {
		config := core.ProgressTrackerConfig{
			Mode: core.ProgressModeWeighted,
			Steps: []core.ProgressStep{
				{Name: "step1", Weight: 1.0},
			},
			RequestID:       202,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		tracker, err := core.NewProgressTracker(config)
		require.NoError(t, err)

		subConfig := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeManual,
			RequestID:       202,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		subTracker, err := core.NewProgressTracker(subConfig)
		require.NoError(t, err)

		err = tracker.SetSubTracker("unknown_step", subTracker)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "step not found")
	})

	t.Run("GetSubTracker returns error for unknown step", func(t *testing.T) {
		config := core.ProgressTrackerConfig{
			Mode: core.ProgressModeWeighted,
			Steps: []core.ProgressStep{
				{Name: "step1", Weight: 1.0},
			},
			RequestID:       203,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		tracker, err := core.NewProgressTracker(config)
		require.NoError(t, err)

		_, err = tracker.GetSubTracker("unknown_step")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "step not found")
	})

	t.Run("RefreshFromSubTrackers recalculates with manual sub-tracker", func(t *testing.T) {
		config := core.ProgressTrackerConfig{
			Mode: core.ProgressModeWeighted,
			Steps: []core.ProgressStep{
				{Name: "step1", Weight: 1.0},
				{Name: "step2", Weight: 2.0},
			},
			RequestID:       204,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		tracker, err := core.NewProgressTracker(config)
		require.NoError(t, err)

		// Create and set a manual sub-tracker for step1
		subConfig := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeManual,
			RequestID:       204,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		subTracker, err := core.NewProgressTracker(subConfig)
		require.NoError(t, err)

		err = tracker.SetSubTracker("step1", subTracker)
		require.NoError(t, err)

		// Set sub-tracker progress to 50%
		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)
		err = subTracker.SetProgress(50.0)
		require.NoError(t, err)

		// Refresh parent - should be (0.5 * 1.0 + 1.0 * 2.0) / 3.0 = 83.33%
		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)
		err = tracker.RefreshFromSubTrackers()
		require.NoError(t, err)
		assert.InDelta(t, 83.33, tracker.GetProgress(), 0.01)
	})

	t.Run("RefreshFromSubTrackers with work units sub-tracker", func(t *testing.T) {
		config := core.ProgressTrackerConfig{
			Mode: core.ProgressModeWeighted,
			Steps: []core.ProgressStep{
				{Name: "step1", Weight: 1.0},
				{Name: "step2", Weight: 1.0},
			},
			RequestID:       205,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		tracker, err := core.NewProgressTracker(config)
		require.NoError(t, err)

		// Create and set a work units sub-tracker for step1
		subConfig := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeWorkUnits,
			TotalWorkUnits:  10,
			RequestID:       205,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		subTracker, err := core.NewProgressTracker(subConfig)
		require.NoError(t, err)
		require.NoError(t, subTracker.Initialize())

		err = tracker.SetSubTracker("step1", subTracker)
		require.NoError(t, err)

		// Complete 5 out of 10 work units (50%)
		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)
		err = subTracker.CompleteWorkUnit(0)
		require.NoError(t, err)
		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)
		err = subTracker.CompleteWorkUnit(1)
		require.NoError(t, err)
		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)
		err = subTracker.CompleteWorkUnit(2)
		require.NoError(t, err)
		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)
		err = subTracker.CompleteWorkUnit(3)
		require.NoError(t, err)
		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)
		err = subTracker.CompleteWorkUnit(4)
		require.NoError(t, err)

		// Refresh parent - should be (0.5 * 1.0 + 1.0 * 1.0) / 2.0 = 75%
		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)
		err = tracker.RefreshFromSubTrackers()
		require.NoError(t, err)
		assert.InDelta(t, 75.0, tracker.GetProgress(), 0.01)
	})

	t.Run("CreateSubTrackerForStep creates and sets sub-tracker", func(t *testing.T) {
		config := core.ProgressTrackerConfig{
			Mode: core.ProgressModeWeighted,
			Steps: []core.ProgressStep{
				{Name: "step1", Weight: 1.0},
			},
			RequestID:       206,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		tracker, err := core.NewProgressTracker(config)
		require.NoError(t, err)

		// Create sub-tracker for step1 with work units mode
		subTracker, err := tracker.CreateSubTrackerForStep("step1", core.ProgressModeWorkUnits, func(cfg *core.ProgressTrackerConfig) {
			cfg.TotalWorkUnits = 5
		})
		require.NoError(t, err)
		assert.NotNil(t, subTracker)

		// Verify it was set
		retrieved, err := tracker.GetSubTracker("step1")
		require.NoError(t, err)
		assert.Equal(t, subTracker, retrieved)

		// Verify sub-tracker works
		require.NoError(t, subTracker.Initialize())
		mockWorkflowService.EXPECT().UpdateWorkflowData(mock.Anything, mock.AnythingOfType("uint"), mock.Anything).Return(nil)
		err = subTracker.CompleteWorkUnit(0)
		require.NoError(t, err)
		assert.Equal(t, 20.0, subTracker.GetProgress())
	})

	t.Run("CreateSubTrackerForStep returns error for unknown step", func(t *testing.T) {
		config := core.ProgressTrackerConfig{
			Mode: core.ProgressModeWeighted,
			Steps: []core.ProgressStep{
				{Name: "step1", Weight: 1.0},
			},
			RequestID:       207,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		tracker, err := core.NewProgressTracker(config)
		require.NoError(t, err)

		_, err = tracker.CreateSubTrackerForStep("unknown_step", core.ProgressModeManual, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "step not found")
	})

	t.Run("GetSubTracker fails in non-weighted mode", func(t *testing.T) {
		config := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeManual,
			RequestID:       208,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		tracker, err := core.NewProgressTracker(config)
		require.NoError(t, err)

		_, err = tracker.GetSubTracker("step1")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "GetSubTracker only available in Weighted mode")
	})

	t.Run("SetSubTracker fails in non-weighted mode", func(t *testing.T) {
		config := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeManual,
			RequestID:       209,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		tracker, err := core.NewProgressTracker(config)
		require.NoError(t, err)

		subConfig := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeManual,
			RequestID:       209,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		subTracker, err := core.NewProgressTracker(subConfig)
		require.NoError(t, err)

		err = tracker.SetSubTracker("step1", subTracker)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "SetSubTracker only available in Weighted mode")
	})

	t.Run("RefreshFromSubTrackers fails in non-weighted mode", func(t *testing.T) {
		config := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeManual,
			RequestID:       210,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		tracker, err := core.NewProgressTracker(config)
		require.NoError(t, err)

		err = tracker.RefreshFromSubTrackers()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "RefreshFromSubTrackers only available in Weighted mode")
	})

	t.Run("CreateSubTrackerForStep fails in non-weighted mode", func(t *testing.T) {
		config := core.ProgressTrackerConfig{
			Mode:            core.ProgressModeManual,
			RequestID:       211,
			WorkflowService: mockWorkflowService,
			Logger:          ctx.Logger(),
		}
		tracker, err := core.NewProgressTracker(config)
		require.NoError(t, err)

		_, err = tracker.CreateSubTrackerForStep("step1", core.ProgressModeManual, nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "CreateSubTrackerForStep only available in Weighted mode")
	})
}
