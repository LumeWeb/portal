package workflow

import (
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
)

// CalculateProgress computes the overall workflow progress percentage based on:
// - Total number of steps in the workflow
// - Current step index (0-based)
// - Progress within current step (0.0 to 1.0)
// - Current request status
//
// The algorithm uses a weighted approach where:
// - Each completed step contributes 100/totalSteps percentage points
// - The current step (if processing) contributes:
//   - Half its weight immediately when step starts (50% of step weight)
//   - The remaining half based on currentStepProgress
//
// Examples:
// 1. 3-step workflow, step 1 just started (0% progress):
//    - 0 steps complete (0%)
//    - Step 1 contributes 16.66% (half of 33.33%)
//    - Total progress: 16.66%
//
// 2. 3-step workflow, step 1 50% complete:
//    - 0 steps complete (0%) 
//    - Step 1 contributes 16.66% (half) + 8.33% (half of remaining 16.66%)
//    - Total progress: 25%
//
// 3. 3-step workflow, step 2 just started:
//    - 1 step complete (33.33%)
//    - Step 2 contributes 16.66% (half of 33.33%)
//    - Total progress: 50%
//
// 4. Completed workflow always returns 100%
// 5. Pending workflow always returns 0%
//
// Edge cases:
// - If totalSteps <= 0, returns 0
// - If currentStepIndex is negative, treated as 0
// - Progress is clamped between 0 and 100
func CalculateProgress(
	totalSteps int,
	currentStepIndex int,
	currentStepProgress float64,
	requestStatus models.RequestStatusType,
) float64 {
	if totalSteps <= 0 {
		return 0
	}

	// For completed requests, report 100%
	if requestStatus == models.RequestStatusCompleted {
		return 100
	}

	// For pending requests, report 0% progress
	if requestStatus == models.RequestStatusPending {
		return 0
	}

	// Calculate progress based on:
	// - Completed steps (currentStepIndex)
	// - Current step progress (if processing)
	progress := 0.0
	if currentStepIndex < 0 {
		currentStepIndex = 0
	}

	// Clamp currentStepProgress to [0,1]
	if currentStepProgress < 0 {
		currentStepProgress = 0
	} else if currentStepProgress > 1 {
		currentStepProgress = 1
	}

	// Each step contributes 100/totalSteps percentage points
	stepWeight := 100.0 / float64(totalSteps)

	// Progress from completed steps
	progress = float64(currentStepIndex) * stepWeight

	// Add progress from current step if processing
	if requestStatus == models.RequestStatusProcessing {
		// Add half the step weight immediately when step starts
		progress += stepWeight / 2
		// Add remaining progress based on current step progress
		progress += currentStepProgress * (stepWeight / 2)
	}

	// Ensure progress is within bounds
	if progress < 0 {
		return 0
	}
	if progress > 100 {
		return 100
	}

	return progress
}

// CalculateWorkflowStatusProgress computes progress for a workflow status by delegating
// to CalculateProgress with values from the workflow status and request status.
//
// This is a convenience wrapper that extracts:
// - TotalSteps from WorkflowStatus
// - CurrentStep from WorkflowStatus  
// - ProgressPercent (converted to 0.0-1.0) from RequestStatus
// - State from RequestStatus
//
// Example:
//   status := &WorkflowStatus{TotalSteps: 3, CurrentStep: 1}
//   reqStatus := &RequestStatus{ProgressPercent: 50, State: "processing"}
//   progress := CalculateWorkflowStatusProgress(status, reqStatus)
//   // progress = 58.33% (1 step complete + 50% of current step progress)
func CalculateWorkflowStatusProgress(
	status *core.WorkflowStatus,
	reqStatus *core.RequestStatus,
) float64 {
	// Handle nil inputs defensively
	if status == nil || reqStatus == nil {
		return 0.0
	}

	// Safely extract fields with defaults
	totalSteps := 0
	if status.TotalSteps > 0 {
		totalSteps = status.TotalSteps
	}

	currentStep := 0
	if status.CurrentStep > 0 {
		currentStep = status.CurrentStep
	}

	progress := 0.0
	if reqStatus.ProgressPercent > 0 {
		progress = reqStatus.ProgressPercent / 100.0
	}

	state := models.RequestStatusPending
	if reqStatus.State != "" {
		state = reqStatus.State
	}

	return CalculateProgress(
		totalSteps,
		currentStep,
		progress,
		state,
	)
}
