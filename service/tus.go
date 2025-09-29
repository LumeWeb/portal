package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/samber/lo"
	tusHandler "github.com/tus/tusd/v2/pkg/handler"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/service/internal/tus"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var _ core.TUSService = (*TUSServiceDefault)(nil)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID:      core.TUS_SERVICE,
		Factory: NewTUSService,
		Depends: []string{core.REQUEST_SERVICE},
	})
}

type TUSServiceDefault struct {
	ctx      core.Context
	db       *gorm.DB
	logger   *core.Logger
	requests core.RequestService
}

func NewTUSService() (core.Service, []core.ContextBuilderOption, error) {
	storage := &TUSServiceDefault{}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			storage.ctx = ctx
			storage.db = ctx.DB()
			storage.logger = ctx.ServiceLogger(storage)
			storage.requests = core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)

			for _, proto := range core.GetProtocolList() {
				if sproto, ok := proto.(core.StorageProtocol); ok {
					storage.requests.RegisterRequestModel(core.TUSUploadOperationName(sproto.Name()), &models.TUSRequest{})
				}
			}

			return nil
		}),
	)

	return storage, opts, nil
}

func (t *TUSServiceDefault) ID() string {
	return core.TUS_SERVICE
}

// Name returns the protocol name
func (t *TUSServiceDefault) Name() string {
	return "tus"
}

func (t *TUSServiceDefault) UploadExists(ctx context.Context, protocol core.StorageProtocol, id string) (bool, *models.TUSRequest) {
	opName := core.TUSUploadOperationName(protocol.Name())

	req, err := t.requests.QueryRequestData(ctx, &models.TUSRequest{TUSUploadID: id}, core.RequestFilter{
		Operation: &opName,
	})
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			t.logger.Error("Failed to query request", zap.Error(err))
		}
		return false, nil
	}

	data, err := t.requests.GetRequestData(ctx, req)
	if err != nil {
		t.logger.Error("Failed to get request data", zap.Error(err))
		return false, nil
	}

	return true, data.(*models.TUSRequest)
}

func (t *TUSServiceDefault) UploadHashExists(ctx context.Context, protocol core.StorageProtocol, hash core.StorageHash) (bool, *models.TUSRequest) {
	opName := core.TUSUploadOperationName(protocol.Name())

	req, err := t.requests.QueryRequest(ctx, &models.TUSRequest{UploadHash: hash.Multihash()}, core.RequestFilter{
		Operation: &opName,
	})

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, nil
	}

	data, err := t.requests.GetRequestData(ctx, req)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, nil
	}

	return true, data.(*models.TUSRequest)
}

func (t *TUSServiceDefault) Uploads(ctx context.Context, protocol core.StorageProtocol, uploaderID uint) ([]*models.TUSRequest, error) {
	var uploads []*models.TUSRequest

	opName := core.TUSUploadOperationName(protocol.Name())

	data, err := t.requests.ListRequestsByUser(ctx, uploaderID, core.RequestFilter{
		Operation: &opName,
	})

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		return nil, err
	}

	for _, req := range data {
		uploadData, err := t.requests.GetRequestData(ctx, req)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, err
			}
			return nil, err
		}
		uploads = append(uploads, uploadData.(*models.TUSRequest))
	}

	return uploads, nil
}

func (t *TUSServiceDefault) CreateUpload(ctx context.Context, hash core.StorageHash, uploadID string, uploaderID uint, uploaderIP string, protocol core.StorageProtocol) (*models.TUSRequest, error) {
	var hashBytes []byte

	if hash != nil {
		hashBytes = hash.Multihash()
	}

	opName := core.TUSUploadOperationName(protocol.Name())

	upload := &models.Request{
		Hash:      hashBytes,
		Protocol:  protocol.Name(),
		Operation: opName,
		Status:    models.RequestStatusPending,
		UserID:    lo.ToPtr(uploaderID),
		SourceIP:  uploaderIP,
	}

	if hash != nil {
		upload.CIDType = hash.CIDType()
	}

	tusData := &models.TUSRequest{TUSUploadID: uploadID}
	request, err := t.requests.CreateRequest(ctx, upload, tusData)
	if err != nil {
		return nil, err
	}

	dataReq, err := t.requests.GetRequestData(ctx, request)

	if err != nil {
		return nil, err
	}

	return dataReq.(*models.TUSRequest), nil
}

