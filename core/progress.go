package core

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// ProgressMode defines how progress is calculated and reported
type ProgressMode int

const (
	// ProgressModeManual allows manual percentage setting (legacy behavior)
	ProgressModeManual ProgressMode = iota
	// ProgressModeWorkUnits calculates progress based on completed work units
	ProgressModeWorkUnits
	// ProgressModeWeighted calculates progress based on weighted steps
	ProgressModeWeighted
)

// ProgressUpdate is a notification sent when progress changes
type ProgressUpdate struct {
	ProgressPercent float64    `json:"progress_percent"` // 0-100
	Message         string     `json:"message"`          // Human-readable status message
	StepName        string     `json:"step_name"`        // Current step name
	StepProgress    float64    `json:"step_progress"`    // Progress within current step (0-100)
	UpdatedAt       time.Time  `json:"updated_at"`       // When this update occurred
	Error           error      `json:"-"`                // Any error that occurred
}

// ProgressMessageProvider generates context-aware status messages
type ProgressMessageProvider interface {
	// GetStepMessage returns a message for the current step
	GetStepMessage(stepName string, progress float64) string
	
	// GetOverallMessage returns a message for the overall operation
	GetOverallMessage(progress float64) string
}

// DefaultProgressMessageProvider provides standard progress messages
// Uses step descriptions directly without adding prefixes
type DefaultProgressMessageProvider struct {
	operationName string // Used for overall fallback messages only
}

// NewDefaultProgressMessageProvider creates a message provider that uses step descriptions as-is
// The operationName is used for overall fallback messages when no step is active
func NewDefaultProgressMessageProvider(operationName string) *DefaultProgressMessageProvider {
	return &DefaultProgressMessageProvider{
		operationName: operationName,
	}
}

func (p *DefaultProgressMessageProvider) GetStepMessage(stepName string, progress float64) string {
	// Use the step name/description directly as the message
	// The step description should be a complete, human-readable message like "Acquiring SD blob from LBRY network"
	return stepName
}

func (p *DefaultProgressMessageProvider) GetOverallMessage(progress float64) string {
	if progress < 25 {
		return fmt.Sprintf("Initializing %s", p.operationName)
	} else if progress < 75 {
		return fmt.Sprintf("Processing %s", p.operationName)
	} else {
		return fmt.Sprintf("Finalizing %s", p.operationName)
	}
}

// SimpleProgressMessageProvider generates progress-based messages from simple step names
// For use when you don't want to write custom descriptions
type SimpleProgressMessageProvider struct {
	operationName string // Used for overall fallback messages only
}

// NewSimpleProgressMessageProvider creates a message provider that generates progress-based messages
// The operationName is used for overall fallback messages when no step is active
func NewSimpleProgressMessageProvider(operationName string) *SimpleProgressMessageProvider {
	return &SimpleProgressMessageProvider{
		operationName: operationName,
	}
}

func (p *SimpleProgressMessageProvider) GetStepMessage(stepName string, progress float64) string {
	// Generate messages based on progress for simple step names
	if progress < 25 {
		return fmt.Sprintf("Starting %s...", stepName)
	} else if progress < 75 {
		return fmt.Sprintf("Processing %s...", stepName)
	} else {
		return fmt.Sprintf("Completing %s...", stepName)
	}
}

func (p *SimpleProgressMessageProvider) GetOverallMessage(progress float64) string {
	if progress < 25 {
		return fmt.Sprintf("Initializing %s", p.operationName)
	} else if progress < 75 {
		return fmt.Sprintf("Processing %s", p.operationName)
	} else {
		return fmt.Sprintf("Finalizing %s", p.operationName)
	}
}

// ProgressCallback is called when progress changes
type ProgressCallback func(update ProgressUpdate)

// WorkUnit represents a single unit of work
type WorkUnit struct {
	Name     string  `json:"name"`
	Weight   float64 `json:"weight"`   // Relative weight (default 1.0)
	Complete bool    `json:"complete"` // Whether this unit is complete
}

// ProgressStep represents a step in a weighted progress calculation
type ProgressStep struct {
	Name        string            `json:"name"`
	Description string            `json:"description"` // Human-readable description of this step
	Weight      float64           `json:"weight"`      // Relative weight of this step in overall progress
	SubTracker  *ProgressTracker  `json:"-"`           // Nested tracker for this step
	progress    float64           `json:"-"`           // Cached progress for this step (0-100)
}

