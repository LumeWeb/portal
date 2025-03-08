package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/db/models/data_models"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"strings"
	"sync"
)

var _ core.RequestService = (*RequestServiceDefault)(nil)

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
	models map[string]data_models.RequestDataModel
	mutex  sync.RWMutex
}

func (r *RequestServiceDefault) RegisterRequestModel(operation string, model data_models.RequestDataModel) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.models[operation] = model
	r.logger.Debug("Registered request model", zap.String("operation", operation))
}

func (r *RequestServiceDefault) GetRequestModel(operation string) (data_models.RequestDataModel, bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	model, ok := r.models[operation]
	return model, ok
}

func (r *RequestServiceDefault) CreateRequestModel(operation string) (data_models.RequestDataModel, error) {
	model, ok := r.GetRequestModel(operation)
	if !ok {
		return nil, fmt.Errorf("no model registered for operation: %s", operation)
	}
	return model.NewInstance().(data_models.RequestDataModel), nil
}

func NewRequestService() (*RequestServiceDefault, []core.ContextBuilderOption, error) {
	req := &RequestServiceDefault{
		models: make(map[string]data_models.RequestDataModel),
	}

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

func (r *RequestServiceDefault) CreateRequest(ctx context.Context, req *models.Request, data interface{}) (*models.Request, error) {
	// Find the operation handler
	_, handler, err := r.findOperationHandler(req.Operation)
	if err != nil {
		return nil, err
	}

	// Validate the request if handler available
	if handler != nil {
		if err := handler.ValidateRequest(ctx, req); err != nil {
			return nil, fmt.Errorf("request validation failed: %w", err)
		}
	}

	// Set default values if not specified
	if req.Status == "" {
		req.Status = models.RequestStatusPending
	}

	// Create the request
	var newReq models.Request
	if err := db.RetryableTransaction(r.ctx, r.db, func(tx *gorm.DB) *gorm.DB {
		return tx.WithContext(ctx).Create(req).Scan(&newReq)
	}); err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// If custom data provided, store it in the protocol-specific table
	if data != nil {
		// Get model for this operation
		model, err := r.CreateRequestModel(req.Operation)
		if err != nil {
			r.logger.Warn("No model registered for operation",
				zap.String("operation", req.Operation))
		} else {
			// Copy data from provided struct to model
			dataBytes, err := json.Marshal(data)
			if err != nil {
				return nil, err
			}

			err = json.Unmarshal(dataBytes, model)
			if err != nil {
				return nil, err
			}

			// Set request ID
			model.SetRequestID(newReq.ID)

			// Validate
			if err := model.Validate(); err != nil {
				return &newReq, fmt.Errorf("data validation failed: %w", err)
			}

			// Store in database
			if err := r.db.Create(model).Error; err != nil {
				return &newReq, fmt.Errorf("failed to store protocol  %w", err)
			}
		}
	}

	// Start async execution if handler provided
	if handler != nil {
		go func() {
			execCtx := context.Background()

			// Update status to processing
			if err := r.UpdateRequestStatus(execCtx, newReq.ID, models.RequestStatusProcessing); err != nil {
				r.logger.Error("Failed to update request status",
					zap.Error(err), zap.Uint("requestID", newReq.ID))
				return
			}

			// Execute the operation
			if err := handler.Execute(execCtx, &newReq); err != nil {
				r.logger.Error("Request execution failed",
					zap.Error(err), zap.Uint("requestID", newReq.ID))

				failErr := r.FailRequest(execCtx, newReq.ID, err.Error())
				if failErr != nil {
					r.logger.Error("Failed to mark request as failed",
						zap.Error(failErr), zap.Uint("requestID", newReq.ID))
				}
				return
			}
		}()
	}

	return &newReq, nil
}

func (r *RequestServiceDefault) GetRequest(ctx context.Context, id uint) (*models.Request, error) {
	var req models.Request
	err := r.db.Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.WithContext(ctx).First(&req, id)
		})
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("request not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get request: %w", err)
	}
	return &req, nil
}

func (r *RequestServiceDefault) UpdateRequest(ctx context.Context, req *models.Request) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.WithContext(ctx).Save(req)
		})
	})
}