func (t *TUSServiceDefault) UploadProgress(ctx context.Context, protocol core.StorageProtocol, uploadID string) error {
	exists, upload := t.UploadExists(ctx, protocol, uploadID)

	if !exists {
		return core.ErrUploadNotFound
	}

	upload.UpdatedAt = time.Now()

	req, err := t.requests.GetRequest(ctx, upload.RequestID)
	if err != nil {
		return err
	}
	return t.requests.UpdateRequestData(ctx, req, upload)
}

func (t *TUSServiceDefault) UploadCompleted(ctx context.Context, protocol core.StorageProtocol, uploadID string) error {
	exists, upload := t.UploadExists(ctx, protocol, uploadID)

	if !exists {
		return core.ErrUploadNotFound
	}

	upload.UpdatedAt = time.Now()
	upload.Completed = true

	req, err := t.requests.GetRequest(ctx, upload.RequestID)
	if err != nil {
		return err
	}
	return t.requests.UpdateRequestData(ctx, req, upload)
}

func (t *TUSServiceDefault) UploadProcessing(ctx context.Context, protocol core.StorageProtocol, uploadID string) error {
	exists, upload := t.UploadExists(ctx, protocol, uploadID)

	if !exists {
		return core.ErrUploadNotFound
	}

	return t.requests.UpdateRequestStatus(ctx, upload.RequestID, models.RequestStatusProcessing, "Uploading...")
}

func (t *TUSServiceDefault) DeleteUpload(ctx context.Context, protocol core.StorageProtocol, uploadID string) error {
	exists, upload := t.UploadExists(ctx, protocol, uploadID)

	if !exists {
		return core.ErrUploadNotFound
	}

	err := t.requests.DeleteRequest(ctx, upload.RequestID)
	if err != nil {
		return err
	}

	return nil
}

func (t *TUSServiceDefault) SetHash(ctx context.Context, protocol core.StorageProtocol, uploadID string, hash core.StorageHash) error {
	exists, upload := t.UploadExists(ctx, protocol, uploadID)

	if !exists {
		return core.ErrUploadNotFound
	}

	req, err := t.requests.GetRequest(ctx, upload.RequestID)
	if err != nil {
		return err
	}

	req.Hash = hash.Multihash()
	req.CIDType = hash.CIDType()

	err = t.requests.UpdateRequest(ctx, req)
	if err != nil {
		return err
	}

	return nil
}

func CreateTusHandler(ctx core.Context, config core.TUSHandlerConfig) (*tus.TusHandlerDefault, error) {
	handler, err := tus.NewTusHandler(ctx, config)
	if err != nil {
		ctx.Logger().Error("Failed to create tus handler", zap.Error(err))
		return nil, err
	}

	return handler, nil
}
func TUSDefaultUploadCreatedHandler(ctx core.Context, verifyFunc core.TUSUploadCreatedVerifyFunc, afterFunc core.TUSUploadCreatedAfterFunc) core.TUSUploadCallbackHandler {
	return tus.DefaultUploadCreatedHandler(ctx, verifyFunc, afterFunc)
}

func TUSDefaultUploadProgressHandler(ctx core.Context) core.TUSUploadCallbackHandler {
	return tus.DefaultUploadProgressHandler(ctx)
}

// TUSOperationHandler implements core.OperationHandler for TUS uploads
type TUSOperationHandler struct {
	core.OperationHelper
	tusService core.TUSService
	handler    TUSOperationHandlerCallback
}

type TUSOperationHandlerCallback = func(ctx context.Context, h core.OperationHelper, req *models.Request, tusReq *models.TUSRequest) error

// NewTUSOperationHandler creates a new TUS operation handler
func NewTUSOperationHandler(ctx core.Context, protocol core.Protocol, handler TUSOperationHandlerCallback) core.Operation {
	svc := core.GetService[core.TUSService](ctx, core.TUS_SERVICE)

	return core.NewTUSUploadOperation(protocol.Name(),
		&TUSOperationHandler{
			tusService:      svc,
			OperationHelper: core.NewProtocolOperationHelper(ctx, protocol.Name()),
			handler:         handler,
		},
	)
}

