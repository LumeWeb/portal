package testing

import (
	"context"
	"errors"
	"go.lumeweb.com/portal/db/models/data_models"
	"gorm.io/gorm"
	"testing"
)

import (
	"go.lumeweb.com/portal/db/mocks"
)

// MockProtocolPinHandler implements core.ProtocolPinHandler for testing
type MockProtocolPinHandler struct {
	createProtocolPinFunc   func(ctx context.Context, id uint, data any) error
	getProtocolPinFunc      func(ctx context.Context, tx *gorm.DB, id uint) (any, error)
	updateProtocolPinFunc   func(ctx context.Context, id uint, data any) error
	deleteProtocolPinFunc   func(ctx context.Context, id uint) error
	queryProtocolPinFunc    func(ctx context.Context, query any) *gorm.DB
	getProtocolPinModelFunc func() data_models.PinDataModel
	defaultBehavior         bool
	pinModel                *mocks.MockPinDataModel
}

// NewMockProtocolPinHandler creates a new mock pin handler with default behavior
// and automatically sets up a mock pin model
func NewMockProtocolPinHandler(t testing.TB) *MockProtocolPinHandler {
	mockModel := mocks.NewMockPinDataModel(t) // Pass through the testing.TB parameter to create the mock
	return &MockProtocolPinHandler{
		defaultBehavior: true,
		pinModel:        mockModel,
		getProtocolPinModelFunc: func() data_models.PinDataModel {
			return mockModel
		},
	}
}

func (h *MockProtocolPinHandler) CreateProtocolPin(ctx context.Context, id uint, data any) error {
	if h.createProtocolPinFunc != nil {
		return h.createProtocolPinFunc(ctx, id, data)
	}
	if h.defaultBehavior {
		return nil
	}
	return errors.New("CreateProtocolPin not implemented")
}

func (h *MockProtocolPinHandler) GetProtocolPin(ctx context.Context, tx *gorm.DB, id uint) (any, error) {
	if h.getProtocolPinFunc != nil {
		return h.getProtocolPinFunc(ctx, tx, id)
	}
	if h.defaultBehavior {
		return nil, nil
	}
	return nil, errors.New("GetProtocolPin not implemented")
}

func (h *MockProtocolPinHandler) UpdateProtocolPin(ctx context.Context, id uint, data any) error {
	if h.updateProtocolPinFunc != nil {
		return h.updateProtocolPinFunc(ctx, id, data)
	}
	if h.defaultBehavior {
		return nil
	}
	return errors.New("UpdateProtocolPin not implemented")
}

func (h *MockProtocolPinHandler) DeleteProtocolPin(ctx context.Context, id uint) error {
	if h.deleteProtocolPinFunc != nil {
		return h.deleteProtocolPinFunc(ctx, id)
	}
	if h.defaultBehavior {
		return nil
	}
	return errors.New("DeleteProtocolPin not implemented")
}

func (h *MockProtocolPinHandler) QueryProtocolPin(ctx context.Context, query any) *gorm.DB {
	if h.queryProtocolPinFunc != nil {
		return h.queryProtocolPinFunc(ctx, query)
	}
	if h.defaultBehavior {
		return nil
	}
	return nil
}

func (h *MockProtocolPinHandler) GetProtocolPinModel() data_models.PinDataModel {
	if h.getProtocolPinModelFunc != nil {
		return h.getProtocolPinModelFunc()
	}
	if h.defaultBehavior {
		return nil
	}
	return nil
}

// WithCreateProtocolPin sets a custom CreateProtocolPin function
func (h *MockProtocolPinHandler) WithCreateProtocolPin(f func(ctx context.Context, id uint, data any) error) *MockProtocolPinHandler {
	h.createProtocolPinFunc = f
	h.defaultBehavior = false
	return h
}

// WithGetProtocolPin sets a custom GetProtocolPin function
func (h *MockProtocolPinHandler) WithGetProtocolPin(f func(ctx context.Context, tx *gorm.DB, id uint) (any, error)) *MockProtocolPinHandler {
	h.getProtocolPinFunc = f
	h.defaultBehavior = false
	return h
}

// WithUpdateProtocolPin sets a custom UpdateProtocolPin function
func (h *MockProtocolPinHandler) WithUpdateProtocolPin(f func(ctx context.Context, id uint, data any) error) *MockProtocolPinHandler {
	h.updateProtocolPinFunc = f
	h.defaultBehavior = false
	return h
}

// WithDeleteProtocolPin sets a custom DeleteProtocolPin function
func (h *MockProtocolPinHandler) WithDeleteProtocolPin(f func(ctx context.Context, id uint) error) *MockProtocolPinHandler {
	h.deleteProtocolPinFunc = f
	h.defaultBehavior = false
	return h
}

// WithQueryProtocolPin sets a custom QueryProtocolPin function
func (h *MockProtocolPinHandler) WithQueryProtocolPin(f func(ctx context.Context, query any) *gorm.DB) *MockProtocolPinHandler {
	h.queryProtocolPinFunc = f
	h.defaultBehavior = false
	return h
}

// WithGetProtocolPinModel sets a custom GetProtocolPinModel function
func (h *MockProtocolPinHandler) WithGetProtocolPinModel(f func() data_models.PinDataModel) *MockProtocolPinHandler {
	h.getProtocolPinModelFunc = f
	h.defaultBehavior = false
	return h
}

// WithDefaultBehavior enables/disables default behavior
func (h *MockProtocolPinHandler) WithDefaultBehavior(enabled bool) *MockProtocolPinHandler {
	h.defaultBehavior = enabled
	return h
}
