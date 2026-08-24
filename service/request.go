package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sync"
	"time"

	"github.com/fatih/structs"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/db/models/data_models"
	requestMetrics "go.lumeweb.com/portal/service/internal/request"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"go.lumeweb.com/queryutil"
)

var _ core.RequestService = (*RequestServiceDefault)(nil)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID:      core.REQUEST_SERVICE,
		Factory: NewRequestService,
		Metrics: requestMetrics.GetCollectors(),
	})
}

type RequestServiceDefault struct {
	*core.BaseComponent
	models map[string]data_models.RequestDataModel
	mutex  sync.RWMutex
	ops    core.OperationFinder
}

func (r *RequestServiceDefault) RegisterRequestModel(operation string, model data_models.RequestDataModel) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	r.models[operation] = model
	r.Logger().Debug("Registered request model", zap.String("operation", operation))
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

func (r *RequestServiceDefault) ListDistinctRequestFilters(ctx context.Context, userID uint, additionalFilters []queryutil.CrudFilter) (map[string][]string, error) {
	ctx, span := core.TraceMethod(ctx, "RequestServiceDefault.ListDistinctRequestFilters")
	defer span.End()

	buildBaseQuery := func() *gorm.DB {
		q := r.DB().Model(&models.Request{}).Where("user_id = ?", userID)
		if len(additionalFilters) > 0 {
			q = queryutil.ApplyFilters(q, additionalFilters, nil)
		}
		return q
	}

	var (
		statuses   []string
		operations []string
		protocols  []string
	)

	if err := buildBaseQuery().
		Distinct(core.RequestFieldStatus).
		Pluck(core.RequestFieldStatus, &statuses).Error; err != nil {
		return nil, fmt.Errorf("failed to query distinct statuses: %w", err)
	}

	if err := buildBaseQuery().
		Distinct(core.RequestFieldOperation).
		Pluck(core.RequestFieldOperation, &operations).Error; err != nil {
		return nil, fmt.Errorf("failed to query distinct operations: %w", err)
	}

	if err := buildBaseQuery().
		Where(core.RequestFieldProtocol+" <> ''").
		Distinct(core.RequestFieldProtocol).
		Pluck(core.RequestFieldProtocol, &protocols).Error; err != nil {
		return nil, fmt.Errorf("failed to query distinct protocols: %w", err)
	}

	return map[string][]string{
		core.FilterRequestKeyStatuses:   statuses,
		core.FilterRequestKeyOperations: operations,
		core.FilterRequestKeyProtocols:  protocols,
	}, nil
}

func NewRequestService() (core.Service, []core.ContextBuilderOption, error) {
	req := &RequestServiceDefault{
		models: make(map[string]data_models.RequestDataModel),
	}

	return req, core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			req.ops = NewOperationFinder(ctx)
			return nil
		}),
	), nil
}

func (r *RequestServiceDefault) ID() string {
	return core.REQUEST_SERVICE
}