// ValidateRequest validates a TUS upload request
func (h *TUSOperationHandler) ValidateRequest(_ context.Context, _ *models.Request) error {
	// For TUS uploads, basic validation is done when the upload is created
	return nil
}

// Execute processes a TUS upload request
func (h *TUSOperationHandler) Execute(ctx context.Context, req *models.Request) error {
	if h.handler != nil {
		// Get the TUS upload data
		data, err := core.GetService[core.RequestService](h.Context(), core.REQUEST_SERVICE).GetRequestData(ctx, req)
		if err != nil {
			return err
		}

		tusReq, ok := data.(*models.TUSRequest)
		if !ok {
			return errors.New("invalid request data type")
		}

		if err = h.handler(ctx, h.OperationHelper, req, tusReq); err != nil {
			return err
		}
	}

	return nil
}

// GetStatus gets the status of a TUS upload request
func (h *TUSOperationHandler) GetStatus(ctx context.Context, req *models.Request) (*core.RequestStatus, error) {
	// Get the TUS upload data
	data, err := core.GetService[core.RequestService](h.Context(), core.REQUEST_SERVICE).GetRequestData(ctx, req)
	if err != nil {
		return nil, err
	}

	tusReq, ok := data.(*models.TUSRequest)
	if !ok {
		return nil, errors.New("invalid request data type")
	}

	status := core.RequestStatus{}

	// If completed, set progress to 100%
	if req.Status == models.RequestStatusCompleted || tusReq.Completed {
		status.ProgressPercent = 100
	} else if req.Status == models.RequestStatusProcessing {
		// For processing, we don't have detailed progress info from TUS
		status.ProgressPercent = 50
	}

	return &status, nil
}

// Cleanup handles any necessary cleanup after the operation completes or fails
func (h *TUSOperationHandler) Cleanup(ctx context.Context, req *models.Request) error {
	// Get the TUS upload data
	data, err := core.GetService[core.RequestService](h.Context(), core.REQUEST_SERVICE).GetRequestData(ctx, req)
	if err != nil {
		return err
	}

	_, ok := data.(*models.TUSRequest)
	if !ok {
		return errors.New("invalid request data type")
	}

	tusReq, ok := data.(*models.TUSRequest)
	if !ok {
		return errors.New("invalid request data type")
	}

	apiName := h.Protocol().Name()

	api := core.GetAPI(apiName)

	if _, ok := api.(core.APITusHandler); !ok {
		return fmt.Errorf("API %T does not implement core.APITusHandler", api)
	}

	tusProto, _ := api.(core.APITusHandler)

	tusSvc := core.GetService[core.TUSService](h.Context(), core.TUS_SERVICE)

	proto := h.Protocol()
	sproto, ok := proto.(core.StorageProtocol)
	if !ok {
		return fmt.Errorf("Protocol %T does not implement core.StorageProtocol", proto)
	}

	err = tusSvc.UploadCompleted(ctx, sproto, tusReq.TUSUploadID)
	if err != nil {
		return err
	}

	err = tusProto.GetTusHandler().DeleteUpload(ctx, tusReq.TUSUploadID)
	if err != nil {
		return err
	}

	return nil
}

func TUSDefaultUploadCompletedHandler(ctx core.Context, processHandler core.TUSUploadCallbackHandler, workflowName string, hashCallback core.TUSUploadCompletedHashFunc) core.TUSUploadCallbackHandler {
	return tus.DefaultUploadCompletedHandler(ctx, processHandler, hashCallback, workflowName)
}

func TUSDefaultUploadTerminatedHandler(ctx core.Context) core.TUSUploadCallbackHandler {
	return tus.DefaultUploadTerminatedHandler(ctx)
}

// TUSHashGeneratorFunc defines a function type that generates a StorageHash from TUS upload data
type TUSHashGeneratorFunc func(hook tusHandler.HookEvent, data io.Reader, size uint64) (core.StorageHash, error)

// TusHandlerFactory defines a function type that creates TusHandler instances
type TusHandlerFactory func() core.TusHandler

