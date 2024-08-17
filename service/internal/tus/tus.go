package tus

import (
	"context"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gorilla/mux"
	"github.com/tus/tusd-etcd3-locker/pkg/etcd3locker"
	"github.com/tus/tusd/v2/pkg/handler"
	"github.com/tus/tusd/v2/pkg/s3store"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/middleware"
	"go.uber.org/zap"
	"go.uber.org/zap/exp/zapslog"
	"gorm.io/gorm"
	"io"
	"log/slog"
	"net/http"
)

type CtxRangeKeyType string

const CtxRangeKey CtxRangeKeyType = "range"

type preUploadCreateCallback func(hook handler.HookEvent) (handler.HTTPResponse, handler.FileInfoChanges, error)
type UploadCreatedVerifyFunc func(hook handler.HookEvent, uploaderId uint) (core.StorageHash, error)
type UploadCreatedAfterFunc func(requestId uint) error

type UploadCallbackHandler func(*TusHandler, handler.HookEvent)

type TusHandler struct {
	handlerConfig   HandlerConfig
	ctx             core.Context
	db              *gorm.DB
	config          config.Manager
	logger          *core.Logger
	tusService      core.TUSService
	cron            core.CronService
	storage         core.StorageService
	users           core.UserService
	metadata        core.MetadataService
	tus             *handler.Handler
	tusStore        handler.DataStore
	s3Client        *s3.Client
	storageProtocol core.StorageProtocol
}

type HandlerConfig struct {
	BasePath                string
	PreUpload               preUploadCreateCallback
	CreatedUploadHandler    UploadCallbackHandler
	UploadProgressHandler   UploadCallbackHandler
	TerminatedUploadHandler UploadCallbackHandler
	CompletedUploadHandler  UploadCallbackHandler
}

func NewTusHandler(
	ctx core.Context, handlerConfig HandlerConfig) (*TusHandler, error) {
	th := &TusHandler{
		handlerConfig: handlerConfig,
		ctx:           ctx,
		db:            ctx.DB(),
		config:        ctx.Config(),
		logger:        ctx.Logger(),
		tusService:    core.GetService[core.TUSService](ctx, core.TUS_SERVICE),
		cron:          core.GetService[core.CronService](ctx, core.CRON_SERVICE),
		storage:       core.GetService[core.StorageService](ctx, core.STORAGE_SERVICE),
		users:         core.GetService[core.UserService](ctx, core.USER_SERVICE),
		metadata:      core.GetService[core.MetadataService](ctx, core.METADATA_SERVICE),
	}

	err := th.init(handlerConfig)
	if err != nil {
		return nil, err
	}

	return th, nil
}

func (t *TusHandler) UploadReader(ctx context.Context, identifier any, start int64) (io.ReadCloser, error) {
	var upload handler.Upload

	switch v := identifier.(type) {
	case core.StorageHash:
		exists, _upload := t.tusService.UploadHashExists(ctx, v)

		if !exists {
			return nil, gorm.ErrRecordNotFound
		}

		meta, err := t.tusStore.GetUpload(ctx, _upload.TUSUploadID)
		if err != nil {
			return nil, err
		}

		upload = meta
	case string:
		exists, _upload := t.tusService.UploadExists(ctx, v)

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

func (t *TusHandler) UploadSize(ctx context.Context, hash core.StorageHash) (uint64, error) {
	exists, _upload := t.tusService.UploadHashExists(ctx, hash)

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

func (t *TusHandler) SetupRoute(router *mux.Router, authMw mux.MiddlewareFunc, path string) {
	subrouter := router.PathPrefix(path).Subrouter()
	subrouter.Use(middleware.TusPathMiddleware(path))
	subrouter.Use(middleware.TusCorsMiddleware())
	if authMw != nil {
		subrouter.Use(authMw)
	}

	tusHandler := func(w http.ResponseWriter, r *http.Request) {
		t.tus.ServeHTTP(w, r)
	}

	subrouter.HandleFunc("", tusHandler).Methods(http.MethodPost, http.MethodOptions)
	subrouter.HandleFunc("/{id}", tusHandler).Methods(http.MethodPatch, http.MethodHead, http.MethodOptions)
}

func (t *TusHandler) SetStorageProtocol(storageProtocol core.StorageProtocol) {
	t.storageProtocol = storageProtocol
}

func (t *TusHandler) StorageProtocol() core.StorageProtocol {
	return t.storageProtocol
}

func (t *TusHandler) HandleEventResponseError(message string, httpCode int, hook handler.HookEvent) {
	resp := handler.HTTPResponse{StatusCode: httpCode, Header: nil, Body: message}
	hook.Upload.StopUpload(resp)
}

func (t *TusHandler) CompleteUpload(ctx context.Context, hash core.StorageHash) error {
	exists, _upload := t.tusService.UploadHashExists(ctx, hash)

	if !exists {
		return gorm.ErrRecordNotFound
	}

	err := t.tusService.UploadCompleted(ctx, _upload.TUSUploadID)
	if err != nil {
		return err
	}

	err = t.deleteUpload(ctx, _upload.TUSUploadID)

	if err != nil {
		return err
	}

	return nil
}

func (t *TusHandler) FailUploadById(ctx context.Context, id string) error {
	err := t.tusService.DeleteUpload(ctx, id)
	if err != nil {
		return err
	}

	err = t.deleteUpload(ctx, id)

	if err != nil {
		return err
	}

	return nil
}

func (t *TusHandler) SetHashById(ctx context.Context, id string, hash core.StorageHash) error {
	err := t.tusService.SetHash(ctx, id, hash)
	if err != nil {
		return err
	}

	return nil

}
func (t *TusHandler) deleteUpload(ctx context.Context, id string) error {
	err := t.tusService.DeleteUpload(ctx, id)
	if err != nil {
		return err
	}

	upload, err := t.tusStore.GetUpload(ctx, id)
	if err != nil {
		return err
	}

	info, err := upload.GetInfo(ctx)
	if err != nil {
		return err
	}

	_, err = t.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(t.config.Config().Core.Storage.S3.BufferBucket),
		Key:    aws.String(info.ID),
	})
	if err != nil {
		t.logger.Error("failed to delete upload object from s3 buffer", zap.Error(err))
	}

	_, err = t.s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(t.config.Config().Core.Storage.S3.BufferBucket),
		Key:    aws.String(info.ID + ".info"),
	})

	if err != nil {
		t.logger.Error("failed to delete upload metadata from s3 buffer", zap.Error(err))
	}

	return nil
}

