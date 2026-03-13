package tus

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/labstack/echo/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/tus/tusd/v2/pkg/handler"
	"github.com/tus/tusd/v2/pkg/prometheuscollector"
	"github.com/tus/tusd/v2/pkg/s3store"
	mcontext "go.lumeweb.com/portal-middleware/context"
	"go.lumeweb.com/portal-middleware/tus"
	router "go.lumeweb.com/portal-router"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
	"go.uber.org/zap"
	"golang.org/x/exp/slog"
	"gorm.io/gorm"
)

type CtxRangeKeyType string

const CtxRangeKey CtxRangeKeyType = "range"

var _ core.TusHandler = (*TusHandlerDefault)(nil)

type TusHandlerDefault struct {
	handlerConfig  core.TUSHandlerConfig
	ctx            core.Context
	db             *gorm.DB
	config         config.Manager
	logger         *core.Logger
	tusService     core.TUSService
	cron           core.CronService
	storage        core.StorageService
	users          core.UserService
	metadata       core.UploadService
	requests       core.RequestService
	tus            *handler.Handler
	tusStore       handler.DataStore
	s3Client       *s3.Client
	workerCancel   context.CancelFunc
}

func (t *TusHandlerDefault) GetTusHandler() *handler.Handler {
	return t.tus
}

func NewTusHandler(
	ctx core.Context, handlerConfig core.TUSHandlerConfig) (*TusHandlerDefault, error) {

	th := &TusHandlerDefault{
		handlerConfig: handlerConfig,
		ctx:           ctx,
		db:            ctx.DB(),
		config:        ctx.Config(),
		logger:        ctx.Logger(),
		tusService:    core.GetService[core.TUSService](ctx, core.TUS_SERVICE),
		cron:          core.GetService[core.CronService](ctx, core.CRON_SERVICE),
		storage:       core.GetService[core.StorageService](ctx, core.STORAGE_SERVICE),
		users:         core.GetService[core.UserService](ctx, core.USER_SERVICE),
		metadata:      core.GetService[core.UploadService](ctx, core.UPLOAD_SERVICE),
		requests:      core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE),
	}

	err := th.init(ctx, handlerConfig)
	if err != nil {
		return nil, err
	}

	return th, nil
}

// getUploadByIdentifier retrieves upload by identifier (hash or string ID)
func (t *TusHandlerDefault) getUploadByIdentifier(ctx context.Context, identifier any, protocol core.StorageProtocol) (handler.Upload, error) {
	ctx, span := core.TraceMethod(ctx, "TusHandlerDefault.getUploadByIdentifier")
	defer span.End()

	switch v := identifier.(type) {
	case core.StorageHash:
		exists, _upload := t.tusService.UploadHashExists(ctx, protocol, v)

		if !exists || _upload == nil {
			return nil, gorm.ErrRecordNotFound
		}

		return t.tusStore.GetUpload(ctx, _upload.TUSUploadID)
	case string:
		exists, _upload := t.tusService.UploadExists(ctx, protocol, v)

		if !exists || _upload == nil {
			return nil, gorm.ErrRecordNotFound
		}

		return t.tusStore.GetUpload(ctx, _upload.TUSUploadID)

	default:
		return nil, fmt.Errorf("invalid identifier type")
	}
}

func (t *TusHandlerDefault) UploadReader(ctx context.Context, identifier any, protocol core.StorageProtocol, start int64) (io.ReadSeekCloser, error) {
	ctx, span := core.TraceMethod(ctx, "TusHandlerDefault.UploadReader")
	defer span.End()

	upload, err := t.getUploadByIdentifier(ctx, identifier, protocol)
	if err != nil {
		return nil, err
	}

	info, err := upload.GetInfo(ctx)
	if err != nil {
		return nil, err
	}

	// Create a TUS upload reader that implements ReadSeekCloser
	reader, err := NewTUSUploadReader(ctx, t.logger, upload, info, start)
	if err != nil {
		return nil, err
	}

	return reader, nil
}

