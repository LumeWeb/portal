package testing

import (
	"testing"

	"github.com/stretchr/testify/require"
	router "go.lumeweb.com/portal-router"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/testing/mocks"
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
}

// Name implements core.API
func (m *MockAPI) Name() string {
	return m.nameValue
}

// Config implements core.API
func (m *MockAPI) Config() config.APIConfig {
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
	_router, err := router.NewRouter(apiInfo)
	require.NoError(t, err)

	mockAPI.openAPIInfoValue = apiInfo

	// Setup default expectations
	mockAPI.EXPECT().Name().Return(name).Maybe()
	mockAPI.EXPECT().Config().Return(nil).Maybe()
	mockAPI.EXPECT().Subdomain().Return("").Maybe()
	mockAPI.EXPECT().AuthTokenName().Return("").Maybe()
	mockAPI.EXPECT().OpenAPIInfo().Return(apiInfo).Maybe()
	mockAPI.EXPECT().Configure(_router, new(core.AccessService)).Return(nil).Maybe()

	return mockAPI
}

// WithConfig sets the config for the mock API
func (m *MockAPI) WithConfig(cfg config.APIConfig) *MockAPI {
	m.configValue = cfg
	m.MockAPI.On("Config").Return(cfg).Maybe()
	return m
}

// WithSubdomain sets the subdomain for the mock API
func (m *MockAPI) WithSubdomain(subdomain string) *MockAPI {
	m.subdomainValue = subdomain
	m.MockAPI.On("Subdomain").Return(subdomain).Maybe()
	return m
}

// WithAuthTokenName sets the auth token name for the mock API
func (m *MockAPI) WithAuthTokenName(authTokenName string) *MockAPI {
	m.authTokenNameValue = authTokenName
	m.MockAPI.On("AuthTokenName").Return(authTokenName).Maybe()
	return m
}

// WithOpenAPIInfo sets the OpenAPI info for the mock API
func (m *MockAPI) WithOpenAPIInfo(openAPIInfo router.APIInfoDefinition) *MockAPI {
	m.openAPIInfoValue = openAPIInfo
	m.MockAPI.On("OpenAPIInfo").Return(openAPIInfo).Maybe()
	return m
}

// WithConfigure sets a custom Configure function for the MockAPI
func (m *MockAPI) WithConfigure(f func(router.Router, core.AccessService) error) *MockAPI {
	m.configureFunc = f
	return m
}

// Ensure MockAPI implements core.API
var _ core.API = (*MockAPI)(nil)
