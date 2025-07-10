package tus

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/labstack/echo/v4"
	"github.com/tus/tusd/v2/pkg/handler"
	"github.com/tus/tusd/v2/pkg/redislocker"
	"github.com/tus/tusd/v2/pkg/s3store"
	mcontext "go.lumeweb.com/portal-middleware/context"
	"go.lumeweb.com/portal-middleware/tus"
	router "go.lumeweb.com/portal-router"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
	"go.uber.org/zap"
	"go.uber.org/zap/exp/zapslog"
	"gorm.io/gorm"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

type CtxRangeKeyType string

const CtxRangeKey CtxRangeKeyType = "range"

var _ core.TusHandler = (*TusHandlerDefault)(nil)

type TusHandlerDefault struct {
	handlerConfig core.TUSHandlerConfig
	ctx           core.Context
	db            *gorm.DB
	config        config.Manager
	logger        *core.Logger
	tusService    core.TUSService
	cron          core.CronService
	storage       core.StorageService
	users         core.UserService
	metadata      core.UploadService
	requests      core.RequestService
	tus           *handler.Handler
	tusStore      handler.DataStore
	s3Client      *s3.Client
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

	err := th.init(handlerConfig)
	if err != nil {
		return nil, err
	}

	return th, nil
}

func (t *TusHandlerDefault) UploadReader(ctx context.Context, identifier any, protocol core.StorageProtocol, start int64) (io.ReadCloser, error) {
	var upload handler.Upload

	switch v := identifier.(type) {
	case core.StorageHash:
		exists, _upload := t.tusService.UploadHashExists(ctx, protocol, v)

		if !exists {
			return nil, gorm.ErrRecordNotFound
		}

		meta, err := t.tusStore.GetUpload(ctx, _upload.TUSUploadID)
		if err != nil {
			return nil, err
		}

		upload = meta
	case string:
		exists, _upload := t.tusService.UploadExists(ctx, protocol, v)

		if !exists {
			return nil, gorm.ErrRecordNotFound
		}

		meta, err := t.tusStore.GetUpload(ctx, _upload.TUSUploadID)

		if err != nil {
			return nil, err
		}

		upload = meta

	default:
		return nil, fmt.Errorf("invalid identifier type")
	}

	info, err := upload.GetInfo(ctx)
	if err != nil {
		return nil, err
	}

	if start > 0 {
		endPosition := start + info.Size - 1
		rangeHeader := fmt.Sprintf("bytes=%d-%d", start, endPosition)
		ctx = context.WithValue(ctx, CtxRangeKey, rangeHeader)
	}

	reader, err := upload.GetReader(ctx)
	if err != nil {
		return nil, err
	}

	return reader, nil
}

func (t *TusHandlerDefault) UploadSize(ctx context.Context, protocol core.StorageProtocol, identifier any) (uint64, error) {
	var exists bool
	var _upload *models.TUSRequest

	switch v := identifier.(type) {
	case core.StorageHash:
		exists, _upload = t.tusService.UploadHashExists(ctx, protocol, v)
	case string:
		exists, _upload = t.tusService.UploadExists(ctx, protocol, v)
	default:
		return 0, fmt.Errorf("invalid identifier type")
	}

	if !exists {
		return 0, gorm.ErrRecordNotFound
	}

	meta, err := t.tusStore.GetUpload(ctx, _upload.TUSUploadID)
	if err != nil {
		return 0, err
	}

	info, err := meta.GetInfo(ctx)
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

func (t *TusHandlerDefault) DeleteUpload(ctx context.Context, id string) error {
	objectId, _ := splitIds(id)

	// Try both IDs for each file type
	for _, deleteId := range []string{id, objectId} {
		if deleteId == "" {
			continue
		}

		// Check main file
		_, err := t.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(t.config.Config().Core.Storage.S3.BufferBucket),
			Key:    aws.String(deleteId),
		})
		if err == nil {
			_, err = t.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(t.config.Config().Core.Storage.S3.BufferBucket),
				Key:    aws.String(deleteId),
			})
			if err != nil {
				t.logger.Error("failed to delete upload object", zap.Error(err))
			}
		}

		// Check info file
		_, err = t.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(t.config.Config().Core.Storage.S3.BufferBucket),
			Key:    aws.String(deleteId + ".info"),
		})
		if err == nil {
			_, err = t.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(t.config.Config().Core.Storage.S3.BufferBucket),
				Key:    aws.String(deleteId + ".info"),
			})
			if err != nil {
				t.logger.Error("failed to delete upload metadata", zap.Error(err))
			}
		}
	}

	return nil
}

