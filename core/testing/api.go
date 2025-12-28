package testing

import (
	"testing"

	"github.com/stretchr/testify/mock"
	router "go.lumeweb.com/portal-router"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/testing/mocks"
	"gorm.io/gorm"
)

// MockAPI implements core.API for testing
type MockAPI struct {
	*mocks.MockAPI
	nameValue          string
	configValue        config.APIConfig
	subdomainValue     string
	authTokenNameValue string
	openAPIInfoValue   router.APIInfoDefinition
	configureFunc      func(router.Router, core.AccessService) error
	componentConfig    config.Manager // For Component.GetConfig() method
}

// Name implements core.API
func (m *MockAPI) Name() string {
	return m.nameValue
}

// GetConfig implements core.API
func (m *MockAPI) GetConfig() config.APIConfig {
	return m.configValue
}

// Subdomain implements core.API
func (m *MockAPI) Subdomain() string {
	return m.subdomainValue
}

// AuthTokenName implements core.API
func (m *MockAPI) AuthTokenName() string {
	return m.authTokenNameValue
}

// OpenAPIInfo implements core.API
func (m *MockAPI) OpenAPIInfo() router.APIInfoDefinition {
	return m.openAPIInfoValue
}

// Configure implements core.API
func (m *MockAPI) Configure(router router.Router, accessSvc core.AccessService) error {
	if m.configureFunc != nil {
		return m.configureFunc(router, accessSvc)
	}
	return m.MockAPI.Configure(router, accessSvc)
}

// NewMockAPI creates a new mock API
func NewMockAPI(t testing.TB, name string) *MockAPI {
	mockAPI := &MockAPI{
		MockAPI:            mocks.NewMockAPI(t), // Using a default TB implementation
		nameValue:          name,
		subdomainValue:     "",
		authTokenNameValue: "",
	}

	apiInfo := router.APIInfo().Title("Test").Version("0.1.0")

	mockAPI.openAPIInfoValue = apiInfo

	// Setup default expectations
	mockAPI.EXPECT().Name().Return(name).Maybe()
	mockAPI.EXPECT().GetConfig().Return(nil).Maybe()
	mockAPI.EXPECT().Subdomain().Return("").Maybe()
	mockAPI.EXPECT().AuthTokenName().Return("").Maybe()
	mockAPI.EXPECT().OpenAPIInfo().Return(apiInfo).Maybe()
	routerType := "*swagger.Router[github.com/labstack/echo/v4.HandlerFunc,github.com/labstack/echo/v4.MiddlewareFunc,*github.com/labstack/echo/v4.Route]"
	mockAPI.EXPECT().Configure(mock.AnythingOfType(routerType), mock.AnythingOfType("core.AccessService")).Return(nil).Maybe()
	mockAPI.EXPECT().Configure(mock.AnythingOfType(routerType), mock.AnythingOfType("*testing.MockAccessService")).Return(nil).Maybe()

	return mockAPI
}

// WithConfig sets the config for the mock API
func (m *MockAPI) WithConfig(cfg config.APIConfig) *MockAPI {
	m.configValue = cfg
	m.MockAPI.EXPECT().GetConfig().Return(cfg).Maybe()
	return m
}

// WithSubdomain sets the subdomain for the mock API
func (m *MockAPI) WithSubdomain(subdomain string) *MockAPI {
	m.subdomainValue = subdomain
	m.MockAPI.EXPECT().Subdomain().Return(subdomain).Maybe()
	return m
}

// WithAuthTokenName sets the auth token name for the mock API
func (m *MockAPI) WithAuthTokenName(authTokenName string) *MockAPI {
	m.authTokenNameValue = authTokenName
	m.MockAPI.EXPECT().AuthTokenName().Return(authTokenName).Maybe()
	return m
}

// WithOpenAPIInfo sets the OpenAPI info for the mock API
func (m *MockAPI) WithOpenAPIInfo(openAPIInfo router.APIInfoDefinition) *MockAPI {
	m.openAPIInfoValue = openAPIInfo
	m.MockAPI.EXPECT().OpenAPIInfo().Return(openAPIInfo).Maybe()
	return m
}

// WithConfigure sets a custom Configure function for the MockAPI
func (m *MockAPI) WithConfigure(f func(router.Router, core.AccessService) error) *MockAPI {
	m.configureFunc = f
	return m
}

// Config implements core.Component
func (m *MockAPI) Config() config.Manager {
	return m.componentConfig
}

// SetConfig implements core.Component
func (m *MockAPI) SetConfig(cfg config.Manager) {
	m.componentConfig = cfg
}

// Context implements core.Component
func (m *MockAPI) Context() core.Context {
	return nil
}

// SetContext implements core.Component
func (m *MockAPI) SetContext(ctx core.Context) {
}

// Logger implements core.Component
func (m *MockAPI) Logger() *core.Logger {
	return nil
}

// SetLogger implements core.Component
func (m *MockAPI) SetLogger(logger *core.Logger) {
}

// DB implements core.Component
func (m *MockAPI) DB() *gorm.DB {
	return nil
}

// SetDB implements core.Component
func (m *MockAPI) SetDB(db *gorm.DB) {
}

// Ensure MockAPI implements core.API
var _ core.API = (*MockAPI)(nil)