// ProgressTrackerConfig configures a ProgressTracker
type ProgressTrackerConfig struct {
	// Mode determines how progress is calculated
	Mode ProgressMode
	
	// TotalWorkUnits is the total number of work units (for WorkUnits mode)
	TotalWorkUnits int
	
	// Steps defines weighted steps (for Weighted mode)
	Steps []ProgressStep
	
	// MessageProvider generates context-aware messages
	MessageProvider ProgressMessageProvider
	
	// Callback is called when progress changes
	Callback ProgressCallback
	
	// RequestID is the workflow request ID for persistence
	RequestID uint
	
	// WorkflowService is used to persist progress updates
	WorkflowService WorkflowService
	
	// Logger for error logging
	Logger *Logger
}

// ProgressTracker tracks and reports operation progress
type ProgressTracker struct {
	config          ProgressTrackerConfig
	currentProgress float64
	currentStep     string
	stepProgress    float64
	workUnits       []WorkUnit
	completedUnits  int
	initialized     bool
}

// NewProgressTracker creates a new ProgressTracker
func NewProgressTracker(config ProgressTrackerConfig) (*ProgressTracker, error) {
	if config.RequestID == 0 {
		return nil, fmt.Errorf("RequestID is required")
	}
	if config.WorkflowService == nil {
		return nil, fmt.Errorf("WorkflowService is required")
	}
	if config.Mode == ProgressModeWorkUnits && config.TotalWorkUnits <= 0 {
		return nil, fmt.Errorf("TotalWorkUnits must be > 0 for WorkUnits mode")
	}
	if config.Mode == ProgressModeWeighted && len(config.Steps) == 0 {
		return nil, fmt.Errorf("Steps must be provided for Weighted mode")
	}
	
	if config.MessageProvider == nil {
		config.MessageProvider = NewDefaultProgressMessageProvider("")
	}
	
	return &ProgressTracker{
		config: config,
	}, nil
}

// Initialize prepares the tracker for use
func (t *ProgressTracker) Initialize() error {
	if t.initialized {
		return nil
	}
	
	switch t.config.Mode {
	case ProgressModeWorkUnits:
		t.workUnits = make([]WorkUnit, t.config.TotalWorkUnits)
		for i := range t.workUnits {
			t.workUnits[i] = WorkUnit{
				Name:     fmt.Sprintf("unit_%d", i),
				Weight:   1.0,
				Complete: false,
			}
		}
	case ProgressModeWeighted:
		// Initialize nested trackers for each step
		for i := range t.config.Steps {
			if t.config.Steps[i].SubTracker != nil {
				if err := t.config.Steps[i].SubTracker.Initialize(); err != nil {
					return fmt.Errorf("failed to initialize sub-tracker for step %s: %w", t.config.Steps[i].Name, err)
				}
			}
		}
	}
	
	t.initialized = true
	return nil
}

// SetProgress manually sets the progress percentage (Manual mode)
func (t *ProgressTracker) SetProgress(progress float64) error {
	if t.config.Mode != ProgressModeManual {
		return fmt.Errorf("SetProgress only available in Manual mode")
	}
	
	if progress < 0 {
		progress = 0
	} else if progress > 100 {
		progress = 100
	}
	
	t.currentProgress = progress
	return t.persistProgress()
}

// SetStep sets the current step and its progress (Manual mode)
func (t *ProgressTracker) SetStep(stepName string, stepProgress float64) error {
	if t.config.Mode != ProgressModeManual {
		return fmt.Errorf("SetStep only available in Manual mode")
	}
	
	if stepProgress < 0 {
		stepProgress = 0
	} else if stepProgress > 100 {
		stepProgress = 100
	}
	
	t.currentStep = stepName
	t.stepProgress = stepProgress
	return t.persistProgress()
}

// CompleteWorkUnit marks a work unit as complete (WorkUnits mode)
func (t *ProgressTracker) CompleteWorkUnit(unitIndex int) error {
	if t.config.Mode != ProgressModeWorkUnits {
		return fmt.Errorf("CompleteWorkUnit only available in WorkUnits mode")
	}
	
	if unitIndex < 0 || unitIndex >= len(t.workUnits) {
		return fmt.Errorf("invalid work unit index: %d", unitIndex)
	}
	
	if t.workUnits[unitIndex].Complete {
		return nil // Already complete
	}
	
	t.workUnits[unitIndex].Complete = true
	t.completedUnits++
	
	// Calculate progress
	t.currentProgress = float64(t.completedUnits) / float64(t.config.TotalWorkUnits) * 100
	t.currentStep = t.workUnits[unitIndex].Name
	t.stepProgress = 100
	
	return t.persistProgress()
}