func (r *RequestServiceDefault) CreateRequest(ctx context.Context, req *models.Request, data interface{}) (*models.Request, error) {
	ctx, span := core.TraceMethod(ctx, "RequestServiceDefault.CreateRequest")
	defer span.End()

	protocol := req.Protocol
	if protocol == "" {
		protocol = requestMetrics.LabelProtocolUnknown
	}
	operation := req.Operation
	if operation == "" {
		operation = requestMetrics.LabelOperationUnknown
	}

	return core.MetricTrackResult(
		requestMetrics.RequestDuration.WithLabelValues(operation),
		requestMetrics.RequestsFailed.WithLabelValues(protocol, operation),
		func() (*models.Request, error) {
			// Validate the request
			if err := r.ValidateRequest(ctx, req); err != nil {
				requestMetrics.RequestsValidationFailed.WithLabelValues(protocol, operation).Inc()
				return nil, fmt.Errorf("request validation failed: %w", err)
			}

			// Set default values if not specified
			if req.Status == "" {
				req.Status = models.RequestStatusPending
			} else if req.Status == models.RequestStatusDuplicate {
				requestMetrics.RequestsDuplicate.WithLabelValues(protocol, operation).Inc()
			}

			// Create the request
			var newReq models.Request
			if err := db.RetryableComponentTransaction(r, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.WithContext(ctx).Create(req).Scan(&newReq)
			}); err != nil {
				return nil, fmt.Errorf("failed to create request: %w", err)
			}

			// If custom data provided, store it in the protocol-specific table
			if data != nil {
				// Get model for this operation
				model, err := r.CreateRequestModel(req.Operation)
				if err != nil {
					r.Logger().Warn("No model registered for operation",
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
					model.SetRequest(newReq)

					// Validate
					if err = model.Validate(); err != nil {
						return &newReq, fmt.Errorf("data validation failed: %w", err)
					}

					// Store in database
					if err = db.RetryableComponentTransaction(r, ctx, func(tx *gorm.DB) *gorm.DB {
						return tx.Create(model)
					}); err != nil {
						return &newReq, fmt.Errorf("failed to store protocol  %w", err)
					}
				}
			}

			requestMetrics.RequestsCreated.WithLabelValues(protocol, operation).Inc()
			requestMetrics.RequestsByStatus.WithLabelValues(string(newReq.Status)).Inc()
			requestMetrics.RequestsByOperation.WithLabelValues(operation).Inc()
			if protocol != requestMetrics.LabelProtocolUnknown {
				requestMetrics.RequestsByProtocol.WithLabelValues(protocol).Inc()
			}

			return &newReq, nil
		},
	)
}

func (r *RequestServiceDefault) ExecuteRequest(ctx context.Context, id uint) error {
	ctx, span := core.TraceMethod(ctx, "RequestServiceDefault.ExecuteRequest")
	defer span.End()

	req, err := r.GetRequest(ctx, id)
	if err != nil {
		return err
	}

	operation := req.Operation
	if operation == "" {
		operation = requestMetrics.LabelOperationExecute
	}

	protocol := req.Protocol
	if protocol == "" {
		protocol = requestMetrics.LabelProtocolUnknown
	}

	return core.MetricTrack(
		requestMetrics.RequestDuration.WithLabelValues(operation),
		requestMetrics.RequestsFailed.WithLabelValues(protocol, operation),
		func() error {
			// Compute current status
			status, err := r.ComputeRequestStatus(ctx, id, false)
			if err != nil {
				return err
			}

			if status.State == "" {
				status.State = models.RequestStatusProcessing
			}

			// Update status based on computed state
			if err := r.UpdateRequestStatus(ctx, id, status.State, status.Message); err != nil {
				r.Logger().Error("Failed to update request status",
					zap.Error(err), zap.Uint("requestID", id))
				return err
			}

			// Find the operation handler
			_, handler, err := r.ops.FindOperationHandler(req.Operation)
			if err != nil {
				return err
			}

			if handler == nil {
				return nil // No handler means nothing to execute
			}

			// Execute the operation
			if err := handler.Execute(ctx, req); err != nil {
				r.Logger().Error("Request execution failed",
					zap.Error(err), zap.Uint("requestID", id))

				failErr := r.FailRequest(ctx, id, err.Error())
				if failErr != nil {
					r.Logger().Error("Failed to mark request as failed",
						zap.Error(failErr), zap.Uint("requestID", id))
				}
				return err
			}

			// Recompute status after execution
			status, err = r.ComputeRequestStatus(ctx, id, true)
			if err != nil {
				return err
			}

			// Update final status
			return r.UpdateRequestStatus(ctx, id, status.State, status.Message)
		},
	)
}

