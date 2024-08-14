package service

import (
	"context"
	"errors"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/db/models"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"reflect"
	"strings"
)

var _ core.RequestService = (*RequestServiceDefault)(nil)

const uploadOperationSuffix = "_upload"

func init() {
	core.RegisterService(core.ServiceInfo{
		ID: core.REQUEST_SERVICE,
		Factory: func() (core.Service, []core.ContextBuilderOption, error) {
			return NewRequestService()
		},
	})
}

type RequestServiceDefault struct {
	ctx    core.Context
	logger *core.Logger
	db     *gorm.DB
}

func NewRequestService() (*RequestServiceDefault, []core.ContextBuilderOption, error) {
	req := &RequestServiceDefault{}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			req.ctx = ctx
			req.logger = ctx.ServiceLogger(req)
			req.db = ctx.DB()
			return nil
		}),
	)

	return req, opts, nil
}

func (r *RequestServiceDefault) ID() string {
	return core.REQUEST_SERVICE
}

func (r *RequestServiceDefault) CreateRequest(ctx context.Context, req *models.Request, protocolData any, uploadData any) (*models.Request, error) {
	if !core.ProtocolHasDataRequestHandler(req.Protocol) {
		r.logger.Panic("protocol %s does not have a data request handler", zap.String("protocol", req.Protocol))
		return nil, nil
	}

	protocolDataHandler := core.GetProtocolDataRequestHandler(req.Protocol)

	if protocolData == nil {
		protocolData = protocolDataHandler.GetProtocolDataModel()
	} else {
		expectedType := reflect.TypeOf(protocolDataHandler.GetProtocolDataModel())
		actualType := reflect.TypeOf(protocolData)

		if expectedType != actualType {
			r.logger.Panic("invalid protocol data type", zap.String("expected", expectedType.String()), zap.String("actual", actualType.String()))
		}
	}

	var uploadDataHandler core.UploadDataHandler

	isUpload := isUploadOperation(req.Operation)

	if isUpload {
		var ok bool
		uploadDataHandler, ok = core.GetUploadDataHandler(getDataHandlerName(req.Operation))
		if !ok {
			r.ctx.Logger().Panic("no upload data handler found for operation: %s", zap.String("operation", string(req.Operation)))
		}

		if uploadData == nil {
			uploadData = uploadDataHandler.GetUploadDataModel()
		} else {
			expectedType := reflect.TypeOf(uploadDataHandler.GetUploadDataModel())
			actualType := reflect.TypeOf(uploadData)

			if expectedType != actualType {
				r.logger.Panic("invalid upload data type", zap.String("expected", expectedType.String()), zap.String("actual", actualType.String()))
			}
		}
	}

	var newReq models.Request

	if req.Status == "" {
		req.Status = models.RequestStatusPending
	}

	if err := r.ctx.DB().Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.WithContext(ctx).FirstOrCreate(&newReq, req)
		})
	}); err != nil {
		return nil, err
	}

	if err := protocolDataHandler.CreateProtocolData(ctx, newReq.ID, protocolData); err != nil {
		return nil, err
	}

	if isUpload {
		if err := uploadDataHandler.CreateUploadData(ctx, r.ctx.DB().WithContext(r.ctx), newReq.ID, uploadData); err != nil {
			return nil, err
		}
	}

	return &newReq, nil
}

func (r *RequestServiceDefault) GetRequest(ctx context.Context, id uint) (*models.Request, error) {
	var req models.Request
	err := r.ctx.DB().Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.WithContext(ctx).Unscoped().First(&req, id)
		})
	})
	if err != nil {
		return nil, err
	}
	return &req, nil
}

func (r *RequestServiceDefault) UpdateRequest(ctx context.Context, req *models.Request) error {
	return r.ctx.DB().Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.WithContext(ctx).Unscoped().Save(req)
		})
	})
}

func (r *RequestServiceDefault) QueryRequest(ctx context.Context, query interface{}, filter core.RequestFilter) (*models.Request, error) {
	var req models.Request

	err := r.ctx.DB().Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			tx = db.WithContext(ctx)
			if query != nil {
				tx = tx.Where(query)
			}

			return tx.Scopes(
				applyFilters(filter),
			).First(&req)
		})
	})
	if err != nil {
		return nil, err
	}
	return &req, nil
}

