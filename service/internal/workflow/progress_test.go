package workflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
)

func TestCalculateProgress(t *testing.T) {
	tests := []struct {
		name                string
		totalSteps          int
		currentStepIndex    int
		currentStepProgress float64
		requestStatus       models.RequestStatusType
		expectedProgress    float64
	}{
		{
			name:                "completed workflow",
			totalSteps:          3,
			currentStepIndex:    2,
			currentStepProgress: 1.0,
			requestStatus:       models.RequestStatusCompleted,
			expectedProgress:    100,
		},
		{
			name:                "first step just started",
			totalSteps:          3,
			currentStepIndex:    0,
			currentStepProgress: 0.0,
			requestStatus:       models.RequestStatusProcessing,
			expectedProgress:    16.666666666666668, // 0 steps complete + 50% of first step
		},
		{
			name:                "first step 50% complete",
			totalSteps:          3,
			currentStepIndex:    0,
			currentStepProgress: 0.5,
			requestStatus:       models.RequestStatusProcessing,
			expectedProgress:    25, // 0 steps complete + 50% of first step + 25% of first step progress
		},
		{
			name:                "second step just started",
			totalSteps:          3,
			currentStepIndex:    1,
			currentStepProgress: 0.0,
			requestStatus:       models.RequestStatusProcessing,
			expectedProgress:    50, // 1 step complete (33.33%) + 50% of second step (16.66%)
		},
		{
			name:                "second step 50% complete",
			totalSteps:          3,
			currentStepIndex:    1,
			currentStepProgress: 0.5,
			requestStatus:       models.RequestStatusProcessing,
			expectedProgress:    58.333333333333336, // 1 step complete (33.33%) + 50% of second step (16.66%) + 8.33% progress
		},
		{
			name:                "third step just started",
			totalSteps:          3,
			currentStepIndex:    2,
			currentStepProgress: 0.0,
			requestStatus:       models.RequestStatusProcessing,
			expectedProgress:    83.33333333333334, // 2 steps complete (66.66%) + 50% of third step (16.66%)
		},
		{
			name:                "third step 50% complete",
			totalSteps:          3,
			currentStepIndex:    2,
			currentStepProgress: 0.5,
			requestStatus:       models.RequestStatusProcessing,
			expectedProgress:    91.66666666666667, // 2 steps complete (66.66%) + 50% of third step (16.66%) + 8.33% progress
		},
		{
			name:                "invalid total steps",
			totalSteps:          0,
			currentStepIndex:    0,
			currentStepProgress: 0.5,
			requestStatus:       models.RequestStatusProcessing,
			expectedProgress:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			progress := CalculateProgress(
				tt.totalSteps,
				tt.currentStepIndex,
				tt.currentStepProgress,
				tt.requestStatus,
			)
			assert.InDelta(t, tt.expectedProgress, progress, 0.0001)
		})
	}
}

func TestCalculateWorkflowStatusProgress(t *testing.T) {
	status := &core.WorkflowStatus{
		TotalSteps:  3,
		CurrentStep: 1, // Second step
	}

	reqStatus := &core.RequestStatus{
		ProgressPercent: 50,
		State:           models.RequestStatusProcessing,
	}

	progress := CalculateWorkflowStatusProgress(status, reqStatus)
	t.Logf("CalculateWorkflowStatusProgress: status=%+v reqStatus=%+v progress=%.2f", status, reqStatus, progress)
	// Expected progress calculation:
	// - 1 step completed (33.33%)
	// - Current step is 50% done (16.66% of total)
	// - Total = 33.33 + 16.66 = ~50%
	// But with our midpoint approach it's:
	// - 1 step completed (33.33%)
	// - Current step counts as half (16.66%)
	// - Plus 50% of current step progress (8.33%)
	// - Total = 33.33 + 16.66 + 8.33 = 58.33%
	assert.InDelta(t, 58.333333, progress, 0.0001)
}
