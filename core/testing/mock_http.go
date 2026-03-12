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
	mockHTTPService.EXPECT().Router().
		Maybe().
		Return(func() router.Router {
			return httpService.router
		})

	mockHTTPService.EXPECT().Serve().
		Maybe().
		Return(nil)

	mockHTTPService.EXPECT().Init().
		Maybe().
		Return(nil)

	mockHTTPService.EXPECT().RegisterGlobalPath(mock.AnythingOfType("string")).
		Maybe().
		Return(nil)

	mockHTTPService.EXPECT().APISubdomain(mock.AnythingOfType("string"), mock.AnythingOfType("bool")).
		Maybe().
		Return(httpService.apiSubdomainFunc("", false))

	return httpService
}

// Router implements core.HTTPService with automatic mock setup.
// If no expectation was set by the test, adds a safe default expectation.
func (m *MockHTTPService) Router() router.Router {
	// Check if test already set up an expectation for Router
	// If not, add a safe default expectation
	if !HasExpectationForMethod(&m.MockHTTPService.Mock, "Router") {
		m.EXPECT().Router().Return(m.router).Maybe()
	}

	// Delegate to the underlying mock implementation
	return m.MockHTTPService.Router()
}

// APISubdomain implements core.HTTPService with automatic mock setup.
// If no expectation was set by the test, adds a safe default expectation.
func (m *MockHTTPService) APISubdomain(id string, proto bool) string {
	// Check if test already set up an expectation for APISubdomain
	// If not, add a safe default expectation
	if !HasExpectationForMethod(&m.MockHTTPService.Mock, "APISubdomain") {
		m.EXPECT().APISubdomain(mock.AnythingOfType("string"), mock.AnythingOfType("bool")).Return("").Maybe()
	}

	// Delegate to the underlying mock implementation
	return m.MockHTTPService.APISubdomain(id, proto)
}

// apiSubdomainFunc generates the return function for APISubdomain expectations
func (m *MockHTTPService) apiSubdomainFunc(id string, proto bool) func(string, bool) string {
	return func(id string, proto bool) string {
		if m.cmanager == nil {
			return ""
		}
		domain := m.cmanager.Config().Core.Domain
		if core.GetAPI(id) == nil {
			return ""
		}
		subdomain := core.GetAPI(id).Subdomain()
		if subdomain == "" {
			return domain
		}
		return fmt.Sprintf("%s.%s", subdomain, domain)
	}
}

// Serve implements core.HTTPService with automatic mock setup.
// If no expectation was set by the test, adds a safe default expectation.
func (m *MockHTTPService) Serve() error {
	// Check if test already set up an expectation for Serve
	// If not, add a safe default expectation
	if !HasExpectationForMethod(&m.MockHTTPService.Mock, "Serve") {
		m.EXPECT().Serve().Return(nil).Maybe()
	}

	// Delegate to the underlying mock implementation
	return m.MockHTTPService.Serve()
}

// Init implements core.HTTPService with automatic mock setup.
// If no expectation was set by the test, adds a safe default expectation.
func (m *MockHTTPService) Init() error {
	// Check if test already set up an expectation for Init
	// If not, add a safe default expectation
	if !HasExpectationForMethod(&m.MockHTTPService.Mock, "Init") {
		m.EXPECT().Init().Return(nil).Maybe()
	}

	// Delegate to the underlying mock implementation
	return m.MockHTTPService.Init()
}

// RegisterGlobalPath implements core.HTTPService with automatic mock setup.
// If no expectation was set by the test, adds a safe default expectation.
func (m *MockHTTPService) RegisterGlobalPath(path string) error {
	// Check if test already set up an expectation for RegisterGlobalPath
	// If not, add a safe default expectation
	if !HasExpectationForMethod(&m.MockHTTPService.Mock, "RegisterGlobalPath") {
		m.EXPECT().RegisterGlobalPath(mock.AnythingOfType("string")).Return(nil).Maybe()
	}

	// Delegate to the underlying mock implementation
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