func (t *TusHandlerDefault) UploadSize(ctx context.Context, protocol core.StorageProtocol, identifier any) (uint64, error) {
	ctx, span := core.TraceMethod(ctx, "TusHandlerDefault.UploadSize")
	defer span.End()

	upload, err := t.getUploadByIdentifier(ctx, identifier, protocol)
	if err != nil {
		return 0, err
	}

	info, err := upload.GetInfo(ctx)
	if err != nil {
		return 0, err
	}

	return uint64(info.Size), nil
}

func (t *TusHandlerDefault) SetupRoute(router router.Router, subdomain string, authRequired bool, twoFARequired bool, path string) error {
	return tus.RegisterTusRoutes(
		t.ctx,
		router,
		core.GetService[core.AccessService](t.ctx, core.ACCESS_SERVICE),
		subdomain,
		path,
		wrapContextHandler(t.tus),
		authRequired,
		twoFARequired,
	)
}
func (t *TusHandlerDefault) StorageProtocol() (core.StorageProtocol, error) {
	if sp, ok := t.handlerConfig.Protocol.(core.StorageProtocol); ok {
		return sp, nil
	}

	if t.handlerConfig.Protocol == nil {
		return nil, fmt.Errorf("storage protocol not initialized")
	}

	return nil, fmt.Errorf("protocol %T does not implement core.StorageProtocol", t.handlerConfig.Protocol)
}

func (t *TusHandlerDefault) HandleEventResponseError(message string, httpCode int, hook handler.HookEvent) {
	resp := handler.HTTPResponse{StatusCode: httpCode, Header: nil, Body: message}
	hook.Upload.StopUpload(resp)
}

func (t *TusHandlerDefault) FailUploadById(ctx context.Context, protocol core.StorageProtocol, id string) error {
	ctx, span := core.TraceMethod(ctx, "TusHandlerDefault.FailUploadById")
	defer span.End()

	exists, upload := t.tusService.UploadExists(ctx, protocol, id)

	if !exists {
		return core.ErrUploadNotFound
	}

	err := t.requests.UpdateRequestStatus(ctx, upload.RequestID, models.RequestStatusFailed, "Upload failed")
	if err != nil {
		return err
	}

	err = t.tusService.DeleteUpload(ctx, protocol, id)
	if err != nil {
		return err
	}

	err = t.DeleteUpload(ctx, id)

	if err != nil {
		return err
	}

	return nil
}

func (t *TusHandlerDefault) SetHashById(ctx context.Context, id string, hash core.StorageHash) error {
	ctx, span := core.TraceMethod(ctx, "TusHandlerDefault.SetHashById")
	defer span.End()

	sp, err := t.StorageProtocol()
	if err != nil {
		return err
	}

	err = t.tusService.SetHash(ctx, sp, id, hash)
	if err != nil {
		return err
	}

	return nil

}

func (t *TusHandlerDefault) Logger() *core.Logger {
	return t.logger
}

func (t *TusHandlerDefault) GetUploadMetadata(ctx context.Context, protocol core.StorageProtocol, identifier any) (map[string]string, error) {
	ctx, span := core.TraceMethod(ctx, "TusHandlerDefault.GetUploadMetadata")
	defer span.End()

	upload, err := t.getUploadByIdentifier(ctx, identifier, protocol)
	if err != nil {
		return nil, err
	}

	info, err := upload.GetInfo(ctx)
	if err != nil {
		return nil, err
	}

	// Return the upload metadata from TUS info
	return info.MetaData, nil
}

func (t *TusHandlerDefault) DeleteUpload(ctx context.Context, id string) error {
	ctx, span := core.TraceMethod(ctx, "TusHandlerDefault.DeleteUpload")
	defer span.End()

	objectId, _ := splitIds(id)

	// Try both IDs for each file type
	for _, deleteId := range []string{id, objectId} {
		if deleteId == "" {
			continue
		}

		// Check main file
		_, err := t.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(t.config.Config().Core.Storage.S3.BufferBucket),
			Key:    aws.String(t.storage.GetTemporaryUploadPath(t.handlerConfig.Protocol.(core.StorageProtocol), deleteId)),
		})
		if err == nil {
			_, err = t.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(t.config.Config().Core.Storage.S3.BufferBucket),
				Key:    aws.String(t.storage.GetTemporaryUploadPath(t.handlerConfig.Protocol.(core.StorageProtocol), deleteId)),
			})
			if err != nil {
				t.logger.Error("failed to delete upload object", zap.Error(err))
			}
		}

		// Check info file
		_, err = t.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(t.config.Config().Core.Storage.S3.BufferBucket),
			Key:    aws.String(t.storage.GetTemporaryUploadPath(t.handlerConfig.Protocol.(core.StorageProtocol), deleteId+".info")),
		})
		if err == nil {
			_, err = t.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(t.config.Config().Core.Storage.S3.BufferBucket),
				Key:    aws.String(t.storage.GetTemporaryUploadPath(t.handlerConfig.Protocol.(core.StorageProtocol), deleteId+".info")),
			})
			if err != nil {
				t.logger.Error("failed to delete upload metadata", zap.Error(err))
			}
		}
	}

	return nil
}

