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

	// Set up default expectations using EXPECT() with Maybe() for optional calls
	mockAccess.EXPECT().RegisterRoute(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil).
		Maybe()

	mockAccess.EXPECT().AssignRoleToUser(mock.Anything, mock.Anything, mock.Anything).
		Return(nil).
		Maybe()

	mockAccess.EXPECT().CheckAccess(mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(true, nil).
		Maybe()

	return access
}

// CheckAccess implements core.AccessService with automatic mock setup
func (m *MockAccessService) CheckAccess(ctx context.Context, userId uint, fqdn, path, method string) (bool, error) {
	return m.MockAccessService.CheckAccess(ctx, userId, fqdn, path, method)
}

// AssignRoleToUser implements core.AccessService with automatic mock setup
func (m *MockAccessService) AssignRoleToUser(ctx context.Context, userId uint, role string) error {
	return m.MockAccessService.AssignRoleToUser(ctx, userId, role)
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