func (r *RequestServiceDefault) DeleteRequest(ctx context.Context, id uint) error {
	req, err := r.GetRequest(ctx, id)
	if err != nil {
		return err
	}

	if req.DeletedAt.Valid {
		return nil
	}

	if !core.ProtocolHasDataRequestHandler(req.Protocol) {
		r.logger.Panic("protocol %s does not have a data request handler", zap.String("protocol", req.Protocol))
		return nil
	}

	protocolDataHandler := core.GetProtocolDataRequestHandler(req.Protocol)

	uploadDataHandler, ok := core.GetUploadDataHandler(getDataHandlerName(req.Operation))

	if !ok {
		r.ctx.Logger().Panic("no upload data handler found for operation: %s", zap.String("operation", string(req.Operation)))
	}

	err = r.ctx.DB().Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.WithContext(ctx).Delete(&models.Request{}, id)
		})
	})

	if err != nil {
		return err
	}

	if err = protocolDataHandler.DeleteProtocolData(ctx, id); err != nil {
		return err
	}

	if err = uploadDataHandler.DeleteUploadData(ctx, r.db, id); err != nil {
		return err
	}

	return nil
}

func (r *RequestServiceDefault) CompleteRequest(ctx context.Context, id uint) error {
	req, err := r.GetRequest(ctx, id)
	if err != nil {
		return err
	}

	if !core.ProtocolHasDataRequestHandler(req.Protocol) {
		r.logger.Panic("protocol %s does not have a data request handler", zap.String("protocol", req.Protocol))
		return nil
	}

	var uploadHandler core.UploadDataHandler

	isUpload := isUploadOperation(req.Operation)

	protocolDataHandler := core.GetProtocolDataRequestHandler(req.Protocol)

	if isUpload {
		var ok bool
		uploadHandler, ok = core.GetUploadDataHandler(getDataHandlerName(req.Operation))
		if !ok {
			r.ctx.Logger().Panic("no upload data handler found for operation: %s", zap.String("operation", string(req.Operation)))
			return nil
		}
	}

	if req.Status != models.RequestStatusCompleted {
		err = r.UpdateRequestStatus(ctx, id, models.RequestStatusCompleted)
		if err != nil {
			return err
		}
	}

	if err = protocolDataHandler.CompleteProtocolData(ctx, id); err != nil {
		return err
	}

	if isUpload {
		if err = uploadHandler.CompleteUploadData(ctx, r.db, id); err != nil {
			return err
		}
	}

	return nil
}

func (r *RequestServiceDefault) GetRequestByHash(ctx context.Context, hash core.StorageHash, filter core.RequestFilter) (*models.Request, error) {
	var req models.Request

	req.Hash = hash.Multihash()

	err := r.ctx.DB().Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.WithContext(ctx).
				Scopes(
					applyFilters(filter),
				).
				Where(&req).First(&req)
		})
	})
	if err != nil {
		return nil, err
	}
	return &req, nil
}

func (r *RequestServiceDefault) GetRequestByUploadHash(ctx context.Context, hash core.StorageHash, filter core.RequestFilter) (*models.Request, error) {
	var req models.Request

	req.UploadHash = hash.Multihash()

	err := r.ctx.DB().Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.WithContext(ctx).
				Scopes(
					applyFilters(filter),
				).
				Where(&req).First(&req)
		})
	})
	if err != nil {
		return nil, err
	}
	return &req, nil
}

func (r *RequestServiceDefault) ListRequestsByUser(ctx context.Context, userID uint, filter core.RequestFilter) ([]*models.Request, error) {
	var requests []*models.Request

	var req models.Request

	req.UserID = userID
	err := r.ctx.DB().Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.WithContext(ctx).Where(&req).Scopes(
				applyFilters(filter),
			).Find(&requests)
		})
	})
	if err != nil {
		return nil, err
	}
	return requests, nil
}

func (r *RequestServiceDefault) ListRequestsByStatus(ctx context.Context, status string, filter core.RequestFilter) ([]*models.Request, error) {
	var requests []*models.Request
	err := r.ctx.DB().Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.WithContext(ctx).Where("status = ?", status).
				Scopes(
					applyFilters(filter),
				).Find(&requests)
		})
	})
	if err != nil {
		return nil, err
	}
	return requests, nil
}

