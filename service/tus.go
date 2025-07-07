package service

import (
	"context"
	"errors"
	"fmt"
	"github.com/labstack/echo/v4"
	"github.com/samber/lo"
	tusHandler "github.com/tus/tusd/v2/pkg/handler"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/service/internal/tus"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"time"
)

type TusHandlerConfig = tus.HandlerConfig
type TusHandler = tus.TusHandler
type TUSUploadCallbackHandler = tus.UploadCallbackHandler
type TUSUploadCreatedVerifyFunc = tus.UploadCreatedVerifyFunc
type UploadCreatedAfterFunc = tus.UploadCreatedAfterFunc

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

			storage.requests.RegisterRequestModel(models.RequestOperationTusUpload, &models.TUSRequest{})

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

func (t *TUSServiceDefault) UploadExists(ctx context.Context, id string) (bool, *models.TUSRequest) {
	req, err := t.requests.QueryRequest(ctx, &models.TUSRequest{TUSUploadID: id}, core.RequestFilter{
		Operation: models.RequestOperationTusUpload,
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

func (t *TUSServiceDefault) UploadHashExists(ctx context.Context, hash core.StorageHash) (bool, *models.TUSRequest) {
	req, err := t.requests.QueryRequest(ctx, &models.TUSRequest{UploadHash: hash.Multihash()}, core.RequestFilter{
		Operation: models.RequestOperationTusUpload,
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

func (t *TUSServiceDefault) Uploads(ctx context.Context, uploaderID uint) ([]*models.TUSRequest, error) {
	var uploads []*models.TUSRequest

	data, err := t.requests.ListRequestsByUser(ctx, uploaderID, core.RequestFilter{
		Operation: models.RequestOperationTusUpload,
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

func (t *TUSServiceDefault) CreateUpload(ctx context.Context, hash core.StorageHash, uploadID string, uploaderID uint, uploaderIP string, protocol core.StorageProtocol, mimeType string) (*models.TUSRequest, error) {
	var hashBytes []byte

	if hash != nil {
		hashBytes = hash.Multihash()
	}

	upload := &models.Request{
		Hash:      hashBytes,
		Protocol:  protocol.Name(),
		Operation: models.RequestOperationTusUpload,
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

func (t *TUSServiceDefault) UploadProgress(ctx context.Context, uploadID string) error {
	exists, upload := t.UploadExists(ctx, uploadID)

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

func (t *TUSServiceDefault) UploadProcessing(ctx context.Context, uploadID string) error {
	exists, upload := t.UploadExists(ctx, uploadID)

	if !exists {
		return core.ErrUploadNotFound
	}

	return t.requests.UpdateRequestStatus(ctx, upload.RequestID, models.RequestStatusProcessing, "Uploading...")
}

func (t *TUSServiceDefault) UploadCompleted(ctx context.Context, uploadID string) error {
	exists, upload := t.UploadExists(ctx, uploadID)

	if !exists {
		return core.ErrUploadNotFound
	}

	if upload.Request.Status == models.RequestStatusDuplicate {
		return nil
	}

	return t.requests.CompleteRequest(ctx, upload.RequestID)
}

func (t *TUSServiceDefault) DeleteUpload(ctx context.Context, uploadID string) error {
	exists, upload := t.UploadExists(ctx, uploadID)

	if !exists {
		return core.ErrUploadNotFound
	}

	err := t.requests.DeleteRequest(ctx, upload.RequestID)
	if err != nil {
		return err
	}

	return nil
}

func (t *TUSServiceDefault) SetHash(ctx context.Context, uploadID string, hash core.StorageHash) error {
	exists, upload := t.UploadExists(ctx, uploadID)

	if !exists {
		return core.ErrUploadNotFound
	}

	upload.Request.Hash = hash.Multihash()
	upload.Request.CIDType = hash.CIDType()

	err := t.requests.UpdateRequest(ctx, &upload.Request)
	if err != nil {
		return err
	}

	return nil
}

func CreateTusHandler(ctx core.Context, config TusHandlerConfig) (*tus.TusHandler, error) {
	handler, err := tus.NewTusHandler(ctx, config)
	if err != nil {
		ctx.Logger().Error("Failed to create tus handler", zap.Error(err))
		return nil, err
	}

	return handler, nil
}
func TUSDefaultUploadCreatedHandler(e echo.Context, ctx core.Context, verifyFunc TUSUploadCreatedVerifyFunc, afterFunc UploadCreatedAfterFunc) TUSUploadCallbackHandler {
	return tus.DefaultUploadCreatedHandler(e, ctx, verifyFunc, afterFunc)
}

func TUSDefaultUploadProgressHandler(e echo.Context, ctx core.Context) TUSUploadCallbackHandler {
	return tus.DefaultUploadProgressHandler(e, ctx)
}

// TUSOperationHandler implements core.OperationHandler for TUS uploads
type TUSOperationHandler struct {
	core.OperationHelper
	tusService core.TUSService
	logger     *core.Logger
}

// NewTUSOperationHandler creates a new TUS operation handler
func NewTUSOperationHandler(ctx core.Context) *TUSOperationHandler {
	svc := core.GetService[core.TUSService](ctx, core.TUS_SERVICE)
	return &TUSOperationHandler{
		tusService:      svc,
		OperationHelper: core.NewOperationHelper(ctx),
	}
}

// ValidateRequest validates a TUS upload request
func (h *TUSOperationHandler) ValidateRequest(_ context.Context, _ *models.Request) error {
	// For TUS uploads, basic validation is done when the upload is created
	return nil
}

// Execute processes a TUS upload request
func (h *TUSOperationHandler) Execute(ctx context.Context, req *models.Request) error {
	// Get the TUS upload data
	data, err := core.GetService[core.RequestService](h.Context(), core.REQUEST_SERVICE).GetRequestData(ctx, req)
	if err != nil {
		return err
	}

	tusReq, ok := data.(*models.TUSRequest)
	if !ok {
		return errors.New("invalid request data type")
	}

	// Mark the request as processing
	if err := h.tusService.UploadProcessing(ctx, tusReq.TUSUploadID); err != nil {
		return err
	}

	// TUS uploads are processed asynchronously by the TUS handler
	return nil
}

// GetStatus gets the status of a TUS upload request
func (h *TUSOperationHandler) GetStatus(ctx context.Context, req *models.Request) (core.RequestStatus, error) {
	// Get the TUS upload data
	data, err := core.GetService[core.RequestService](h.Context(), core.REQUEST_SERVICE).GetRequestData(ctx, req)
	if err != nil {
		return core.RequestStatus{}, err
	}

	tusReq, ok := data.(*models.TUSRequest)
	if !ok {
		return core.RequestStatus{}, errors.New("invalid request data type")
	}

	status := core.RequestStatus{
		State:   string(req.Status),
		Message: req.StatusMessage,
	}

	// If completed, set progress to 100%
	if req.Status == models.RequestStatusCompleted || tusReq.Completed {
		status.ProgressPercent = 100
	} else if req.Status == models.RequestStatusProcessing {
		// For processing, we don't have detailed progress info from TUS
		status.ProgressPercent = 50
	}

	return status, nil
}

// Cleanup handles any necessary cleanup after the operation completes or fails
func (h *TUSOperationHandler) Cleanup(_ context.Context, _ *models.Request) error {
	// For TUS uploads, most cleanup is handled by the TUS server
	// This is here to satisfy the OperationHandler interface
	return nil
}

func TUSDefaultUploadCompletedHandler(ctx core.Context, processHandler TUSUploadCallbackHandler, workflowName string) TUSUploadCallbackHandler {
	return func(handler *tus.TusHandler, info tusHandler.HookEvent) {
		// Call the original handler first
		processHandler(handler, info)

		// Get the TUS service
		tusService := core.GetService[core.TUSService](ctx, core.TUS_SERVICE)

		// Check if this upload is part of a workflow
		exists, tusReq := tusService.UploadExists(ctx, info.Upload.ID)
		if !exists {
			return // Not our upload, nothing to do
		}

		requestSvc := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		exists, err := requestSvc.RequestExists(ctx, tusReq.RequestID)
		if !exists || err != nil {
			ctx.Logger().Error("Failed to get request", zap.Error(err))
			return
		}

		// Get the workflow service
		workflowSvc := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
		if workflowSvc == nil {
			ctx.Logger().Error("Workflow service not available")
			// Mark request as failed since workflow service is unavailable
			if updateErr := requestSvc.FailRequest(ctx, tusReq.RequestID, "Workflow service unavailable"); updateErr != nil {
				ctx.Logger().Error("Failed to update request status after workflow service failure",
					zap.Error(updateErr),
					zap.Uint("requestID", tusReq.RequestID))
			}
			return
		}

		// Convert the request to a workflow and start it
		err = workflowSvc.ConvertRequestToWorkflow(ctx, tusReq.RequestID, workflowName, 0)
		if err != nil {
			ctx.Logger().Error("Failed to convert request to workflow", zap.Error(err))
			// Mark request as failed since workflow conversion failed
			if updateErr := requestSvc.FailRequest(ctx, tusReq.RequestID, fmt.Sprintf("Workflow conversion failed: %v", err)); updateErr != nil {
				ctx.Logger().Error("Failed to update request status after workflow conversion failure",
					zap.Error(updateErr),
					zap.Uint("requestID", tusReq.RequestID))
			}
			return
		}

		// Execute the first step of the workflow
		err = workflowSvc.ExecuteWorkflowStep(ctx, tusReq.RequestID)
		if err != nil {
			ctx.Logger().Error("Failed to execute workflow step", zap.Error(err))
			// Mark request as failed since workflow execution failed
			if updateErr := requestSvc.FailRequest(ctx, tusReq.RequestID, fmt.Sprintf("Workflow execution failed: %v", err)); updateErr != nil {
				ctx.Logger().Error("Failed to update request status after workflow execution failure",
					zap.Error(updateErr),
					zap.Uint("requestID", tusReq.RequestID))
			}
			return
		}
	}
}

func TUSDefaultUploadTerminatedHandler(e echo.Context, ctx core.Context) TUSUploadCallbackHandler {
	return tus.DefaultUploadTerminatedHandler(e, ctx)
}