func (t *TusHandlerDefault) init(handlerConfig core.TUSHandlerConfig) error {
	// Validate handler config
	if t.handlerConfig.Protocol == nil {
		return fmt.Errorf("handler config Protocol cannot be nil")
	}

	if _, ok := t.handlerConfig.Protocol.(core.StorageProtocol); !ok {
		return fmt.Errorf("handler config Protocol must implement core.StorageProtocol")
	}

	s3Client, err := t.storage.S3Client(context.Background())
	if err != nil {
		return err
	}

	store := s3store.New(t.config.Config().Core.Storage.S3.BufferBucket, s3Client)

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
		BasePath:                handlerConfig.BasePath,
		StoreComposer:           composer,
		DisableDownload:         true,
		NotifyCompleteUploads:   true,
		NotifyTerminatedUploads: true,
		NotifyCreatedUploads:    true,
		RespectForwardedHeaders: true,
		PreUploadCreateCallback: handlerConfig.PreUpload,
		Logger:                  loggerToSlog(t.logger),
	})

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
	ctx := t.ctx

	// Handle created uploads
	go func() {
		for {
			select {
			case <-ctx.Done():
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
			case <-ctx.Done():
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
			case <-ctx.Done():
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
			case <-ctx.Done():
				return
			case info := <-t.tus.CompleteUploads:
				if t.handlerConfig.CompletedUploadHandler != nil {
					t.handlerConfig.CompletedUploadHandler(t, info)
				}
			}
		}
	}()
}

func getLockerMode(cm config.Manager, logger *core.Logger) string {
	cfg := cm.Config().Core.Storage.Tus

	switch cfg.LockerMode {
	case "", "none":
		return "none"
	case "db":
		return "db"
	case "redis":
		if cm.Config().Core.Clustered.Enabled {
			return "redis"
		}

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
	case "redis":
		client, err := cm.Config().Core.Clustered.Redis.Client()
		if err != nil {
			return nil, err
		}
		locker, err := redislocker.NewWithClient(client, redislocker.WithLogger(loggerToSlog(logger)))
		if err != nil {
			return nil, err
		}
		return locker, nil
	}

	return nil, nil
}

func DefaultUploadCreatedHandler(ctx core.Context, verifyFunc core.TUSUploadCreatedVerifyFunc, afterFunc core.TUSUploadCreatedAfterFunc) core.TUSUploadCallbackHandler {
	return func(handlr core.TusHandler, hook handler.HookEvent) {
		var errMessage string

		echoCtx, ok := getEchoContext(hook.Context)
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

func DefaultUploadCompletedHandler(ctx core.Context, processHandler core.TUSUploadCallbackHandler) core.TUSUploadCallbackHandler {
	return func(handlr core.TusHandler, hook handler.HookEvent) {
		sp, err := handlr.StorageProtocol()
		if err != nil {
			errMessage := "Failed to get storage protocol"
			handlr.HandleEventResponseError(errMessage, http.StatusBadRequest, hook)
			ctx.Logger().Error(errMessage, zap.Error(err))
			return
		}
		err = core.GetService[core.TUSService](ctx, core.TUS_SERVICE).UploadProcessing(ctx, sp, hook.Upload.ID)
		if err != nil {
			errMessage := "Failed to update upload status"
			handlr.HandleEventResponseError(errMessage, http.StatusInternalServerError, hook)
			ctx.Logger().Error(errMessage, zap.Error(err))
			return
		}

		if processHandler != nil {
			processHandler(handlr, hook)
		}
	}
}
func loggerToSlog(logger *core.Logger) *slog.Logger {
	return slog.New(zapslog.NewHandler(logger.Core()))
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

func getEchoContext(ctx context.Context) (echo.Context, bool) {
	c, ok := ctx.Value(echoContextKey).(echo.Context)
	return c, ok
}
