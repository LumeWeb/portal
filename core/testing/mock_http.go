package testing

import (
	"fmt"
	router "go.lumeweb.com/portal-router"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/testing/mocks"
)

// MockHTTPService implements core.HTTPService for testing with default expectations
type MockHTTPService struct {
	*mocks.MockHTTPService
	router   router.Router
	cmanager config.Manager
}

// NewMockHTTPService creates a new mock HTTP service with default expectations
func NewMockHTTPService(t TB) *MockHTTPService {
	mockHTTPService := mocks.NewMockHTTPService(t)
	httpService := &MockHTTPService{
		MockHTTPService: mockHTTPService,
	}

	// Set up default expectations
	mockHTTPService.On("Router").
		Maybe().
		Return(func() router.Router {
			return httpService.router
		})

	mockHTTPService.On("Serve").
		Maybe().
		Return(nil)

	mockHTTPService.On("Init").
		Maybe().
		Return(nil)

	mockHTTPService.On("APISubdomain", mock.AnythingOfType("string"), mock.AnythingOfType("bool")).
		Maybe().
		Return(httpService.apiSubdomainFunc("", false))

	return httpService
}

// APISubdomain implements core.HTTPService with automatic mock setup
func (m *MockHTTPService) APISubdomain(id string, proto bool) string {
	// Set up expectation if not already set
	if !m.MethodCalled("APISubdomain", id, proto) {
		m.On("APISubdomain", id, proto).Return(m.apiSubdomainFunc(id, proto))
	}
	
	return m.MockHTTPService.APISubdomain(id, proto)
}

// apiSubdomainFunc generates the return function for APISubdomain expectations
func (m *MockHTTPService) apiSubdomainFunc(id string, proto bool) func(string, bool) string {
	return func(id string, proto bool) string {
		if core.GetAPI(id) == nil {
			return ""
		}
		return fmt.Sprintf("%s.%s", core.GetAPI(id).Subdomain(), m.cmanager.Config().Core.Domain)
	}
}

// Serve implements core.HTTPService with automatic mock setup
func (m *MockHTTPService) Serve() error {
	// Set up expectation if not already set
	if !m.MethodCalled("Serve") {
		m.On("Serve").Return(nil)
	}
	
	return m.MockHTTPService.Serve()
}

// Init implements core.HTTPService with automatic mock setup
func (m *MockHTTPService) Init() error {
	// Set up expectation if not already set
	if !m.MethodCalled("Init") {
		m.On("Init").Return(nil)
	}
	
	return m.MockHTTPService.Init()
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
