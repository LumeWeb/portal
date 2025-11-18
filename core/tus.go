package core

import (
	"context"
	"github.com/tus/tusd/v2/pkg/handler"
	router "go.lumeweb.com/portal-router"
	"go.lumeweb.com/portal/db/models"
	"io"
)

// TUS_SERVICE is the service identifier for the TUS upload service
const TUS_SERVICE = "tus"

// APITusHandler defines an interface for apis that support TUS uploads
type APITusHandler interface {
	// GetTusHandler returns the TUS handler for the protocol
	GetTusHandler() TusHandler
}

// TusHandler defines the interface for handling TUS protocol uploads
type TusHandler interface {
	// UploadReader gets a reader for an upload with optional byte range
	UploadReader(ctx context.Context, identifier any, protocol StorageProtocol, start int64) (io.ReadCloser, error)

	// UploadSize gets the size of an upload
	UploadSize(ctx context.Context, protocol StorageProtocol, identifier any) (uint64, error)

	// SetupRoute configures the TUS handler routes on the router
	SetupRoute(router router.Router, subdomain string, authRequired bool, twoFARequired bool, path string) error

	// StorageProtocol gets the current storage protocol
	StorageProtocol() (StorageProtocol, error)

	// HandleEventResponseError handles errors during upload events by stopping the upload
	// with the given HTTP status code and error message
	HandleEventResponseError(message string, httpCode int, hook handler.HookEvent)

	// FailUploadById marks an upload as failed by ID
	FailUploadById(ctx context.Context, protocol StorageProtocol, id string) error

	// SetHashById sets the content hash for an upload by ID
	SetHashById(ctx context.Context, id string, hash StorageHash) error

	// DeleteUpload removes upload files from storage
	DeleteUpload(ctx context.Context, id string) error

	// GetTusHandler returns the underlying TUS handler implementation
	GetTusHandler() *handler.Handler

	// Logger returns the logger instance for this handler
	Logger() *Logger

	// GetUploadMetadata gets metadata for an upload by ID
	GetUploadMetadata(ctx context.Context, protocol StorageProtocol, identifier any) (map[string]string, error)
}

// TUSUploadCallbackHandler defines the signature for upload callback handlers
type TUSUploadCallbackHandler func(TusHandler, handler.HookEvent)

// TUSPreUploadCreateCallback defines a callback that runs before upload creation
// Can modify the upload metadata and HTTP response
type TUSPreUploadCreateCallback func(hook handler.HookEvent) (handler.HTTPResponse, handler.FileInfoChanges, error)

// TUSUploadCreatedVerifyFunc defines a callback to verify newly created uploads
type TUSUploadCreatedVerifyFunc func(hook handler.HookEvent, uploaderId uint) (StorageHash, error)

// TUSUploadCreatedAfterFunc defines a callback that runs after upload creation
type TUSUploadCreatedAfterFunc func(requestId uint) error

// TUSUploadCompletedHashFunc defines a callback that computes and returns the multihash for a completed upload
type TUSUploadCompletedHashFunc func(handlr TusHandler, hook handler.HookEvent) (StorageHash, error)

// TUSPreFinishResponseCallback defines a callback that runs before finishing an upload
// Can modify the HTTP response before it's sent to the client
type TUSPreFinishResponseCallback func(hook handler.HookEvent) (handler.HTTPResponse, error)

// TUSHandlerConfig defines configuration for the TUS handler
type TUSHandlerConfig struct {
	// BasePath is the base URL path for TUS endpoints
	BasePath string

	// PreUpload is an optional callback that runs before upload creation.
	// It can modify upload metadata and HTTP response.
	PreUpload TUSPreUploadCreateCallback

	// CreatedUploadHandler handles events when new uploads are created
	CreatedUploadHandler TUSUploadCallbackHandler

	// UploadProgressHandler handles events when uploads make progress
	UploadProgressHandler TUSUploadCallbackHandler

	// TerminatedUploadHandler handles events when uploads are terminated
	TerminatedUploadHandler TUSUploadCallbackHandler

	// CompletedUploadHandler handles events when uploads complete successfully
	CompletedUploadHandler TUSUploadCallbackHandler

	// PreFinishResponse is an optional callback that runs before finishing an upload
	// Can modify the HTTP response before it's sent to the client
	PreFinishResponse TUSPreFinishResponseCallback

	// Protocol is the storage protocol this handler is associated with
	Protocol Protocol
}

// TUSService defines the service interface for TUS upload management
type TUSService interface {
	// UploadExists checks if an upload exists by ID
	UploadExists(ctx context.Context, protocol StorageProtocol, id string) (bool, *models.TUSRequest)

	// UploadHashExists checks if an upload exists by content hash
	UploadHashExists(ctx context.Context, protocol StorageProtocol, hash StorageHash) (bool, *models.TUSRequest)

	// Uploads lists all uploads for a user
	Uploads(ctx context.Context, protocol StorageProtocol, uploaderID uint) ([]*models.TUSRequest, error)

	// CreateUpload creates a new upload record
	CreateUpload(ctx context.Context, hash StorageHash, uploadID string, uploaderID uint, uploaderIP string, protocol StorageProtocol) (*models.TUSRequest, error)

	// UploadProgress updates upload progress status
	UploadProgress(ctx context.Context, protocol StorageProtocol, uploadID string) error

	// UploadProcessing marks an upload as being processed
	UploadProcessing(ctx context.Context, protocol StorageProtocol, uploadID string) error

	// UploadProcessing marks an upload as being completed
	UploadCompleted(ctx context.Context, protocol StorageProtocol, uploadID string) error

	// DeleteUpload removes an upload record
	DeleteUpload(ctx context.Context, protocol StorageProtocol, uploadID string) error

	// SetHash sets the content hash for an upload
	SetHash(ctx context.Context, protocol StorageProtocol, uploadID string, hash StorageHash) error

	Service
}