func tusErrorResponse(hook tusHandler.HookEvent) tusHandler.HTTPResponse {
	tusVersion := hook.HTTPRequest.Header.Get("Tus-Resumable")
	if tusVersion == "" {
		tusVersion = "1.0.0" // fallback to default version
	}
	return tusHandler.HTTPResponse{
		StatusCode: http.StatusInternalServerError,
		Header: map[string]string{
			"Tus-Resumable": tusVersion,
			"Content-Type":  "application/json",
		},
		Body: `{"error":"internal server error"}`,
	}
}

// TUSDefaultPreFinishResponse creates a default PreFinishResponse callback that returns a CID JSON object.
// handlerFactory: Invoked per pre-finish event (may be called concurrently). Should be cheap or cache internally;
//
//	if using external resources, prefer lazy retrieval tied to hook.Context.
//
// hashFunc: Optional custom hash generator; if nil, the storage protocol's Hash method is used.
func TUSDefaultPreFinishResponse(handlerFactory TusHandlerFactory, hashFunc TUSHashGeneratorFunc) core.TUSPreFinishResponseCallback {
	return func(hook tusHandler.HookEvent) (tusHandler.HTTPResponse, error) {
		if handlerFactory == nil {
			return tusErrorResponse(hook), nil
		}

		handlr := handlerFactory()
		if handlr == nil {
			return tusErrorResponse(hook), nil
		}

		protocol, err := handlr.StorageProtocol()
		if err != nil {
			return tusHandler.HTTPResponse{}, fmt.Errorf("failed to get storage protocol: %w", err)
		}

		// Resolve effective size and validate (pre-finish should have final offset)
		size := hook.Upload.Size
		if size < 0 {
			size = hook.Upload.Offset
		}
		if size < 0 {
			return tusHandler.HTTPResponse{}, fmt.Errorf("invalid/unknown upload size for %s: size=%d offset=%d", hook.Upload.ID, hook.Upload.Size, hook.Upload.Offset)
		}

		// Get the upload reader
		reader, err := handlr.UploadReader(hook.Context, hook.Upload.ID, protocol, 0)
		if err != nil {
			return tusHandler.HTTPResponse{}, fmt.Errorf("failed to get upload reader for %s: %w", hook.Upload.ID, err)
		}
		defer func() {
			if err := reader.Close(); err != nil {
				handlr.Logger().Error("failed to close upload reader",
					zap.String("uploadID", hook.Upload.ID),
					zap.String("protocol", protocol.Name()),
					zap.Int64("size", hook.Upload.Size),
					zap.Int64("offset", hook.Upload.Offset),
					zap.Int64("effective_size", size),
					zap.Error(err))
			}
		}()
		if size < 0 {
			size = hook.Upload.Offset
		}
		if size < 0 {
			return tusHandler.HTTPResponse{}, fmt.Errorf("invalid/unknown upload size for %s: size=%d offset=%d", hook.Upload.ID, hook.Upload.Size, hook.Upload.Offset)
		}
		// Ensure we never read beyond the validated size and verify full read
		lr := &io.LimitedReader{R: reader, N: size}
		hashReader := lr

		var storageHash core.StorageHash
		if hashFunc != nil {
			// Use custom hash function if provided
			storageHash, err = hashFunc(hook, hashReader, uint64(size))
		} else {
			// Fall back to storage protocol's hash method
			storageHash, err = protocol.Hash(hashReader, uint64(size))
		}
		if err != nil {
			return tusHandler.HTTPResponse{}, fmt.Errorf("hash generation failed for upload %s: %w", hook.Upload.ID, err)
		}
		if storageHash == nil {
			return tusHandler.HTTPResponse{}, fmt.Errorf("hash function returned nil for upload %s", hook.Upload.ID)
		}
		if storageHash.Multihash() == nil || len(storageHash.Multihash()) == 0 {
			return tusHandler.HTTPResponse{}, fmt.Errorf("empty multihash returned for upload %s", hook.Upload.ID)
		}

		// Return JSON response with CID
		jsonBody, err := json.Marshal(struct {
			CID string `json:"cid"`
		}{
			CID: cid.NewCidV1(storageHash.CIDType(), storageHash.Multihash()).String(),
		})
		if err != nil {
			return tusHandler.HTTPResponse{}, err
		}

		return tusHandler.HTTPResponse{
			Header: map[string]string{
				"Content-Type": "application/json",
			},
			Body: string(jsonBody),
		}, nil
	}
}
