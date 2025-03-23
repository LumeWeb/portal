package testing_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	coretesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/db/models"
	"go.uber.org/zap"
)

// Simple mock implementation of ServiceConfig for testing
type MockServiceConfig struct{}

func (c *MockServiceConfig) Defaults() map[string]interface{} {
	return map[string]interface{}{
		"name": "test-service",
	}
}

// Example of using the MockConfigManager
func TestConfigManager(t *testing.T) {
	// Create the mock config manager
	manager := coretesting.NewMockConfigManager(t)

	// Set up some config values
	manager.SetValue("feature.enabled", true)
	manager.SetValue("app.name", "Portal")
	manager.SetValue("max.connections", 100)

	// Test retrieving values
	assert.True(t, manager.GetBool("feature.enabled"))
	assert.Equal(t, "Portal", manager.GetString("app.name"))
	assert.Equal(t, 100, manager.GetInt("max.connections"))

	// Set up a plugin entity for a test
	mockService := &MockServiceConfig{}
	pluginEntity := &config.PluginEntity{
		Service: map[string]config.ServiceConfig{
			"test-service": mockService,
		},
	}
	manager.SetupPluginEntity("test-plugin", pluginEntity)

	// Test getting the plugin entity
	result := manager.GetPlugin("test-plugin")
	assert.Equal(t, pluginEntity, result)
}

// Example of using the mockery-generated mocks with the MockConfigManager
func TestServiceWithConfig(t *testing.T) {
	// Create mock auth service
	authMock := &MockAuthService{
		MockService: &MockService{IDValue: core.AUTH_SERVICE},
	}

	// Set up a test user
	user := &models.User{
		Email: "test@example.com",
	}

	// Set up the auth service behavior
	authMock.LoginPasswordFunc = func(email string, password string, ip string, rememberMe bool) (string, *models.User, error) {
		return "jwt-token", user, nil
	}

	// Create a test config manager
	configManager := coretesting.NewMockConfigManager(t)
	configManager.DefaultExpectations()

	// Test the auth service with the config manager
	token, resultUser, err := authMock.LoginPassword("test@example.com", "password", "127.0.0.1", true)
	
	assert.NoError(t, err)
	assert.Equal(t, "jwt-token", token)
	assert.Equal(t, user, resultUser)
	assert.Equal(t, core.AUTH_SERVICE, authMock.ID())
}

// MockService is a simple mock implementation of core.Service for the example
type MockService struct {
	IDValue string
}

func (s *MockService) ID() string {
	return s.IDValue
}

// MockAuthService implements core.AuthService for testing
type MockAuthService struct {
	*MockService
	LoginPasswordFunc func(email string, password string, ip string, rememberMe bool) (string, *models.User, error)
}

func (s *MockAuthService) LoginPassword(email string, password string, ip string, rememberMe bool) (string, *models.User, error) {
	return s.LoginPasswordFunc(email, password, ip, rememberMe)
}

// Example of using NewTestContext with benchmarks
func BenchmarkWithTestContext(b *testing.B) {
	// Create a test context with a benchmark
	ctx := coretesting.NewTestContext(b)
	defer ctx.Teardown()

	// Configure test context
	mockAuth := &MockAuthService{
		MockService: &MockService{IDValue: core.AUTH_SERVICE},
	}
	mockAuth.LoginPasswordFunc = func(email string, password string, ip string, rememberMe bool) (string, *models.User, error) {
		return "token", &models.User{Email: email}, nil
	}

	// Register the service
	ctx.RegisterService(core.AUTH_SERVICE, mockAuth)

	// Get a logger with context
	logger := ctx.WithLogger(zap.String("benchmark", "example"))

	// Run the benchmark
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc := ctx.Service(core.AUTH_SERVICE).(*MockAuthService)
		token, user, err := svc.LoginPassword("test@example.com", "password", "127.0.0.1", false)
		if err != nil || token == "" || user == nil {
			b.Fatalf("Login failed: %v", err)
		}
		logger.Info("Login successful", zap.String("token", token))
	}
}