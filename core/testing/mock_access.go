package testing

import (
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/testing/mocks"
	"github.com/stretchr/testify/mock"
)

// MockAccessService implements core.AccessService for testing with default expectations
type MockAccessService struct {
	*mocks.MockAccessService
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
func (m *MockAccessService) CheckAccess(userId uint, fqdn, path, method string) (bool, error) {
	// Set up expectation if not already set
	if !WasMethodCalled(&m.MockAccessService.Mock, "CheckAccess", userId, fqdn, path, method) {
		m.On("CheckAccess", userId, fqdn, path, method).Return(true, nil)
	}
	
	return m.MockAccessService.CheckAccess(userId, fqdn, path, method)
}

// AssignRoleToUser implements core.AccessService with automatic mock setup
func (m *MockAccessService) AssignRoleToUser(userId uint, role string) error {
	// Set up expectation if not already set
	if !WasMethodCalled(&m.MockAccessService.Mock, "AssignRoleToUser", userId, role) {
		m.On("AssignRoleToUser", userId, role).Return(nil)
	}
	
	return m.MockAccessService.AssignRoleToUser(userId, role)
}

// Ensure MockAccessService implements core.AccessService
var _ core.AccessService = (*MockAccessService)(nil)