func (t *TusHandlerDefault) init(ctx context.Context, handlerConfig core.TUSHandlerConfig) error {
	ctx, span := core.TraceMethod(ctx, "TusHandlerDefault.init")
	defer span.End()

	// Validate handler config
	if t.handlerConfig.Protocol == nil {
		return fmt.Errorf("handler config Protocol cannot be nil")
	}

	if _, ok := t.handlerConfig.Protocol.(core.StorageProtocol); !ok {
		return fmt.Errorf("handler config Protocol must implement core.StorageProtocol")
	}

	s3Client, err := t.storage.S3Client(ctx)
	if err != nil {
		return err
	}

	store := s3store.New(t.config.Config().Core.Storage.S3.BufferBucket, s3Client)
	store.ObjectPrefix = t.storage.GetTemporaryUploadDir(t.handlerConfig.Protocol.(core.StorageProtocol))

	composer := handler.NewStoreComposer()
	store.UseIn(composer)

	locker, err := getLocker(t.config, t.db, t.logger)
	if err != nil {
		return err
	}

	if locker != nil {
		composer.UseLocker(locker)
	}

	handlr, err := handler.NewHandler(handler.Config{
		BasePath:                  handlerConfig.BasePath,
		StoreComposer:             composer,
		DisableDownload:           true,
		NotifyCompleteUploads:     true,
		NotifyTerminatedUploads:   true,
		NotifyCreatedUploads:      true,
		RespectForwardedHeaders:   true,
		PreUploadCreateCallback:   handlerConfig.PreUpload,
		PreFinishResponseCallback: handlerConfig.PreFinishResponse,
		Logger:                    loggerToSlog(t.logger),
	})

	if err != nil {
		return err
	}

	err = core.RegisterServiceMetrics(core.TUS_SERVICE, []prometheus.Collector{prometheuscollector.New(handlr.Metrics)})
	if err != nil {
		return err
	}

	t.tus = handlr
	t.tusStore = store
	t.s3Client = s3Client

	go t.worker()

	return nil
}
func (t *TusHandlerDefault) worker() {
	// Detach context for upload operations to survive service cancellation
	// This allows in-progress uploads to complete during shutdown
	detachedCtx := core.DetachContext(t.ctx)

	// Create cancellable sub-context for worker lifecycle management
	// This allows graceful shutdown while preserving upload operation context
	workerCtx, cancel := context.WithCancel(detachedCtx)
	t.workerCancel = cancel
	defer cancel()

	// Handle created uploads
	go func() {
		for {
			select {
			case <-workerCtx.Done():
				return
			case info := <-t.tus.CreatedUploads:
				if t.handlerConfig.CreatedUploadHandler != nil {
					t.handlerConfig.CreatedUploadHandler(t, info)
				}
			}
		}
	}()

	// Handle upload progress
	go func() {
		for {
			select {
			case <-workerCtx.Done():
				return
			case info := <-t.tus.UploadProgress:
				if t.handlerConfig.UploadProgressHandler != nil {
					t.handlerConfig.UploadProgressHandler(t, info)
				}
			}
		}
	}()

	// Handle terminated uploads
	go func() {
		for {
			select {
			case <-workerCtx.Done():
				return
			case info := <-t.tus.TerminatedUploads:
				if t.handlerConfig.TerminatedUploadHandler != nil {
					t.handlerConfig.TerminatedUploadHandler(t, info)
				}
			}
		}
	}()

	// Handle completed uploads
	go func() {
		for {
			select {
			case <-workerCtx.Done():
				return
			case info := <-t.tus.CompleteUploads:
				if t.handlerConfig.CompletedUploadHandler != nil {
					t.handlerConfig.CompletedUploadHandler(t, info)
				}
			}
		}
	}()
}