func (r *RequestServiceDefault) GetRequest(ctx context.Context, id uint) (*models.Request, error) {
	ctx, span := core.TraceMethod(ctx, "RequestServiceDefault.GetRequest")
	defer span.End()

	return core.MetricTrackResult(
		requestMetrics.RequestDuration.WithLabelValues(requestMetrics.LabelQueryTypeGet),
		nil,
		func() (*models.Request, error) {
			result, err := r.getRequest(ctx, id, false)
			if err == nil {
				requestMetrics.RequestsQueryTotal.WithLabelValues(requestMetrics.LabelQueryTypeGet).Inc()
			}
			return result, err
		},
	)
}

func (r *RequestServiceDefault) GetRequestWithDeleted(ctx context.Context, id uint) (*models.Request, error) {
	ctx, span := core.TraceMethod(ctx, "RequestServiceDefault.GetRequestWithDeleted")
	defer span.End()

	return core.MetricTrackResult(
		requestMetrics.RequestDuration.WithLabelValues(requestMetrics.LabelQueryTypeGet),
		nil,
		func() (*models.Request, error) {
			result, err := r.getRequest(ctx, id, true)
			if err == nil {
				requestMetrics.RequestsQueryTotal.WithLabelValues(requestMetrics.LabelQueryTypeGet).Inc()
			}
			return result, err
		},
	)
}

func (r *RequestServiceDefault) getRequest(ctx context.Context, id uint, withDeleted bool) (*models.Request, error) {
	ctx, span := core.TraceMethod(ctx, "RequestServiceDefault.getRequest")
	defer span.End()

	var req models.Request
	err := db.RetryableComponentTransaction(r, ctx, func(tx *gorm.DB) *gorm.DB {
		query := tx.WithContext(ctx)
		if withDeleted {
			query = query.Unscoped()
		}
		return query.First(&req, id)
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
	ctx, span := core.TraceMethod(ctx, "RequestServiceDefault.UpdateRequest")
	defer span.End()

	protocol := req.Protocol
	if protocol == "" {
		protocol = requestMetrics.LabelProtocolUnknown
	}
	operation := req.Operation
	if operation == "" {
		operation = requestMetrics.LabelOperationUnknown
	}

	return core.MetricTrack(
		requestMetrics.RequestDuration.WithLabelValues(operation),
		requestMetrics.RequestsFailed.WithLabelValues(protocol, operation),
		func() error {
			err := db.RetryableComponentTransaction(r, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.WithContext(ctx).Save(req)
			})

			if err != nil {
				return err
			}

			requestMetrics.RequestsUpdated.WithLabelValues(protocol, operation).Inc()

			return nil
		},
	)
}

func (r *RequestServiceDefault) DeleteRequest(ctx context.Context, id uint) error {
	ctx, span := core.TraceMethod(ctx, "RequestServiceDefault.DeleteRequest")
	defer span.End()

	req, err := r.GetRequest(ctx, id)
	if err != nil {
		return err
	}

	protocol := req.Protocol
	if protocol == "" {
		protocol = requestMetrics.LabelProtocolUnknown
	}
	operation := req.Operation
	if operation == "" {
		operation = requestMetrics.LabelOperationUnknown
	}

	_, handler, err := r.ops.FindOperationHandler(req.Operation)
	if err != nil {
		r.Logger().Warn("Could not find operation handler for cleanup",
			zap.Error(err), zap.String("operation", req.Operation))
	} else if handler != nil {
		if err := handler.Cleanup(ctx, req); err != nil {
			r.Logger().Warn("Cleanup failed but continuing with deletion",
				zap.Error(err), zap.Uint("requestID", req.ID))
		}
	}

	return core.MetricTrack(
		requestMetrics.RequestDuration.WithLabelValues(operation),
		requestMetrics.RequestsFailed.WithLabelValues(protocol, operation),
		func() error {
			err = db.RetryableComponentTransaction(r, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.WithContext(ctx).Delete(&models.Request{}, id)
			})

			if err != nil {
				return err
			}

			requestMetrics.RequestsDeleted.WithLabelValues(protocol, operation).Inc()
			requestMetrics.RequestsByStatus.WithLabelValues(string(req.Status)).Dec()
			requestMetrics.RequestsByOperation.WithLabelValues(operation).Dec()
			if protocol != requestMetrics.LabelProtocolUnknown {
				requestMetrics.RequestsByProtocol.WithLabelValues(protocol).Dec()
			}

			return nil
		},
	)
}