func (r *RequestServiceDefault) DeleteRequest(ctx context.Context, id uint) error {
	req, err := r.GetRequest(ctx, id)
	if err != nil {
		return err
	}

	_, handler, err := r.findOperationHandler(req.Operation)
	if err != nil {
		r.logger.Warn("Could not find operation handler for cleanup",
			zap.Error(err), zap.String("operation", req.Operation))
	} else if handler != nil {
		if err := handler.Cleanup(ctx, req); err != nil {
			r.logger.Warn("Cleanup failed but continuing with deletion",
				zap.Error(err), zap.Uint("requestID", req.ID))
		}
	}

	return db.RetryableTransaction(r.ctx, r.db, func(tx *gorm.DB) *gorm.DB {
		return tx.WithContext(ctx).Delete(&models.Request{}, id)
	})
}

func (r *RequestServiceDefault) QueryRequest(ctx context.Context, query interface{}, filter core.RequestFilter) (*models.Request, error) {
	var req models.Request

	err := r.db.Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			tx := db.WithContext(ctx)
			if query != nil {
				tx = tx.Where(query)
			}

			return tx.Scopes(
				applyFilters(filter),
			).First(&req)
		})
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("request not found: %w", err)
		}
		return nil, fmt.Errorf("query failed: %w", err)
	}
	return &req, nil
}

func (r *RequestServiceDefault) GetRequestByHash(ctx context.Context, hash core.StorageHash, filter core.RequestFilter) (*models.Request, error) {
	var req models.Request
	req.Hash = hash.Multihash()

	err := r.db.Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.WithContext(ctx).
				Scopes(
					applyFilters(filter),
				).
				Where(&req).First(&req)
		})
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("request with hash not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get request: %w", err)
	}
	return &req, nil
}

func (r *RequestServiceDefault) GetRequestByUploadHash(ctx context.Context, hash core.StorageHash, filter core.RequestFilter) (*models.Request, error) {
	var req models.Request
	req.UploadHash = hash.Multihash()

	err := r.db.Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.WithContext(ctx).
				Scopes(
					applyFilters(filter),
				).
				Where(&req).First(&req)
		})
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("request with upload hash not found: %w", err)
		}
		return nil, fmt.Errorf("failed to get request: %w", err)
	}
	return &req, nil
}

func (r *RequestServiceDefault) ListRequestsByUser(ctx context.Context, userID uint, filter core.RequestFilter) ([]*models.Request, error) {
	var requests []*models.Request

	var req models.Request
	req.UserID = userID
	err := r.db.Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.WithContext(ctx).Where(&req).Scopes(
				applyFilters(filter),
			).Find(&requests)
		})
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list requests: %w", err)
	}
	return requests, nil
}

func (r *RequestServiceDefault) ListRequestsByStatus(ctx context.Context, status string, filter core.RequestFilter) ([]*models.Request, error) {
	var requests []*models.Request
	err := r.db.Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.WithContext(ctx).Where("status = ?", status).
				Scopes(
					applyFilters(filter),
				).Find(&requests)
		})
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list requests: %w", err)
	}
	return requests, nil
}

func (r *RequestServiceDefault) UpdateRequestStatus(ctx context.Context, id uint, status models.RequestStatusType) error {
	return db.RetryableTransaction(r.ctx, r.db, func(tx *gorm.DB) *gorm.DB {
		return tx.Model(&models.Request{}).
			Where("id = ?", id).
			Update("status", status)
	})
}

func (r *RequestServiceDefault) CompleteRequest(ctx context.Context, id uint) error {
	req, err := r.GetRequest(ctx, id)
	if err != nil {
		return err
	}

	// Don't complete if already completed or failed
	if req.Status == models.RequestStatusCompleted || req.Status == models.RequestStatusFailed {
		return nil
	}

	return r.UpdateRequestStatus(ctx, id, models.RequestStatusCompleted)
}

func (r *RequestServiceDefault) FailRequest(ctx context.Context, id uint, reason string) error {
	return db.RetryableTransaction(r.ctx, r.db, func(tx *gorm.DB) *gorm.DB {
		return tx.Model(&models.Request{}).
			Where("id = ?", id).
			Updates(map[string]interface{}{
				"status":         models.RequestStatusFailed,
				"status_message": reason,
			})
	})
}