// Shutdown gracefully terminates the worker goroutines
// This should be called during service shutdown to prevent goroutine leaks
func (t *TusHandlerDefault) Shutdown() {
	if t.workerCancel != nil {
		t.workerCancel()
	}
}

func getLockerMode(cm config.Manager, logger *core.Logger) string {
	cfg := cm.Config().Core.Storage.Tus

	switch cfg.LockerMode {
	case "", "none":
		return "none"
	case "db":
		return "db"
	case "redis":

		return "db"
	default:
		logger.Fatal("invalid locker mode", zap.String("mode", cfg.LockerMode))
	}

	return "none"
}

func getLocker(cm config.Manager, db *gorm.DB, logger *core.Logger) (handler.Locker, error) {
	mode := getLockerMode(cm, logger)

	switch mode {
	case "none":
		return nil, nil
	case "db":
		return NewDbLocker(db, logger), nil
	}

	return nil, nil
}

func DefaultUploadCreatedHandler(ctx core.Context, verifyFunc core.TUSUploadCreatedVerifyFunc, afterFunc core.TUSUploadCreatedAfterFunc) core.TUSUploadCallbackHandler {
	return func(handlr core.TusHandler, hook handler.HookEvent) {
		var errMessage string

		echoCtx, ok := GetEchoContext(hook.Context)
		if !ok {
			errMessage = "Failed to get echo context"
			handlr.HandleEventResponseError(errMessage, http.StatusInternalServerError, hook)
			ctx.Logger().Error(errMessage)
			return
		}

		uploaderID, err := mcontext.GetUserID(echoCtx)

		if err != nil {
			errMessage = "Failed to get user from context"
			handlr.HandleEventResponseError(errMessage, http.StatusBadRequest, hook)
			ctx.Logger().Error(errMessage, zap.Error(err))
			return
		}

		// Verify the uploader
		if verifyFunc == nil {
			panic("verifyFunc is required")
		}

		hash, err := verifyFunc(hook, uploaderID)
		if err != nil {
			errMessage = "Failed to verify upload"
			handlr.HandleEventResponseError(errMessage, http.StatusBadRequest, hook)
			ctx.Logger().Error(errMessage, zap.Error(err))
			return
		}

		uploaderIP := hook.HTTPRequest.RemoteAddr

		sp, err := handlr.StorageProtocol()
		if err != nil {
			errMessage = "Failed to get storage protocol"
			handlr.HandleEventResponseError(errMessage, http.StatusBadRequest, hook)
			ctx.Logger().Error(errMessage, zap.Error(err))
			return
		}

		req, err := core.GetService[core.TUSService](ctx, core.TUS_SERVICE).CreateUpload(ctx, hash, hook.Upload.ID, uploaderID, uploaderIP, sp)
		if err != nil {
			errMessage = "Failed to update upload status"
			handlr.HandleEventResponseError(errMessage, http.StatusInternalServerError, hook)
			ctx.Logger().Error(errMessage, zap.Error(err))
		}

		if afterFunc != nil {
			err = afterFunc(req.RequestID)
			if err != nil {
				errMessage = "Failed to process upload"
				handlr.HandleEventResponseError(errMessage, http.StatusInternalServerError, hook)
				ctx.Logger().Error(errMessage, zap.Error(err))
			}
		}
	}
}

func DefaultUploadProgressHandler(ctx core.Context) core.TUSUploadCallbackHandler {
	return func(handlr core.TusHandler, hook handler.HookEvent) {
		sp, err := handlr.StorageProtocol()
		if err != nil {
			errMessage := "Failed to get storage protocol"
			handlr.HandleEventResponseError(errMessage, http.StatusBadRequest, hook)
			ctx.Logger().Error(errMessage, zap.Error(err))
			return
		}
		err = core.GetService[core.TUSService](ctx, core.TUS_SERVICE).UploadProgress(ctx, sp, hook.Upload.ID)
		if err != nil {
			errMessage := "Failed to update upload progress"
			handlr.HandleEventResponseError(errMessage, http.StatusInternalServerError, hook)
			ctx.Logger().Error(errMessage, zap.Error(err))
		}
	}
}