func (r *RequestServiceDefault) QueryRequest(ctx context.Context, query interface{}, filter core.RequestFilter) (*models.Request, error) {
	ctx, span := core.TraceMethod(ctx, "RequestServiceDefault.QueryRequest")
	defer span.End()

	var req models.Request

	result, err := core.MetricTrackResult(
		requestMetrics.RequestDuration.WithLabelValues(requestMetrics.LabelQueryTypeQuery),
		nil,
		func() (*models.Request, error) {
			err := db.RetryableComponentTransaction(r, ctx, func(tx *gorm.DB) *gorm.DB {
				tx = tx.WithContext(ctx)
				if query != nil {
					tx = tx.Where(query)
				}

				return tx.Scopes(
					applyFilters(filter),
				).First(&req)
			})
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil, fmt.Errorf("request not found: %w", err)
				}
				return nil, fmt.Errorf("query failed: %w", err)
			}
			requestMetrics.RequestsQueryTotal.WithLabelValues(requestMetrics.LabelQueryTypeQuery).Inc()
			return &req, nil
		},
	)
	return result, err
}

func (r *RequestServiceDefault) GetRequestByHash(ctx context.Context, hash core.StorageHash, filter core.RequestFilter) (*models.Request, error) {
	ctx, span := core.TraceMethod(ctx, "RequestServiceDefault.GetRequestByHash")
	defer span.End()

	var req models.Request
	req.Hash = hash.Multihash()

	result, err := core.MetricTrackResult(
		requestMetrics.RequestDuration.WithLabelValues(requestMetrics.LabelQueryTypeByHash),
		nil,
		func() (*models.Request, error) {
			err := db.RetryableComponentTransaction(r, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.WithContext(ctx).
					Scopes(
						applyFilters(filter),
					).
					Where(&req).First(&req)
			})
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil, fmt.Errorf("request with hash not found: %w", err)
				}
				return nil, fmt.Errorf("failed to get request: %w", err)
			}
			requestMetrics.RequestsQueryTotal.WithLabelValues(requestMetrics.LabelQueryTypeByHash).Inc()
			return &req, nil
		},
	)
	return result, err
}

func (r *RequestServiceDefault) ListRequestsByUser(ctx context.Context, userID uint, filter core.RequestFilter) ([]*models.Request, error) {
	ctx, span := core.TraceMethod(ctx, "RequestServiceDefault.ListRequestsByUser")
	defer span.End()

	var requests []*models.Request

	var req models.Request
	req.UserID = &userID

	result, err := core.MetricTrackResult(
		requestMetrics.RequestDuration.WithLabelValues(requestMetrics.LabelQueryTypeByUser),
		nil,
		func() ([]*models.Request, error) {
			err := db.RetryableComponentTransaction(r, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.WithContext(ctx).Where(&req).Scopes(
					applyFilters(filter),
				).Find(&requests)
			})
			if err != nil {
				return nil, fmt.Errorf("failed to list requests: %w", err)
			}
			requestMetrics.RequestsQueryTotal.WithLabelValues(requestMetrics.LabelQueryTypeByUser).Inc()
			return requests, nil
		},
	)
	return result, err
}

