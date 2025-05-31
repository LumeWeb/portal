package testing

import (
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/testing/mocks"
	"github.com/stretchr/testify/mock"
)

// MockAccessService implements core.AccessService for testing with default RegisterRoute expectations
type MockAccessService struct {
	*mocks.MockAccessService
}

// NewMockAccessService creates a new mock access service with default RegisterRoute expectations
func NewMockAccessService(t TB) *MockAccessService {
	mockAccess := mocks.NewMockAccessService(t)
	access := &MockAccessService{
		MockAccessService: mockAccess,
	}

	// Set up default expectation for RegisterRoute
	// This allows router registration to proceed without explicit mocks in every test
	mockAccess.On("RegisterRoute", mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string")).
		Maybe().
		Return(nil)

	return access
}

// Ensure MockAccessService implements core.AccessService
var _ core.AccessService = (*MockAccessService)(nil)
