package testing

import (
	"context"

	"github.com/stretchr/testify/mock"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/testing/mocks"
	"gorm.io/gorm"
)

// MockAccessService implements core.AccessService for testing with default expectations
type MockAccessService struct {
	*mocks.MockAccessService
	componentConfig  config.Manager
	componentContext core.Context
	componentLogger  *core.Logger
	componentDB      *gorm.DB
}

// NewMockAccessService creates a new mock access service with default expectations
func NewMockAccessService(t TB) *MockAccessService {
	mockAccess := mocks.NewMockAccessService(t)
	access := &MockAccessService{
		MockAccessService: mockAccess,
	}

	// Set up default expectations
	mockAccess.On("RegisterRoute", mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string")).
		Maybe().
		Return(nil)

	mockAccess.On("AssignRoleToUser", mock.AnythingOfType("uint"), mock.AnythingOfType("string")).
		Maybe().
		Return(nil)

	mockAccess.On("CheckAccess", mock.AnythingOfType("uint"), mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string")).
		Maybe().
		Return(true, nil)

	return access
}

// CheckAccess implements core.AccessService with automatic mock setup
func (m *MockAccessService) CheckAccess(ctx context.Context, userId uint, fqdn, path, method string) (bool, error) {
	// Set up expectation if not already set
	if !WasMethodCalled(&m.MockAccessService.Mock, "CheckAccess", userId, fqdn, path, method) {
		m.On("CheckAccess", userId, fqdn, path, method).Return(true, nil)
	}

	return m.MockAccessService.CheckAccess(nil, userId, fqdn, path, method)
}

// AssignRoleToUser implements core.AccessService with automatic mock setup
func (m *MockAccessService) AssignRoleToUser(ctx context.Context, userId uint, role string) error {
	// Set up expectation if not already set
	if !WasMethodCalled(&m.MockAccessService.Mock, "AssignRoleToUser", userId, role) {
		m.On("AssignRoleToUser", userId, role).Return(nil)
	}

	return m.MockAccessService.AssignRoleToUser(nil, userId, role)
}

// Config implements core.Component
func (m *MockAccessService) Config() config.Manager {
	return m.componentConfig
}

// SetConfig implements core.Component
func (m *MockAccessService) SetConfig(cfg config.Manager) {
	m.componentConfig = cfg
}

// Context implements core.Component
func (m *MockAccessService) Context() core.Context {
	return m.componentContext
}

// SetContext implements core.Component
func (m *MockAccessService) SetContext(ctx core.Context) {
	m.componentContext = ctx
}

// Logger implements core.Component
func (m *MockAccessService) Logger() *core.Logger {
	return m.componentLogger
}

// SetLogger implements core.Component
func (m *MockAccessService) SetLogger(logger *core.Logger) {
	m.componentLogger = logger
}

// DB implements core.Component
func (m *MockAccessService) DB() *gorm.DB {
	return m.componentDB
}

// SetDB implements core.Component
func (m *MockAccessService) SetDB(db *gorm.DB) {
	m.componentDB = db
}

// Ensure MockAccessService implements core.AccessService
var _ core.AccessService = (*MockAccessService)(nil)