// SetStepProgress sets progress for a specific step (Weighted mode)
func (t *ProgressTracker) SetStepProgress(stepName string, stepProgress float64) error {
	if t.config.Mode != ProgressModeWeighted {
		return fmt.Errorf("SetStepProgress only available in Weighted mode")
	}
	
	if stepProgress < 0 {
		stepProgress = 0
	} else if stepProgress > 100 {
		stepProgress = 100
	}
	
	// Find the step
	found := false
	var totalWeight float64
	var completedWeight float64
	
	for i := range t.config.Steps {
		step := &t.config.Steps[i]
		totalWeight += step.Weight
		
		if step.Name == stepName {
			found = true
			t.currentStep = stepName
			t.stepProgress = stepProgress
			step.progress = stepProgress
			completedWeight += step.Weight * (stepProgress / 100)
		} else if step.SubTracker != nil {
			// If this step has a sub-tracker, use its progress
			completedWeight += step.Weight * (step.SubTracker.GetProgress() / 100)
		} else {
			// Use cached progress for steps without sub-trackers
			completedWeight += step.Weight * (step.progress / 100)
		}
	}
	
	if !found {
		return fmt.Errorf("step not found: %s", stepName)
	}
	
	// Calculate overall progress
	if totalWeight > 0 {
		t.currentProgress = completedWeight / totalWeight * 100
	}
	
	return t.persistProgress()
}

// GetProgress returns the current progress percentage
func (t *ProgressTracker) GetProgress() float64 {
	return t.currentProgress
}

// GetStepProgress returns the current step and its progress
func (t *ProgressTracker) GetStepProgress() (string, float64) {
	return t.currentStep, t.stepProgress
}

// GetSubTracker returns the sub-tracker for a specific step (Weighted mode)
func (t *ProgressTracker) GetSubTracker(stepName string) (*ProgressTracker, error) {
	if t.config.Mode != ProgressModeWeighted {
		return nil, fmt.Errorf("GetSubTracker only available in Weighted mode")
	}
	
	for i := range t.config.Steps {
		if t.config.Steps[i].Name == stepName {
			return t.config.Steps[i].SubTracker, nil
		}
	}
	
	return nil, fmt.Errorf("step not found: %s", stepName)
}

// SetSubTracker sets a sub-tracker for a specific step (Weighted mode)
func (t *ProgressTracker) SetSubTracker(stepName string, subTracker *ProgressTracker) error {
	if t.config.Mode != ProgressModeWeighted {
		return fmt.Errorf("SetSubTracker only available in Weighted mode")
	}
	
	for i := range t.config.Steps {
		if t.config.Steps[i].Name == stepName {
			t.config.Steps[i].SubTracker = subTracker
			return nil
		}
	}
	
	return fmt.Errorf("step not found: %s", stepName)
}

// RefreshFromSubTrackers recalculates parent progress based on all sub-trackers (Weighted mode)
// This should be called after updating a sub-tracker's progress
func (t *ProgressTracker) RefreshFromSubTrackers() error {
	if t.config.Mode != ProgressModeWeighted {
		return fmt.Errorf("RefreshFromSubTrackers only available in Weighted mode")
	}
	
	var totalWeight float64
	var completedWeight float64
	
	for i := range t.config.Steps {
		step := &t.config.Steps[i]
		totalWeight += step.Weight
		
		if step.SubTracker != nil {
			// Use sub-tracker progress
			completedWeight += step.Weight * (step.SubTracker.GetProgress() / 100)
		} else {
			// Assume step is complete if it has no sub-tracker when refreshing
			completedWeight += step.Weight
		}
	}
	
	// Calculate overall progress
	if totalWeight > 0 {
		t.currentProgress = completedWeight / totalWeight * 100
	}
	
	return t.persistProgress()
}

// CreateSubTrackerForStep creates and sets a sub-tracker for a specific step (Weighted mode)
// This is a convenience method that combines CreateSubTracker and SetSubTracker
func (t *ProgressTracker) CreateSubTrackerForStep(stepName string, mode ProgressMode, configFunc func(*ProgressTrackerConfig)) (*ProgressTracker, error) {
	if t.config.Mode != ProgressModeWeighted {
		return nil, fmt.Errorf("CreateSubTrackerForStep only available in Weighted mode")
	}
	
	// Find the step
	var targetStep *ProgressStep
	for i := range t.config.Steps {
		if t.config.Steps[i].Name == stepName {
			targetStep = &t.config.Steps[i]
			break
		}
	}
	
	if targetStep == nil {
		return nil, fmt.Errorf("step not found: %s", stepName)
	}
	
	// Create sub-tracker using parent config
	subTracker, err := CreateSubTracker(t.config, mode, configFunc)
	if err != nil {
		return nil, fmt.Errorf("failed to create sub-tracker for step %s: %w", stepName, err)
	}
	
	// Set the sub-tracker for this step
	targetStep.SubTracker = subTracker
	
	return subTracker, nil
}

