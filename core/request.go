package core

import (
	"context"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/db/models/data_models"
	"time"
)

const REQUEST_SERVICE = "request"

type RequestService interface {
	// Request validation
	ValidateRequest(ctx context.Context, req *models.Request) error

	// Model registration methods
	// RegisterRequestModel registers a protocol-specific request data model for an operation
	RegisterRequestModel(operation string, model data_models.RequestDataModel)
	// GetRequestModel retrieves the registered model for an operation
	GetRequestModel(operation string) (data_models.RequestDataModel, bool)
	// CreateRequestModel creates a new instance of the registered model for an operation
	CreateRequestModel(operation string) (data_models.RequestDataModel, error)

	// Core CRUD operations
	CreateRequest(ctx context.Context, req *models.Request, data interface{}) (*models.Request, error)
	GetRequest(ctx context.Context, id uint) (*models.Request, error)
	GetRequestData(ctx context.Context, req *models.Request) (interface{}, error)
	UpdateRequest(ctx context.Context, req *models.Request) error
	UpdateRequestData(ctx context.Context, req *models.Request, data interface{}) error
	DeleteRequest(ctx context.Context, id uint) error

	// Query operations
	QueryRequest(ctx context.Context, query any, filter RequestFilter) (*models.Request, error)
	QueryRequestData(ctx context.Context, query any, filter RequestFilter) (*models.Request, error)
	GetRequestByHash(ctx context.Context, hash StorageHash, filter RequestFilter) (*models.Request, error)
	ListRequestsByUser(ctx context.Context, userID uint, filter RequestFilter) ([]*models.Request, error)
	ListRequestsByStatus(ctx context.Context, status string, filter RequestFilter) ([]*models.Request, error)

	// Status operations
	UpdateRequestStatus(ctx context.Context, id uint, status models.RequestStatusType, message string) error
	CompleteRequest(ctx context.Context, id uint) error
	FailRequest(ctx context.Context, id uint, reason string) error
	GetRequestStatus(ctx context.Context, id uint, withDeleted bool) (*RequestStatus, error)
	ComputeRequestStatus(ctx context.Context, id uint) (*RequestStatus, error)
	ComputeRequestStatusWithDeleted(ctx context.Context, id uint) (*RequestStatus, error)

	// Utility operations
	RequestExists(ctx context.Context, id uint) (bool, error)
	GetRequestWithDeleted(ctx context.Context, id uint) (*models.Request, error)
	ExecuteRequest(ctx context.Context, id uint) error

	Service
}

type RequestFilter struct {
	Protocol  string
	Operation string
	UserID    uint
	Limit     int
	Offset    int
}

type RequestStatus struct {
	State           models.RequestStatusType // e.g., "pending", "processing", "completed", "failed"
	ProgressPercent float64                  // 0-100 completion percentage
	Message         string                   // Human-readable status message
	UpdatedAt       time.Time                // When status was last updated
	Error           error                    // Error if operation failed
}

// GetDefaultStatusMessage returns the default status message for a given request status
func GetDefaultStatusMessage(status models.RequestStatusType) string {
	switch status {
	case models.RequestStatusPending:
		return "Request is pending processing"
	case models.RequestStatusProcessing:
		return "Request is being processed"
	case models.RequestStatusCompleted:
		return "Request completed successfully"
	case models.RequestStatusFailed:
		return "Request failed"
	default:
		return ""
	}
}