func DefaultUploadTerminatedHandler(ctx core.Context) core.TUSUploadCallbackHandler {
	return func(handlr core.TusHandler, hook handler.HookEvent) {
		sp, err := handlr.StorageProtocol()
		if err != nil {
			errMessage := "Failed to get storage protocol"
			handlr.HandleEventResponseError(errMessage, http.StatusBadRequest, hook)
			ctx.Logger().Error(errMessage, zap.Error(err))
			return
		}
		err = handlr.FailUploadById(ctx, sp, hook.Upload.ID)
		if err != nil {
			errMessage := "Failed to update upload status"
			handlr.HandleEventResponseError(errMessage, http.StatusInternalServerError, hook)
			ctx.Logger().Error(errMessage, zap.Error(err))
		}
	}
}

func DefaultUploadCompletedHandler(ctx core.Context, processHandler core.TUSUploadCallbackHandler, hashCallback core.TUSUploadCompletedHashFunc, workflowName string) core.TUSUploadCallbackHandler {
	return func(handlr core.TusHandler, hook handler.HookEvent) {
		sp, err := handlr.StorageProtocol()
		if err != nil {
			errMessage := "Failed to get storage protocol"
			handlr.HandleEventResponseError(errMessage, http.StatusBadRequest, hook)
			ctx.Logger().Error(errMessage, zap.Error(err))
			return
		}

		// Set status to processing first so the request appears in operations
		err = core.GetService[core.TUSService](ctx, core.TUS_SERVICE).UploadProcessing(ctx, sp, hook.Upload.ID)
		if err != nil {
			errMessage := "Failed to update upload status"
			handlr.HandleEventResponseError(errMessage, http.StatusInternalServerError, hook)
			ctx.Logger().Error(errMessage, zap.Error(err))
			return
		}

		// Get services for workflow processing
		requestSvc := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
		tusService := core.GetService[core.TUSService](ctx, core.TUS_SERVICE)
		workflowSvc := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)

		// Check if this upload exists
		exists, tusReq := tusService.UploadExists(ctx, sp, hook.Upload.ID)
		if !exists {
			return // Not our upload, nothing to do
		}

		// Verify request exists
		exists, err = requestSvc.RequestExists(ctx, tusReq.RequestID)
		if !exists || err != nil {
			ctx.Logger().Error("Failed to get request", zap.Error(err))
			return
		}

		// Convert to workflow before hashing so we can use workflow failure handling
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

		// Compute hash if callback is provided (now the request is visible in operations and in a workflow)
		var computedHash core.StorageHash
		if hashCallback != nil {
			// Get the upload reader and size for progress tracking
			protocol, err := handlr.StorageProtocol()
			if err != nil {
				errMessage := "Failed to get storage protocol"
				ctx.Logger().Error(errMessage, zap.Error(err))
				if failErr := handlr.FailUploadById(ctx, sp, hook.Upload.ID); failErr != nil {
					ctx.Logger().Error("Failed to fail upload",
						zap.Error(failErr),
						zap.String("uploadID", hook.Upload.ID))
				}
				return
			}

			// Resolve effective size and validate
			size := hook.Upload.Size
			if size < 0 {
				size = hook.Upload.Offset
			}
			if size < 0 {
				errMessage := fmt.Sprintf("invalid/unknown upload size for %s: size=%d offset=%d", hook.Upload.ID, hook.Upload.Size, hook.Upload.Offset)
				ctx.Logger().Error(errMessage)
				if failErr := handlr.FailUploadById(ctx, sp, hook.Upload.ID); failErr != nil {
					ctx.Logger().Error("Failed to fail upload",
						zap.Error(failErr),
						zap.String("uploadID", hook.Upload.ID))
				}
				return
			}

			// Get the upload reader
			// Note: ctx is already detached from the worker goroutine
			reader, err := handlr.UploadReader(ctx, hook.Upload.ID, protocol, 0)
			if err != nil {
				errMessage := fmt.Sprintf("failed to get upload reader for %s: %v", hook.Upload.ID, err)
				ctx.Logger().Error(errMessage)
				if failErr := handlr.FailUploadById(ctx, sp, hook.Upload.ID); failErr != nil {
					ctx.Logger().Error("Failed to fail upload",
						zap.Error(failErr),
						zap.String("uploadID", hook.Upload.ID))
				}
				return
			}

			// Wrap reader with progress tracking
			progressReader := NewHashProgressReader(reader, size, tusReq.RequestID, workflowSvc, ctx.Logger(), 0)
			defer func() {
				if err := reader.Close(); err != nil {
					ctx.Logger().Error("failed to close upload reader", zap.Error(err))
				}
				progressReader.Finalize(ctx)
			}()

			// Call hashCallback with the progress reader
			computedHash, err = hashCallback(handlr, hook, progressReader)
			if err != nil {
				errMessage := "Failed to compute hash"
				ctx.Logger().Error(errMessage, zap.Error(err))

				// Fail the upload since hashing failed - this cleans up the uploaded files from S3 buffer bucket
				if failErr := handlr.FailUploadById(ctx, sp, hook.Upload.ID); failErr != nil {
					ctx.Logger().Error("Failed to fail upload",
						zap.Error(failErr),
						zap.String("uploadID", hook.Upload.ID))
				}
				return
			}

			// Update the request with the computed hash using TUS service
			if computedHash != nil {
				err = handlr.SetHashById(ctx, hook.Upload.ID, computedHash)
				if err != nil {
					errMessage := "Failed to update request with computed hash"
					ctx.Logger().Error(errMessage, zap.Error(err))

					// Fail the upload since hash update failed - this cleans up the uploaded files from S3 buffer bucket
					if failErr := handlr.FailUploadById(ctx, sp, hook.Upload.ID); failErr != nil {
						ctx.Logger().Error("Failed to fail upload",
							zap.Error(failErr),
							zap.String("uploadID", hook.Upload.ID))
					}
					return
				}
			}
		}

		if processHandler != nil {
			processHandler(handlr, hook)
		}

		// Dispatch the first workflow step using the public interface
		err = workflowSvc.DispatchWorkflowStep(ctx, tusReq.RequestID)
		if err != nil {
			ctx.Logger().Error("Failed to dispatch workflow step", zap.Error(err))
			if updateErr := requestSvc.FailRequest(ctx, tusReq.RequestID,
				fmt.Sprintf("Workflow dispatch failed: %v", err)); updateErr != nil {
				ctx.Logger().Error("Failed to update request status",
					zap.Error(updateErr),
					zap.Uint("requestID", tusReq.RequestID))
			}
		}
	}
}
func loggerToSlog(logger *core.Logger) *slog.Logger {
	// Create a bridge
	bridge := NewSlogBridge(logger)

	// Create a handler that wraps the new slog handler but presents the old interface
	oldHandler := bridge.Handler()

	// Create an old-style logger with this handler
	return slog.New(oldHandler)
}

func splitIds(id string) (objectId, multipartId string) {
	// We use LastIndex to allow plus signs in the object ID and assume that S3 will never
	// returns multipart ID that incldues a plus sign.
	index := strings.LastIndex(id, "+")
	if index == -1 {
		return
	}

	objectId = id[:index]
	multipartId = id[index+1:]
	return
}

type echoContextKeyType string

const echoContextKey echoContextKeyType = "echoContext"

func wrapContextHandler(h http.Handler) echo.HandlerFunc {
	return func(c echo.Context) error {
		ctx := context.WithValue(c.Request().Context(), echoContextKey, c)
		req := c.Request().WithContext(ctx)
		h.ServeHTTP(c.Response(), req)
		return nil
	}
}

func GetEchoContext(ctx context.Context) (echo.Context, bool) {
	ctx, span := core.TraceMethod(ctx, "GetEchoContext")
	defer span.End()

	c, ok := ctx.Value(echoContextKey).(echo.Context)
	return c, ok
}