// getStepDescription returns the human-readable description for a step
// Falls back to the step name if no description is provided
func (t *ProgressTracker) getStepDescription(stepName string) string {
	if t.config.Mode == ProgressModeWeighted {
		for _, step := range t.config.Steps {
			if step.Name == stepName && step.Description != "" {
				return step.Description
			}
		}
	}
	// Fall back to step name for other modes or if no description
	return stepName
}

// persistProgress saves the progress to workflow data and notifies callbacks
func (t *ProgressTracker) persistProgress() error {
	// Create progress update
	update := ProgressUpdate{
		ProgressPercent: t.currentProgress,
		StepName:        t.currentStep,
		StepProgress:    t.stepProgress,
		UpdatedAt:       time.Now(),
	}
	
	// Generate messages
	if t.currentStep != "" {
		// Try to get step description for better messaging
		stepDescription := t.getStepDescription(t.currentStep)
		update.Message = t.config.MessageProvider.GetStepMessage(stepDescription, t.stepProgress)
	} else {
		update.Message = t.config.MessageProvider.GetOverallMessage(t.currentProgress)
	}
	
	// Persist to workflow data
	progressData := map[string]any{
		"progress_percent": t.currentProgress,
		"step_name":        t.currentStep,
		"step_progress":    t.stepProgress,
		"message":          update.Message,
		"updated_at":       update.UpdatedAt,
	}
	
	if err := t.config.WorkflowService.UpdateWorkflowData(context.Background(), t.config.RequestID, progressData); err != nil {
		if t.config.Logger != nil {
			t.config.Logger.Warn("Failed to persist progress to workflow data", 
				zap.Uint("request_id", t.config.RequestID),
				zap.Float64("progress", t.currentProgress),
				zap.Error(err),
			)
		}
		// Don't fail the operation for progress update failures
	}
	
	// Notify callback
	if t.config.Callback != nil {
		t.config.Callback(update)
	}
	
	return nil
}

// CreateSubTracker creates a nested progress tracker for a step
// The mode parameter determines how the sub-tracker calculates progress
// The configFunc parameter allows customization of the sub-tracker configuration
func CreateSubTracker(parentConfig ProgressTrackerConfig, mode ProgressMode, configFunc func(*ProgressTrackerConfig)) (*ProgressTracker, error) {
	subConfig := ProgressTrackerConfig{
		Mode:            mode,
		MessageProvider: parentConfig.MessageProvider,
		Callback:        parentConfig.Callback,
		RequestID:       parentConfig.RequestID,
		WorkflowService: parentConfig.WorkflowService,
		Logger:          parentConfig.Logger,
	}
	
	// Allow customization of the sub-tracker configuration
	if configFunc != nil {
		configFunc(&subConfig)
	}
	
	return NewProgressTracker(subConfig)
}

// ProgressTrackerHelper provides convenience methods for operations
type ProgressTrackerHelper struct {
	tracker *ProgressTracker
	ctx     Context
}

// NewProgressTrackerHelper creates a new helper with the given tracker
func NewProgressTrackerHelper(tracker *ProgressTracker, ctx Context) *ProgressTrackerHelper {
	return &ProgressTrackerHelper{
		tracker: tracker,
		ctx:     ctx,
	}
}

// RunWithProgress executes a function and updates progress
func (h *ProgressTrackerHelper) RunWithProgress(stepName string, stepProgress float64, fn func() error) error {
	if err := h.tracker.SetStep(stepName, stepProgress); err != nil {
		return err
	}
	
	if err := fn(); err != nil {
		return err
	}
	
	return nil
}

// RunWorkUnit executes a function and marks the work unit as complete
func (h *ProgressTrackerHelper) RunWorkUnit(unitIndex int, fn func() error) error {
	if err := fn(); err != nil {
		return err
	}
	
	return h.tracker.CompleteWorkUnit(unitIndex)
}

// RunStep executes a function for a weighted step
func (h *ProgressTrackerHelper) RunStep(stepName string, progress float64, fn func() error) error {
	if err := h.tracker.SetStepProgress(stepName, progress); err != nil {
		return err
	}
	
	if err := fn(); err != nil {
		return err
	}
	
	return nil
}

// NewProgressTrackerFromHelper creates a progress tracker from an OperationHelper
// The requestID parameter is required and must be non-zero
func NewProgressTrackerFromHelper(helper OperationHelper, requestID uint, mode ProgressMode, configFunc func(*ProgressTrackerConfig)) (*ProgressTracker, error) {
	cfg := ProgressTrackerConfig{
		Mode:            mode,
		RequestID:       requestID,
		WorkflowService: GetService[WorkflowService](helper.Context(), WORKFLOW_SERVICE),
		Logger:          helper.Logger(),
	}
	
	if configFunc != nil {
		configFunc(&cfg)
	}
	
	return NewProgressTracker(cfg)
}
