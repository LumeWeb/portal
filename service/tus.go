package service

import (
	"context"
	"errors"
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

var _ core.TUSService = (*TUSServiceDefault)(nil)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID: core.TUS_SERVICE,
		Factory: func() (core.Service, []core.ContextBuilderOption, error) {
			return NewTUSService()
		},
	})
}

type TUSServiceDefault struct {
	ctx      core.Context
	db       *gorm.DB
	logger   *core.Logger
	requests core.RequestService
}

func NewTUSService() (*TUSServiceDefault, []core.ContextBuilderOption, error) {
	storage := &TUSServiceDefault{}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			storage.ctx = ctx
			storage.db = ctx.DB()
			storage.logger = ctx.ServiceLogger(storage)
			storage.requests = core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)
			return nil
		}),
	)

	return storage, opts, nil
}

func (t *TUSServiceDefault) ID() string {
	return core.TUS_SERVICE
}

func (t *TUSServiceDefault) UploadExists(ctx context.Context, id string) (bool, *models.TUSRequest) {
	data, err := t.requests.QueryUploadData(ctx, models.RequestOperationTusUpload, &models.TUSRequest{TUSUploadID: id}, core.RequestFilter{
		Operation: models.RequestOperationTusUpload,
	})
	if err != nil {
		return false, nil
	}

	if data == nil {
		return false, nil
	}

	return true, data.(*models.TUSRequest)
}

func (t *TUSServiceDefault) UploadHashExists(ctx context.Context, hash core.StorageHash) (bool, *models.TUSRequest) {
	req, err := t.requests.GetRequestByUploadHash(ctx, hash, core.RequestFilter{
		Operation: models.RequestOperationTusUpload,
	})

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, nil
	}

	data, err := t.requests.GetUploadData(ctx, req.ID)
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
		uploadData, err := t.requests.GetUploadData(ctx, req.ID)
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
		UserID:    uploaderID,
		SourceIP:  uploaderIP,
		MimeType:  mimeType,
	}

	if hash != nil {
		upload.HashType = hash.Type()
	}

	request, err := t.requests.CreateRequest(ctx, upload, nil, &models.TUSRequest{TUSUploadID: uploadID})
	if err != nil {
		return nil, err
	}

	dataReq, err := t.requests.GetUploadData(ctx, request.ID)

	if err != nil {
		return nil, err
	}

	return dataReq.(*models.TUSRequest), nil
}

func (t *TUSServiceDefault) UploadProgress(ctx context.Context, uploadID string) error {
	upload, err := t.getUpload(ctx, uploadID)
	if err != nil {
		return err
	}

	upload.UpdatedAt = time.Now()

	err = t.requests.UpdateUploadData(ctx, upload.RequestID, upload)
	if err != nil {
		return err
	}

	return nil
}

func (t *TUSServiceDefault) UploadCompleted(ctx context.Context, uploadID string) error {
	upload, err := t.getUpload(ctx, uploadID)
	if err != nil {
		return err
	}

	err = t.requests.CompleteRequest(ctx, upload.RequestID)
	if err != nil {
		return err
	}

	return nil
}

func (t *TUSServiceDefault) DeleteUpload(ctx context.Context, uploadID string) error {
	upload, err := t.getUpload(ctx, uploadID)
	if err != nil {
		return err
	}

	err = t.requests.UpdateRequestStatus(ctx, upload.RequestID, models.RequestStatusFailed)
	if err != nil {
		return err
	}

	err = t.requests.DeleteRequest(ctx, upload.RequestID)
	if err != nil {
		return err
	}

	return nil
}

func (t *TUSServiceDefault) getUpload(ctx context.Context, uploadID string) (*models.TUSRequest, error) {
	data, err := t.requests.QueryUploadData(ctx, models.RequestOperationTusUpload, &models.TUSRequest{TUSUploadID: uploadID}, core.RequestFilter{
		Operation: models.RequestOperationTusUpload,
	})
	if err != nil {
		return nil, err
	}

	if data == nil {
		return nil, errors.New("upload not found")
	}

	return data.(*models.TUSRequest), nil
}

func CreateTusHandler(ctx core.Context, config TusHandlerConfig) (*tus.TusHandler, error) {
	handler, err := tus.NewTusHandler(ctx, config)
	if err != nil {
		ctx.Logger().Error("Failed to create tus handler", zap.Error(err))
		return nil, err
	}

	return handler, nil
}
func TUSDefaultUploadCreatedHandler(ctx core.Context, verifyFunc TUSUploadCreatedVerifyFunc) TUSUploadCallbackHandler {
	return tus.DefaultUploadCreatedHandler(ctx, verifyFunc)
}

func TUSDefaultUploadProgressHandler(ctx core.Context) TUSUploadCallbackHandler {
	return tus.DefaultUploadProgressHandler(ctx)
}

func TUSDefaultUploadCompletedHandler(ctx core.Context, processHandler TUSUploadCallbackHandler) TUSUploadCallbackHandler {
	return tus.DefaultUploadCompletedHandler(ctx, processHandler)
}

func TUSDefaultUploadTerminatedHandler(ctx core.Context) TUSUploadCallbackHandler {
	return tus.DefaultUploadTerminatedHandler(ctx)
}