func (r *RequestServiceDefault) ListRequestsByStatus(ctx context.Context, status string, filter core.RequestFilter) ([]*models.Request, error) {
	ctx, span := core.TraceMethod(ctx, "RequestServiceDefault.ListRequestsByStatus")
	defer span.End()

	var requests []*models.Request

	result, err := core.MetricTrackResult(
		requestMetrics.RequestDuration.WithLabelValues(requestMetrics.LabelQueryTypeByStatus),
		nil,
		func() ([]*models.Request, error) {
			err := db.RetryableComponentTransaction(r, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.WithContext(ctx).Where("status = ?", status).
					Scopes(
						applyFilters(filter),
					).Find(&requests)
			})
			if err != nil {
				return nil, fmt.Errorf("failed to list requests: %w", err)
			}
			requestMetrics.RequestsQueryTotal.WithLabelValues(requestMetrics.LabelQueryTypeByStatus).Inc()
			return requests, nil
		},
	)
	return result, err
}

func (r *RequestServiceDefault) UpdateRequestStatus(ctx context.Context, id uint, status models.RequestStatusType, message string) error {
	ctx, span := core.TraceMethod(ctx, "RequestServiceDefault.UpdateRequestStatus")
	defer span.End()

	return db.RetryableComponentTransaction(r, ctx, func(tx *gorm.DB) *gorm.DB {
		return tx.Model(&models.Request{}).
			Where("id = ?", id).
			Updates(map[string]interface{}{
				"status":         status,
				"status_message": message,
				"updated_at":     time.Now(),
			})
	})
}

func (r *RequestServiceDefault) CompleteRequest(ctx context.Context, id uint) error {
	ctx, span := core.TraceMethod(ctx, "RequestServiceDefault.CompleteRequest")
	defer span.End()

	req, err := r.GetRequest(ctx, id)
	if err != nil {
		return err
	}

	// Don't complete if already completed or failed
	if req.Status == models.RequestStatusCompleted || req.Status == models.RequestStatusFailed {
		r.Logger().Debug("CompleteRequest skipped: request already terminal",
			zap.Uint("requestID", id),
			zap.String("currentStatus", string(req.Status)))
		return nil
	}

	protocol := req.Protocol
	if protocol == "" {
		protocol = requestMetrics.LabelProtocolUnknown
	}
	operation := req.Operation
	if operation == "" {
		operation = requestMetrics.LabelOperationUnknown
	}

	return core.MetricTrack(
		requestMetrics.RequestDuration.WithLabelValues(operation),
		requestMetrics.RequestsFailed.WithLabelValues(protocol, operation),
		func() error {
			// Get the request status to use its message
			status, err := r.ComputeRequestStatus(ctx, id, false)
			if err != nil {
				return err
			}

			message := "Request completed successfully"
			if status.Message != "" {
				message = status.Message
			}

			err = r.UpdateRequestStatus(ctx, id, models.RequestStatusCompleted, message)
			if err != nil {
				return err
			}

			status, err = r.ComputeRequestStatus(ctx, id, false)
			if err != nil {
				return err
			}

			if status.Message != "" {
				message = status.Message
			}

			if err = r.UpdateRequestStatus(ctx, id, models.RequestStatusCompleted, message); err != nil {
				return err
			}

			requestMetrics.RequestsCompleted.WithLabelValues(protocol, operation).Inc()
			requestMetrics.RequestsByStatus.WithLabelValues(string(models.RequestStatusCompleted)).Inc()
			requestMetrics.RequestsByStatus.WithLabelValues(string(req.Status)).Dec()
			requestMetrics.RequestsByOperation.WithLabelValues(operation).Inc()
			if protocol != requestMetrics.LabelProtocolUnknown {
				requestMetrics.RequestsByProtocol.WithLabelValues(protocol).Inc()
			}

			r.Logger().Debug("request completed successfully",
				zap.Uint("requestID", id),
				zap.String("protocol", protocol),
				zap.String("operation", operation))

			return nil
		},
	)
}

