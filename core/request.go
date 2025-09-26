package core

import (
	"context"
	"time"

	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/db/models/data_models"
	"go.lumeweb.com/queryutil/filter"
)

const REQUEST_SERVICE = "request"

// RequestFilter defines filtering options for requests and workflow instances
type RequestFilter struct {
	UserID       *uint
	Status       *models.RequestStatusType
	Protocol     *string
	Operation    *string
	SourceIP     *string
	Hash         []byte
	CIDType      *string
	CreatedAfter *time.Time
	UpdatedAfter *time.Time
	Limit        int
	Offset       int
}

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
	ComputeRequestStatus(ctx context.Context, id uint, keepExisting bool) (*RequestStatus, error)
	ComputeRequestStatusWithDeleted(ctx context.Context, id uint, keepExisting bool) (*RequestStatus, error)

	// Utility operations
	RequestExists(ctx context.Context, id uint) (bool, error)
	GetRequestWithDeleted(ctx context.Context, id uint) (*models.Request, error)
	ExecuteRequest(ctx context.Context, id uint) error

	// ListDistinctRequestFilters fetches distinct filter values for requests
	// (statuses, operations, protocols) from the database
	ListDistinctRequestFilters(ctx context.Context, userID uint, additionalFilters []filter.CrudFilter) (map[string][]string, error)

	Service
}

const (
	FilterRequestKeyStatuses   = "statuses"
	FilterRequestKeyOperations = "operations"
	FilterRequestKeyProtocols  = "protocols"
)

const (
	RequestFieldStatus    = "status"
	RequestFieldOperation = "operation"
	RequestFieldProtocol  = "protocol"
)

type RequestStatus struct {
	State           models.RequestStatusType `json:"state"`            // e.g., "pending", "processing", "completed", "failed"
	ProgressPercent float64                  `json:"progress_percent"` // 0-100 completion percentage
	Message         string                   `json:"message"`          // Human-readable status message
	UpdatedAt       time.Time                `json:"updated_at"`       // When status was last updated
	Error           error                    `json:"-"`                // Not serialized
}

// RequestStatusInfo contains human-readable display information for a request status
type RequestStatusInfo struct {
	Name        string `json:"name"`         // Human-readable display name
	Description string `json:"description"`  // Detailed description of the status
}

// RequestStatusDisplayNames maps request status types to their human-readable display information
var RequestStatusDisplayNames = map[models.RequestStatusType]RequestStatusInfo{
	models.RequestStatusPending:   {Name: "Pending", Description: "Waiting to be processed"},
	models.RequestStatusProcessing: {Name: "Processing", Description: "Currently being processed"},
	models.RequestStatusCompleted:  {Name: "Completed", Description: "Successfully completed"},
	models.RequestStatusFailed:     {Name: "Failed", Description: "Failed to complete"},
	models.RequestStatusDuplicate:  {Name: "Duplicate", Description: "Duplicate request"},
}

// GetRequestStatusDisplayInfo returns the human-readable display information for a given request status
func GetRequestStatusDisplayInfo(status models.RequestStatusType) (RequestStatusInfo, bool) {
	info, exists := RequestStatusDisplayNames[status]
	return info, exists
}

// GetDefaultStatusMessage returns the default status message for a given request status
func GetDefaultStatusMessage(status models.RequestStatusType) string {
	if info, exists := RequestStatusDisplayNames[status]; exists {
		return info.Description
	}
	return ""
}
