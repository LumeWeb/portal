package testing

import (
	"fmt"

	"github.com/stretchr/testify/mock"
	router "go.lumeweb.com/portal-router"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/testing/mocks"
	"gorm.io/gorm"
)

// MockHTTPService implements core.HTTPService for testing with default expectations
type MockHTTPService struct {
	*mocks.MockHTTPService
	router            router.Router
	cmanager          config.Manager
	componentContext  core.Context
	componentLogger   *core.Logger
	componentDB       *gorm.DB
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

	mockHTTPService.On("RegisterGlobalPath", mock.AnythingOfType("string")).
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
	if !WasMethodCalled(&m.MockHTTPService.Mock, "APISubdomain", id, proto) {
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
	if !WasMethodCalled(&m.MockHTTPService.Mock, "Serve") {
		m.On("Serve").Return(nil)
	}

	return m.MockHTTPService.Serve()
}

// Init implements core.HTTPService with automatic mock setup
func (m *MockHTTPService) Init() error {
	// Set up expectation if not already set
	if !WasMethodCalled(&m.MockHTTPService.Mock, "Init") {
		m.On("Init").Return(nil)
	}

	return m.MockHTTPService.Init()
}

// RegisterGlobalPath implements core.HTTPService with automatic mock setup
func (m *MockHTTPService) RegisterGlobalPath(path string) error {
	// Set up expectation if not already set
	if !WasMethodCalled(&m.MockHTTPService.Mock, "RegisterGlobalPath", path) {
		m.On("RegisterGlobalPath", path).Return(nil)
	}

	return m.MockHTTPService.RegisterGlobalPath(path)
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

// Config implements core.Component
func (m *MockHTTPService) Config() config.Manager {
	return m.cmanager
}

// SetConfig implements core.Component
func (m *MockHTTPService) SetConfig(cfg config.Manager) {
	m.cmanager = cfg
}

// Context implements core.Component
func (m *MockHTTPService) Context() core.Context {
	return m.componentContext
}

// SetContext implements core.Component
func (m *MockHTTPService) SetContext(ctx core.Context) {
	m.componentContext = ctx
}

// Logger implements core.Component
func (m *MockHTTPService) Logger() *core.Logger {
	return m.componentLogger
}

// SetLogger implements core.Component
func (m *MockHTTPService) SetLogger(logger *core.Logger) {
	m.componentLogger = logger
}

// DB implements core.Component
func (m *MockHTTPService) DB() *gorm.DB {
	return m.componentDB
}

// SetDB implements core.Component
func (m *MockHTTPService) SetDB(db *gorm.DB) {
	m.componentDB = db
}

// Ensure MockHTTPService implements core.HTTPService
var _ core.HTTPService = (*MockHTTPService)(nil)