func (r *RequestServiceDefault) FailRequest(ctx context.Context, id uint, reason string) error {
	ctx, span := core.TraceMethod(ctx, "RequestServiceDefault.FailRequest")
	defer span.End()

	req, err := r.GetRequest(ctx, id)
	if err != nil {
		return err
	}

	// Don't fail if already completed or failed
	if req.Status == models.RequestStatusCompleted || req.Status == models.RequestStatusFailed {
		return nil
	}

	protocol := req.Protocol
	if protocol == "" {
		protocol = requestMetrics.LabelProtocolUnknown
	}
	operation := req.Operation
	if operation == "" {
		operation = requestMetrics.LabelOperationUnknown
	}

	return core.MetricTrack(
		requestMetrics.RequestDuration.WithLabelValues(operation),
		requestMetrics.RequestsFailed.WithLabelValues(protocol, operation),
		func() error {
			err = db.RetryableComponentTransaction(r, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.Model(&models.Request{}).
					Where("id = ?", id).
					Updates(map[string]interface{}{
						"status":         models.RequestStatusFailed,
						"status_message": reason,
					})
			})

			if err != nil {
				return err
			}

			requestMetrics.RequestsFailed.WithLabelValues(protocol, operation).Inc()
			requestMetrics.RequestsByStatus.WithLabelValues(string(models.RequestStatusFailed)).Inc()
			requestMetrics.RequestsByStatus.WithLabelValues(string(req.Status)).Dec()
			requestMetrics.RequestsByOperation.WithLabelValues(operation).Inc()
			if protocol != requestMetrics.LabelProtocolUnknown {
				requestMetrics.RequestsByProtocol.WithLabelValues(protocol).Inc()
			}

			return nil
		},
	)
}

func (r *RequestServiceDefault) ComputeRequestStatus(ctx context.Context, id uint, keepExisting bool) (*core.RequestStatus, error) {
	ctx, span := core.TraceMethod(ctx, "RequestServiceDefault.ComputeRequestStatus")
	defer span.End()

	return r.computeRequestStatus(ctx, id, keepExisting, false)
}

func (r *RequestServiceDefault) ComputeRequestStatusWithDeleted(ctx context.Context, id uint, keepExisting bool) (*core.RequestStatus, error) {
	ctx, span := core.TraceMethod(ctx, "RequestServiceDefault.ComputeRequestStatusWithDeleted")
	defer span.End()

	return r.computeRequestStatus(ctx, id, keepExisting, true)
}

func (r *RequestServiceDefault) computeRequestStatus(ctx context.Context, id uint, keepExisting bool, withDeleted bool) (*core.RequestStatus, error) {
	ctx, span := core.TraceMethod(ctx, "RequestServiceDefault.computeRequestStatus")
	defer span.End()

	var req *models.Request
	var err error

	if withDeleted {
		req, err = r.GetRequestWithDeleted(ctx, id)
	} else {
		req, err = r.GetRequest(ctx, id)
	}
	if err != nil {
		return nil, err
	}

	// Find operation handler
	_, handler, err := r.ops.FindOperationHandler(req.Operation)
	if err != nil {
		return nil, err
	}

	// Get status from handler if available
	status := &core.RequestStatus{
		UpdatedAt: req.UpdatedAt,
	}

	if keepExisting {
		status.State = req.Status
	}

	if handler != nil {
		detailedStatus, err := handler.GetStatus(ctx, req)
		if err != nil {
			r.Logger().Warn("Failed to get detailed status from handler",
				zap.Error(err), zap.Uint("requestID", req.ID))
		} else if detailedStatus != nil {
			if detailedStatus.State != "" {
				status.State = detailedStatus.State
			}
			if detailedStatus.Message != "" {
				status.Message = detailedStatus.Message
			}
			if !detailedStatus.UpdatedAt.IsZero() {
				status.UpdatedAt = detailedStatus.UpdatedAt
			}
			if detailedStatus.ProgressPercent > 0 {
				status.ProgressPercent = detailedStatus.ProgressPercent
			}
			if detailedStatus.Error != nil {
				status.Error = detailedStatus.Error
			}
		}
	}

	// Set default message based on status if not provided
	if status.Message == "" {
		status.Message = core.GetDefaultStatusMessage(req.Status)
		if req.Status == models.RequestStatusFailed && req.StatusMessage != "" {
			status.Message = req.StatusMessage
		}
	}

	return status, nil
}