func (r *RequestServiceDefault) UpdateRequestStatus(ctx context.Context, id uint, status models.RequestStatusType) error {
	return r.ctx.DB().Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.WithContext(ctx).Model(&models.Request{}).Where("id = ?", id).Update("status", status)
		})
	})
}

func (r *RequestServiceDefault) RequestExists(ctx context.Context, id uint) (bool, error) {
	var exists bool
	err := r.ctx.DB().Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.WithContext(ctx).Model(&models.Request{}).Select("count(*) > 0").Where("id = ?", id).Find(&exists)
		})
	})
	return exists, err
}

func (r *RequestServiceDefault) UpdateProtocolData(ctx context.Context, id uint, data any) error {
	req, err := r.GetRequest(ctx, id)
	if err != nil {
		return err
	}

	if !core.ProtocolHasDataRequestHandler(req.Protocol) {
		r.logger.Panic("protocol %s does not have a data request handler", zap.String("protocol", req.Protocol))
		return nil
	}

	handler := core.GetProtocolDataRequestHandler(req.Protocol)

	if data == nil {
		data = handler.GetProtocolDataModel()
	}

	return handler.UpdateProtocolData(ctx, id, data)
}

func (r *RequestServiceDefault) GetProtocolData(ctx context.Context, id uint) (any, error) {
	req, err := r.GetRequest(ctx, id)
	if err != nil {
		return nil, err
	}

	if !core.ProtocolHasDataRequestHandler(req.Protocol) {
		r.logger.Panic("protocol %s does not have a data request handler", zap.String("protocol", req.Protocol))
		return nil, nil
	}

	return core.GetProtocolDataRequestHandler(req.Protocol).GetProtocolData(ctx, r.db.Preload("Request"), id)
}

func (r *RequestServiceDefault) QueryProtocolData(ctx context.Context, protocol string, query any, filter core.RequestFilter) (interface{}, error) {
	if !core.ProtocolHasDataRequestHandler(protocol) {
		r.ctx.Logger().Panic("protocol %s does not have a data request handler", zap.String("protocol", protocol))
	}

	handler := core.GetProtocolDataRequestHandler(protocol)

	model := handler.GetProtocolDataModel()
	mt := reflect.TypeOf(model)

	if mt.Kind() == reflect.Ptr {
		mt = mt.Elem()
	}

	// Create a new instance of the model type
	result := reflect.New(mt).Interface()

	err := r.ctx.DB().WithContext(ctx).Unscoped().Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			tx = db.Model(result)
			tx = handler.QueryProtocolData(ctx, tx, query)

			if tx == nil {
				r.logger.Panic("QueryProtocolData returned nil")
			}

			tx = tx.Joins("Request")

			return tx.Scopes(applyFilters(filter)).First(result)
		})
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return result, nil
}

func (r *RequestServiceDefault) CreateUploadData(ctx context.Context, id uint, data any) error {
	req, err := r.GetRequest(ctx, id)
	if err != nil {
		return err
	}

	if !isUploadOperation(req.Operation) {
		return nil
	}

	if handler, ok := core.GetUploadDataHandler(getDataHandlerName(req.Operation)); ok {
		if data == nil {
			data = handler.GetUploadDataModel()
		}

		if data == nil {
			return nil
		}

		return handler.CreateUploadData(ctx, r.db, id, data)
	}

	r.ctx.Logger().Panic("no upload data handler found for operation: %s", zap.String("operation", string(req.Operation)))

	return nil
}

func (r *RequestServiceDefault) GetUploadData(ctx context.Context, id uint) (any, error) {
	req, err := r.GetRequest(ctx, id)
	if err != nil {
		return nil, err
	}

	if !isUploadOperation(req.Operation) {
		return nil, nil
	}

	if handler, ok := core.GetUploadDataHandler(getDataHandlerName(req.Operation)); ok {
		return handler.GetUploadData(ctx, r.db.Preload("Request"), id)
	}

	r.ctx.Logger().Panic("no upload data handler found for operation: %s", zap.String("operation", string(req.Operation)))

	return nil, nil
}

