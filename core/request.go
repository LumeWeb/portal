package core

import (
	"context"
	"errors"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/db/models/data_models"
	"time"
)

const REQUEST_SERVICE = "request"

var (
	ErrDuplicateRequest = errors.New("duplicate request")
)

type RequestService interface {
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
	QueryRequest(ctx context.Context, query interface{}, filter RequestFilter) (*models.Request, error)
	GetRequestByHash(ctx context.Context, hash StorageHash, filter RequestFilter) (*models.Request, error)
	GetRequestByUploadHash(ctx context.Context, hash StorageHash, filter RequestFilter) (*models.Request, error)
	ListRequestsByUser(ctx context.Context, userID uint, filter RequestFilter) ([]*models.Request, error)
	ListRequestsByStatus(ctx context.Context, status string, filter RequestFilter) ([]*models.Request, error)

	// Status operations
	UpdateRequestStatus(ctx context.Context, id uint, status models.RequestStatusType) error
	CompleteRequest(ctx context.Context, id uint) error
	FailRequest(ctx context.Context, id uint, reason string) error
	GetRequestStatus(ctx context.Context, id uint) (*RequestStatus, error)

	// Utility operations
	RequestExists(ctx context.Context, id uint) (bool, error)

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
	State           string    // e.g., "pending", "processing", "completed", "failed"
	ProgressPercent float64   // 0-100 completion percentage
	Message         string    // Human-readable status message
	UpdatedAt       time.Time // When status was last updated
	Error           error     // Error if operation failed
}