func (r *RequestServiceDefault) ValidateRequest(ctx context.Context, req *models.Request) error {
	ctx, span := core.TraceMethod(ctx, "RequestServiceDefault.ValidateRequest")
	defer span.End()

	// Find the operation handler
	_, handler, err := r.ops.FindOperationHandler(req.Operation)
	if err != nil {
		return fmt.Errorf("failed to find handler for operation %s: %w", req.Operation, err)
	}

	// Validate if handler exists
	if handler != nil {
		return handler.ValidateRequest(ctx, req)
	}

	return nil
}

func (r *RequestServiceDefault) GetRequestStatus(ctx context.Context, id uint, withDeleted bool) (*core.RequestStatus, error) {
	ctx, span := core.TraceMethod(ctx, "RequestServiceDefault.GetRequestStatus")
	defer span.End()

	var req *models.Request
	var err error

	if withDeleted {
		req, err = r.GetRequestWithDeleted(ctx, id)
	} else {
		req, err = r.GetRequest(ctx, id)
	}
	if err != nil {
		return nil, err
	}

	requestMetrics.RequestsQueryTotal.WithLabelValues(requestMetrics.LabelQueryTypeGetStatus).Inc()

	status := &core.RequestStatus{
		State:     req.Status,
		UpdatedAt: req.UpdatedAt,
	}

	// Get progress percentage and detailed status from handler
	var computedStatus *core.RequestStatus
	if withDeleted {
		computedStatus, err = r.ComputeRequestStatusWithDeleted(ctx, id, true)
	} else {
		computedStatus, err = r.ComputeRequestStatus(ctx, id, true)
	}
	if err == nil {
		// Normalize to 2 decimal places to ensure consistent JSON output for UI clients
		status.ProgressPercent = math.Round(computedStatus.ProgressPercent*100) / 100
		// Use state from handler if provided (may be different from req.Status due to progress tracking)
		// Only apply handler-provided state if it's non-empty to preserve terminal states
		if computedStatus.State != "" {
			status.State = computedStatus.State
		}
		// Use message from handler if provided, otherwise fall back to request status message
		if computedStatus.Message != "" {
			status.Message = computedStatus.Message
		}
	}

	// Set default message based on status if not yet set
	if status.Message == "" {
		if req.StatusMessage == "" {
			status.Message = core.GetDefaultStatusMessage(req.Status)
		} else {
			status.Message = req.StatusMessage
		}
	}

	return status, nil
}

func (r *RequestServiceDefault) RequestExists(ctx context.Context, id uint) (bool, error) {
	ctx, span := core.TraceMethod(ctx, "RequestServiceDefault.RequestExists")
	defer span.End()

	var exists bool
	err := db.RetryableComponentTransaction(r, ctx, func(tx *gorm.DB) *gorm.DB {
		return tx.WithContext(ctx).
			Model(&models.Request{}).
			Select("count(*) > 0").
			Where("id = ?", id).
			Find(&exists)
	})
	return exists, err
}

func (r *RequestServiceDefault) GetRequestData(ctx context.Context, req *models.Request) (interface{}, error) {
	ctx, span := core.TraceMethod(ctx, "RequestServiceDefault.GetRequestData")
	defer span.End()

	// Get model for this operation
	model, err := r.CreateRequestModel(req.Operation)
	if err != nil {
		return nil, err
	}
	// Query the database

	if err = db.RetryableComponentTransaction(r, ctx, func(tx *gorm.DB) *gorm.DB {
		return tx.Where("request_id = ?", req.ID).First(model)
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // No data found, but not an error
		}
		return nil, err
	}

	return model, nil
}