func (r *RequestServiceDefault) UpdateUploadData(ctx context.Context, id uint, data any) error {
	req, err := r.GetRequest(ctx, id)
	if err != nil {
		return err
	}

	if !isUploadOperation(req.Operation) {
		return nil
	}

	if handler, ok := core.GetUploadDataHandler(getDataHandlerName(req.Operation)); ok {
		if data == nil {
			data = handler.GetUploadDataModel()
		}
		if data == nil {
			return nil
		}

		return handler.UpdateUploadData(ctx, r.db, id, data)
	}

	r.ctx.Logger().Panic("no upload data handler found for operation: %s", zap.String("operation", string(req.Operation)))

	return nil
}

func (r *RequestServiceDefault) DeleteUploadData(ctx context.Context, id uint) error {
	req, err := r.GetRequest(ctx, id)
	if err != nil {
		return err
	}

	if !isUploadOperation(req.Operation) {
		return nil
	}

	handler, ok := core.GetUploadDataHandler(getDataHandlerName(req.Operation))
	if !ok {
		r.ctx.Logger().Panic("no upload data handler found for operation: %s", zap.String("operation", string(req.Operation)))
		return nil
	}

	return handler.DeleteUploadData(ctx, r.db, id)
}

func (r *RequestServiceDefault) QueryUploadData(ctx context.Context, uploadMethod models.RequestOperationType, query any, filter core.RequestFilter) (any, error) {
	isUpload := isUploadOperation(uploadMethod)

	if !isUpload {
		return nil, nil
	}

	handler, ok := core.GetUploadDataHandler(getDataHandlerName(uploadMethod))
	if !ok {
		r.ctx.Logger().Panic("no upload data handler found for operation: %s", zap.String("operation", string(uploadMethod)))
		return nil, nil
	}

	model := handler.GetUploadDataModel()
	mt := reflect.TypeOf(model)

	if mt.Kind() == reflect.Ptr {
		mt = mt.Elem()
	}

	// Create a new instance of the model type
	result := reflect.New(mt).Interface()

	err := r.ctx.DB().WithContext(ctx).Unscoped().Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			tx = db.Model(result)
			tx = handler.QueryUploadData(ctx, tx, query)

			if tx == nil {
				r.logger.Panic("QueryUploadData returned nil")
			}

			tx = tx.Joins("Request")

			return tx.Scopes(applyUploadDataFilters(filter)).First(result)
		})
	})

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}

	return result, nil
}

func (r *RequestServiceDefault) CompleteUploadData(ctx context.Context, id uint) error {
	req, err := r.GetRequest(ctx, id)
	if err != nil {
		return err
	}

	if !isUploadOperation(req.Operation) {
		return nil
	}

	handler, ok := core.GetUploadDataHandler(getDataHandlerName(req.Operation))
	if !ok {
		r.ctx.Logger().Panic("no upload data handler found for operation: %s", zap.String("operation", string(req.Operation)))
		return nil
	}

	return handler.CompleteUploadData(ctx, r.db, id)
}

func isUploadOperation[T string | models.RequestOperationType](operation T) bool {
	return strings.HasSuffix(string(operation), uploadOperationSuffix)
}

func getDataHandlerName[T string | models.RequestOperationType](operation T) string {
	handlerName := string(operation)
	return strings.TrimSuffix(handlerName, uploadOperationSuffix)
}

func applyFilters(filter core.RequestFilter) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if filter.Protocol != "" {
			db = db.Where("Request.protocol = ?", filter.Protocol)
		}
		if filter.Operation != "" {
			db = db.Where("Request.operation = ?", filter.Operation)
		}
		if filter.Limit > 0 {
			db = db.Limit(filter.Limit)
		}
		if filter.Offset > 0 {
			db = db.Offset(filter.Offset)
		}

		return db
	}
}

func applyUploadDataFilters(filter core.RequestFilter) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if filter.Protocol != "" {
			db = db.Where("Request.protocol = ?", filter.Protocol)
		}
		if filter.Operation != "" {
			db = db.Where("Request.operation = ?", filter.Operation)
		}
		if filter.Limit > 0 {
			db = db.Limit(filter.Limit)
		}
		if filter.Offset > 0 {
			db = db.Offset(filter.Offset)
		}

		return db
	}
}
