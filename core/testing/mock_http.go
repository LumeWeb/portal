package testing

import (
	"fmt"
	router "go.lumeweb.com/portal-router"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/testing/mocks"
)

// MockHTTPService implements core.HTTPService for testing with default Router expectations
type MockHTTPService struct {
	*mocks.MockHTTPService
	router   router.Router
	cmanager config.Manager
}

// NewMockHTTPService creates a new mock HTTP service with default Router expectations
func NewMockHTTPService(t TB) *MockHTTPService {
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

func (m *MockHTTPService) WithConfigManager(c config.Manager) *MockHTTPService {
	m.cmanager = c
	return m
}

func (m *MockHTTPService) APISubdomain(id string, proto bool) string {
	formatter := ""

	if proto {
		formatter += "https://"
	}

	formatter += "%s.%s"

	if core.GetAPI(id) == nil {
		return ""
	}

	return fmt.Sprintf(formatter, core.GetAPI(id).Subdomain(), m.cmanager.Config().Core.Domain)
}

// Ensure MockHTTPService implements core.HTTPService
var _ core.HTTPService = (*MockHTTPService)(nil)