func (r *RequestServiceDefault) GetRequestStatus(ctx context.Context, id uint) (*core.RequestStatus, error) {
	req, err := r.GetRequest(ctx, id)
	if err != nil {
		return nil, err
	}

	// Find operation handler
	_, handler, err := r.findOperationHandler(req.Operation)
	if err != nil {
		return nil, err
	}

	// Basic status from request model
	status := &core.RequestStatus{
		State:     string(req.Status),
		UpdatedAt: req.UpdatedAt,
	}

	// If we have a handler, get detailed status
	if handler != nil {
		detailedStatus, err := handler.GetStatus(ctx, req)
		if err != nil {
			r.logger.Warn("Failed to get detailed status from handler",
				zap.Error(err), zap.Uint("requestID", req.ID))
		} else {
			status = &detailedStatus
		}
	}

	// Set default message based on status if not provided
	if status.Message == "" {
		switch req.Status {
		case models.RequestStatusPending:
			status.Message = "Request is pending processing"
		case models.RequestStatusProcessing:
			status.Message = "Request is being processed"
		case models.RequestStatusCompleted:
			status.Message = "Request completed successfully"
		case models.RequestStatusFailed:
			status.Message = req.StatusMessage
			if status.Message == "" {
				status.Message = "Request failed"
			}
		}
	}

	return status, nil
}

func (r *RequestServiceDefault) RequestExists(ctx context.Context, id uint) (bool, error) {
	var exists bool
	err := r.db.Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.WithContext(ctx).
				Model(&models.Request{}).
				Select("count(*) > 0").
				Where("id = ?", id).
				Find(&exists)
		})
	})
	return exists, err
}

func (r *RequestServiceDefault) GetRequestData(ctx context.Context, req *models.Request) (interface{}, error) {
	// Get model for this operation
	model, err := r.CreateRequestModel(req.Operation)
	if err != nil {
		return nil, err
	}

	// Query the database
	if err := r.db.Where("request_id = ?", req.ID).First(model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // No data found, but not an error
		}
		return nil, err
	}

	return model, nil
}

func (r *RequestServiceDefault) UpdateRequestData(ctx context.Context, req *models.Request, data interface{}) error {
	// Get model for this operation
	model, err := r.CreateRequestModel(req.Operation)
	if err != nil {
		return err
	}

	// Copy data from provided struct to model
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal  %w", err)
	}

	if err = json.Unmarshal(dataBytes, model); err != nil {
		return fmt.Errorf("failed to unmarshal data into model: %w", err)
	}

	// Set request ID
	model.SetRequestID(req.ID)

	// Validate
	if err := model.Validate(); err != nil {
		return fmt.Errorf("data validation failed: %w", err)
	}

	// Store in database
	return r.db.Where("request_id = ?", req.ID).Save(model).Error
}

// findOperationHandler locates the operation and handler for a given operation type
func (r *RequestServiceDefault) findOperationHandler(operationType string) (core.Operation, core.OperationHandler, error) {
	// Parse operation type to find protocol
	parts := strings.Split(operationType, ".")
	if len(parts) < 2 {
		return nil, nil, fmt.Errorf("invalid operation type format: %s", operationType)
	}

	protocolName := parts[0]

	// Find protocol
	protocol := core.GetProtocol(protocolName)
	if protocol == nil {
		return nil, nil, fmt.Errorf("protocol not found: %s", protocolName)
	}

	// Get operations from protocol
	operations := protocol.Operations()

	// Find matching operation
	for _, op := range operations {
		if op.Type() == operationType {
			return op, op.Handler(), nil
		}
	}

	// Check for content scan operation
	if operationType == "content.scan" {
		// Return no-op scanner if no scanner registered
		scanner := core.NewNoContentScanner()
		scanOp := core.NewOperation("content.scan", core.OpTypeScan, scanner.(core.OperationHandler))
		return scanOp, scanner.(core.OperationHandler), nil
	}

	return nil, nil, fmt.Errorf("operation not found: %s", operationType)
}

// Helper functions
func applyFilters(filter core.RequestFilter) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if filter.Protocol != "" {
			db = db.Where("protocol = ?", filter.Protocol)
		}
		if filter.Operation != "" {
			db = db.Where("operation = ?", filter.Operation)
		}
		if filter.UserID > 0 {
			db = db.Where("user_id = ?", filter.UserID)
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
