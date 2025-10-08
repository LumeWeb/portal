package core

import (
	"net/http"
)

const workflowErrorNamespace = "workflow"

type WorkflowErrorType = ErrorType

const (
	// Workflow execution errors
	ErrKeyWorkflowStepRetried      WorkflowErrorType = "ErrWorkflowStepRetried"
	ErrKeyWorkflowExecutionFailed WorkflowErrorType = "ErrWorkflowExecutionFailed"
	ErrKeyWorkflowStepFailed      WorkflowErrorType = "ErrWorkflowStepFailed"
	
	// Workflow lifecycle errors
	ErrKeyWorkflowHasNoSteps       WorkflowErrorType = "ErrWorkflowHasNoSteps"
	ErrKeyWorkflowNotFound         WorkflowErrorType = "ErrWorkflowNotFound"
	ErrKeyWorkflowAlreadyExists    WorkflowErrorType = "ErrWorkflowAlreadyExists"
	ErrKeyWorkflowDisabled         WorkflowErrorType = "ErrWorkflowDisabled"
	
	// Workflow transition errors
	ErrKeyWorkflowCannotTransition WorkflowErrorType = "ErrWorkflowCannotTransition"
	ErrKeyWorkflowStepNotFound     WorkflowErrorType = "ErrWorkflowStepNotFound"
	
	// Workflow data errors
	ErrKeyWorkflowDataInvalid      WorkflowErrorType = "ErrWorkflowDataInvalid"
	ErrKeyWorkflowMetadataInvalid   WorkflowErrorType = "ErrWorkflowMetadataInvalid"
)

var defaultWorkflowErrorMessages = map[WorkflowErrorType]ErrorDefinition{
	// Workflow execution errors
	ErrKeyWorkflowStepRetried: {
		Key:     ErrKeyWorkflowStepRetried,
		Message: "Workflow step was retried due to failure",
	},
	ErrKeyWorkflowExecutionFailed: {
		Key:     ErrKeyWorkflowExecutionFailed,
		Message: "Workflow execution failed",
	},
	ErrKeyWorkflowStepFailed: {
		Key:     ErrKeyWorkflowStepFailed,
		Message: "Workflow step failed",
	},
	
	// Workflow lifecycle errors
	ErrKeyWorkflowHasNoSteps: {
		Key:     ErrKeyWorkflowHasNoSteps,
		Message: "Workflow has no steps defined",
	},
	ErrKeyWorkflowNotFound: {
		Key:     ErrKeyWorkflowNotFound,
		Message: "The requested workflow was not found",
	},
	ErrKeyWorkflowAlreadyExists: {
		Key:     ErrKeyWorkflowAlreadyExists,
		Message: "A workflow with this name already exists",
	},
	ErrKeyWorkflowDisabled: {
		Key:     ErrKeyWorkflowDisabled,
		Message: "The requested workflow is disabled",
	},
	
	// Workflow transition errors
	ErrKeyWorkflowCannotTransition: {
		Key:     ErrKeyWorkflowCannotTransition,
		Message: "Workflow step cannot be transitioned from its current state",
	},
	ErrKeyWorkflowStepNotFound: {
		Key:     ErrKeyWorkflowStepNotFound,
		Message: "The requested workflow step was not found",
	},
	
	// Workflow data errors
	ErrKeyWorkflowDataInvalid: {
		Key:     ErrKeyWorkflowDataInvalid,
		Message: "Invalid workflow data provided",
	},
	ErrKeyWorkflowMetadataInvalid: {
		Key:     ErrKeyWorkflowMetadataInvalid,
		Message: "Invalid workflow metadata format",
	},
}

var (
	workflowErrorCodeToHttpStatus = map[WorkflowErrorType]int{
		// Workflow execution errors
		ErrKeyWorkflowStepRetried:      http.StatusOK,      // Retries are expected behavior
		ErrKeyWorkflowExecutionFailed: http.StatusInternalServerError,
		ErrKeyWorkflowStepFailed:      http.StatusInternalServerError,
		
		// Workflow lifecycle errors
		ErrKeyWorkflowHasNoSteps:       http.StatusBadRequest,
		ErrKeyWorkflowNotFound:         http.StatusNotFound,
		ErrKeyWorkflowAlreadyExists:    http.StatusConflict,
		ErrKeyWorkflowDisabled:         http.StatusForbidden,
		
		// Workflow transition errors
		ErrKeyWorkflowCannotTransition: http.StatusConflict,
		ErrKeyWorkflowStepNotFound:     http.StatusNotFound,
		
		// Workflow data errors
		ErrKeyWorkflowDataInvalid:      http.StatusBadRequest,
		ErrKeyWorkflowMetadataInvalid:   http.StatusBadRequest,
	}
)

func init() {
	MustRegisterNamespace(workflowErrorNamespace)
	MustRegisterDefaultErrorMessages(workflowErrorNamespace, defaultWorkflowErrorMessages)
	MustRegisterErrorCodes(workflowErrorNamespace, workflowErrorCodeToHttpStatus)
}

// NewWorkflowError creates a new Error instance using the core error registry.
func NewWorkflowError(key WorkflowErrorType, err error, args ...interface{}) *Error {
	return NewError(workflowErrorNamespace, key, err, args...)
}

// IsWorkflowError checks if the error is a workflow error.
func IsWorkflowError(err error) bool {
	return IsNamespaceError(err, workflowErrorNamespace)
}

// AsWorkflowError casts the error to a Error if possible.
func AsWorkflowError(err error) *Error {
	if err == nil {
		return nil
	}
	e, ok := err.(*Error)
	if !ok {
		return nil
	}
	if !e.IsNamespace(workflowErrorNamespace) {
		return nil
	}
	return e
}

// IsWorkflowErrorType checks if the error is a specific workflow error type.
func IsWorkflowErrorType(err error, errorType WorkflowErrorType) bool {
	if err == nil {
		return false
	}
	e := AsWorkflowError(err)
	if e == nil {
		return false
	}
	return e.IsErrorType(errorType)
}
