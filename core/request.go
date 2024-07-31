package core

import (
	"context"
	"go.lumeweb.com/portal/db/models"
)

const REQUEST_SERVICE = "request"

type RequestService interface {
	// Core CRUD operations
	CreateRequest(ctx context.Context, req *models.Request, protocolData any) (*models.Request, error)
	GetRequest(ctx context.Context, id uint) (*models.Request, error)
	UpdateRequest(ctx context.Context, req *models.Request) error
	DeleteRequest(ctx context.Context, id uint) error
	QueryRequest(ctx context.Context, query interface{}, filter RequestFilter) (*models.Request, error)
	CompleteRequest(ctx context.Context, id uint) error

	// Query operations
	GetRequestByHash(ctx context.Context, hash StorageHash, filter RequestFilter) (*models.Request, error)
	GetRequestByUploadHash(ctx context.Context, hash StorageHash, filter RequestFilter) (*models.Request, error)
	ListRequestsByUser(ctx context.Context, userID uint, filter RequestFilter) ([]*models.Request, error)
	ListRequestsByStatus(ctx context.Context, status string, filter RequestFilter) ([]*models.Request, error)

	// Status operations
	UpdateRequestStatus(ctx context.Context, id uint, status models.RequestStatusType) error

	// Protocol data operations
	UpdateProtocolData(ctx context.Context, id uint, data any) error
	GetProtocolData(ctx context.Context, id uint) (any, error)
	QueryProtocolData(ctx context.Context, protocol string, query any, filter RequestFilter) (any, error)

	// Upload data operations
	UpdateUploadData(ctx context.Context, id uint, data any) error
	GetUploadData(ctx context.Context, id uint) (any, error)
	DeleteUploadData(ctx context.Context, id uint) error
	QueryUploadData(ctx context.Context, query any, filter RequestFilter) (any, error)
	CompleteUploadData(ctx context.Context, id uint) error

	// Utility operations
	RequestExists(ctx context.Context, id uint) (bool, error)
}

type RequestFilter struct {
	Protocol  string
	Operation models.RequestOperationType
	Limit     int
	Offset    int
}