func (t *TusHandler) init(handlerConfig HandlerConfig) error {
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
		Logger:                  slog.New(zapslog.NewHandler(t.logger.Core(), nil)),
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
func (t *TusHandler) worker() {
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
	case "etcd":
		if cm.Config().Core.Clustered.Enabled {
			return "etcd"
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
	case "etcd":
		client, err := cm.Config().Core.Clustered.Etcd.Client()
		if err != nil {
			return nil, err
		}
		locker, err := etcd3locker.NewWithPrefix(client, "s5-tus-locks")
		if err != nil {
			return nil, err
		}
		return locker, nil
	}

	return nil, nil
}

func DefaultUploadCreatedHandler(ctx core.Context, verifyFunc UploadCreatedVerifyFunc, afterFunc UploadCreatedAfterFunc) UploadCallbackHandler {
	return func(handlr *TusHandler, hook handler.HookEvent) {
		var errMessage string

		uploaderID, err := middleware.GetUserFromContext(hook.Context)

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

		var mimeType string

		for _, field := range []string{"mimeType", "mimetype", "filetype"} {
			typ, ok := hook.Upload.MetaData[field]
			if ok {
				mimeType = typ
				break
			}
		}

		req, err := core.GetService[core.TUSService](ctx, core.TUS_SERVICE).CreateUpload(ctx, hash, hook.Upload.ID, uploaderID, uploaderIP, handlr.StorageProtocol(), mimeType)
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

func DefaultUploadProgressHandler(ctx core.Context) UploadCallbackHandler {
	return func(handlr *TusHandler, hook handler.HookEvent) {
		err := core.GetService[core.TUSService](ctx, core.TUS_SERVICE).UploadProgress(ctx, hook.Upload.ID)
		if err != nil {
			errMessage := "Failed to update upload progress"
			handlr.HandleEventResponseError(errMessage, http.StatusInternalServerError, hook)
			ctx.Logger().Error(errMessage, zap.Error(err))
		}
	}
}

func DefaultUploadTerminatedHandler(ctx core.Context) UploadCallbackHandler {
	return func(handlr *TusHandler, hook handler.HookEvent) {
		err := core.GetService[core.TUSService](ctx, core.TUS_SERVICE).DeleteUpload(ctx, hook.Upload.ID)
		if err != nil {
			errMessage := "Failed to update upload status"
			handlr.HandleEventResponseError(errMessage, http.StatusInternalServerError, hook)
			ctx.Logger().Error(errMessage, zap.Error(err))
		}
	}
}

func DefaultUploadCompletedHandler(ctx core.Context, processHandler UploadCallbackHandler) UploadCallbackHandler {
	return func(handlr *TusHandler, hook handler.HookEvent) {
		err := core.GetService[core.TUSService](ctx, core.TUS_SERVICE).UploadProcessing(ctx, hook.Upload.ID)
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
