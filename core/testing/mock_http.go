package testing

import (
	"github.com/stretchr/testify/mock"
	router "go.lumeweb.com/portal-router"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/testing/mocks"
	"testing"
)

// MockHTTPService implements core.HTTPService for testing with default Router expectations
type MockHTTPService struct {
	*mocks.MockHTTPService
	router router.Router
}

// NewMockHTTPService creates a new mock HTTP service with default Router expectations
func NewMockHTTPService(t mock.TestingT) *MockHTTPService {
	mockHTTPService := mocks.NewMockHTTPService(t)
	httpService := &MockHTTPService{
		MockHTTPService: mockHTTPService,
	}

	// Set up default expectation for Router
	mockHTTPService.On("Router").
		Maybe().
		Return(func() router.Router {
			return httpService.router
		})

	return httpService
}

// WithRouter sets the router for the mock HTTP service
func (m *MockHTTPService) WithRouter(r router.Router) *MockHTTPService {
	m.router = r
	return m
}

// Ensure MockHTTPService implements core.HTTPService
var _ core.HTTPService = (*MockHTTPService)(nil)