func (r *RequestServiceDefault) UpdateRequestData(ctx context.Context, req *models.Request, data interface{}) error {
	ctx, span := core.TraceMethod(ctx, "RequestServiceDefault.UpdateRequestData")
	defer span.End()

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
	model.SetRequest(req)

	// Validate
	if err = model.Validate(); err != nil {
		return fmt.Errorf("data validation failed: %w", err)
	}

	return db.RetryableComponentTransaction(r, ctx, func(tx *gorm.DB) *gorm.DB {
		return tx.Where("request_id = ?", req.ID).Save(model)
	})
}

func (r *RequestServiceDefault) QueryRequestData(ctx context.Context, query any, filter core.RequestFilter) (*models.Request, error) {
	ctx, span := core.TraceMethod(ctx, "RequestServiceDefault.QueryRequestData")
	defer span.End()

	var req models.Request

	// Get model for the operation if specified
	var model data_models.RequestDataModel
	var err error
	if filter.Operation != nil && *filter.Operation != "" {
		model, err = r.CreateRequestModel(*filter.Operation)
		if err != nil {
			return nil, fmt.Errorf("failed to create data model: %w", err)
		}
	}

	err = db.RetryableComponentTransaction(r, ctx, func(tx *gorm.DB) *gorm.DB {
		tx = tx.Model(&req)

		// Join with data model if operation was specified
		if model != nil {
			tx = tx.Joins(
				"INNER JOIN ? as data ON data.request_id = requests.id",
				gorm.Expr(model.TableName()),
			)
		}

		if query != nil {
			queryMap := requestModelToMap(query)
			for column, value := range queryMap {
				tx = tx.Where("data."+column+" = ?", value)
			}
		}

		return tx.Scopes(
			applyFilters(filter),
		).First(&req)
	})

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("request not found: %w", err)
		}
		return nil, fmt.Errorf("query failed: %w", err)
	}
	return &req, nil
}

// Helper functions
func applyFilters(filter core.RequestFilter) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if filter.UserID != nil {
			db = db.Where("user_id = ?", *filter.UserID)
		}
		if filter.Status != nil {
			db = db.Where("status = ?", *filter.Status)
		}
		if filter.Protocol != nil {
			db = db.Where("protocol = ?", *filter.Protocol)
		}
		if filter.Operation != nil {
			db = db.Where("operation = ?", *filter.Operation)
		}
		if filter.SourceIP != nil {
			db = db.Where("source_ip = ?", *filter.SourceIP)
		}
		if len(filter.Hash) > 0 {
			db = db.Where("hash = ?", filter.Hash)
		}
		if filter.CIDType != nil {
			db = db.Where("cid_type = ?", *filter.CIDType)
		}
		if filter.CreatedAfter != nil {
			db = db.Where("created_at > ?", *filter.CreatedAfter)
		}
		if filter.UpdatedAfter != nil {
			db = db.Where("updated_at > ?", *filter.UpdatedAfter)
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

func requestModelToMap(v any) map[string]any {
	s := structs.New(v)

	out := make(map[string]any)

	for _, field := range s.Fields() {
		if !field.IsExported() || field.IsEmbedded() {
			continue
		}

		if field.Kind() == reflect.Interface || field.Kind() == reflect.Map || field.Kind() == reflect.Pointer {
			continue
		}

		if field.Kind() == reflect.Struct {
			gfield, exists := field.FieldOk("Model")
			if exists && reflect.TypeOf(gfield.Value()) == reflect.TypeOf(gorm.Model{}) {
				continue
			}
		}

		if field.Tag("gorm") == "" {
			continue
		}

		if field.IsZero() {
			continue
		}

		out[field.Tag("gorm")] = field.Value()
	}

	return out
}
