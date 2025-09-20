package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/ipfs/go-cid"
	mh "github.com/multiformats/go-multihash"
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

	upload.Request.Hash = hash.Multihash()
	upload.Request.CIDType = hash.CIDType()

	err := t.requests.UpdateRequest(ctx, &upload.Request)
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

func TUSDefaultUploadCompletedHandler(ctx core.Context, processHandler core.TUSUploadCallbackHandler, workflowName string) core.TUSUploadCallbackHandler {
	return func(handlr core.TusHandler, info tusHandler.HookEvent) {
		// Call the original handler first
		processHandler(handlr, info)

		// Get services
		requestSvc := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		tusService := core.GetService[core.TUSService](ctx, core.TUS_SERVICE)
		workflowSvc := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)

		storageProtoMissing := false

		// Get storage protocol first
		sproto, err := handlr.StorageProtocol()
		if err != nil {
			storageProtoMissing = true
		}

		// Check if this upload exists
		exists, tusReq := tusService.UploadExists(ctx, sproto, info.Upload.ID)
		if !exists {
			return // Not our upload, nothing to do
		}

		// Verify request exists
		exists, err = requestSvc.RequestExists(ctx, tusReq.RequestID)
		if !exists || err != nil {
			ctx.Logger().Error("Failed to get request", zap.Error(err))
			return
		}

		if workflowSvc == nil {
			ctx.Logger().Error("Workflow service not available")
			if updateErr := requestSvc.FailRequest(ctx, tusReq.RequestID, "Workflow service unavailable"); updateErr != nil {
				ctx.Logger().Error("Failed to update request status",
					zap.Error(updateErr),
					zap.Uint("requestID", tusReq.RequestID))
			}
			return
		}

		if storageProtoMissing {
			if updateErr := requestSvc.FailRequest(ctx, tusReq.RequestID, "Storage protocol missing"); updateErr != nil {
				ctx.Logger().Error("Failed to update request status",
					zap.Error(updateErr),
					zap.Uint("requestID", tusReq.RequestID))
			} else {
				ctx.Logger().Error("Failed to get storage protocol", zap.Error(err), zap.Uint("requestID", tusReq.RequestID))
			}

		}

		// Convert and execute workflow
		err = workflowSvc.ConvertRequestToWorkflow(ctx, tusReq.RequestID, workflowName, 0)
		if err != nil {
			ctx.Logger().Error("Failed to convert request to workflow", zap.Error(err))
			if updateErr := requestSvc.FailRequest(ctx, tusReq.RequestID,
				fmt.Sprintf("Workflow conversion failed: %v", err)); updateErr != nil {
				ctx.Logger().Error("Failed to update request status",
					zap.Error(updateErr),
					zap.Uint("requestID", tusReq.RequestID))
			}
			return
		}

		err = workflowSvc.ExecuteWorkflowStep(ctx, tusReq.RequestID)
		if err != nil {
			ctx.Logger().Error("Failed to execute workflow step", zap.Error(err))
			if updateErr := requestSvc.FailRequest(ctx, tusReq.RequestID,
				fmt.Sprintf("Workflow execution failed: %v", err)); updateErr != nil {
				ctx.Logger().Error("Failed to update request status",
					zap.Error(updateErr),
					zap.Uint("requestID", tusReq.RequestID))
			}
		}
	}
}

func TUSDefaultUploadTerminatedHandler(ctx core.Context) core.TUSUploadCallbackHandler {
	return tus.DefaultUploadTerminatedHandler(ctx)
}

// TUSMultihashGeneratorFunc defines a function type that generates a multihash from TUS upload data
type TUSMultihashGeneratorFunc func(hook tusHandler.HookEvent) (mh.Multihash, error)

// TUSDefaultPreFinishResponse creates a default PreFinishResponse callback that returns a CID JSON object
// hashFunc: A function that generates the multihash from the upload data
func TUSDefaultPreFinishResponse(hashFunc TUSMultihashGeneratorFunc) core.TUSPreFinishResponseCallback {
	return func(hook tusHandler.HookEvent) (tusHandler.HTTPResponse, error) {
		// Generate the multihash
		multihash, err := hashFunc(hook)
		if err != nil {
			return tusHandler.HTTPResponse{}, err
		}

		// Create CID from multihash
		fileCID := cid.NewCidV1(cid.Raw, multihash)

		// Return JSON response with CID
		jsonBody, err := json.Marshal(struct {
			CID string `json:"cid"`
		}{
			CID: fileCID.String(),
		})
		if err != nil {
			return tusHandler.HTTPResponse{}, err
		}

		return tusHandler.HTTPResponse{
			StatusCode: http.StatusOK,
			Header: map[string]string{
				"Content-Type": "application/json",
			},
			Body: string(jsonBody),
		}, nil
	}
}
