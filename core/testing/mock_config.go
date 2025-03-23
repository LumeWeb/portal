package testing

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/mock"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core/testing/mocks"
	"go.uber.org/zap"
)

// MockConfigManager implements config.Manager for testing
// It builds on top of the mockery-generated mock and adds state tracking
type MockConfigManager struct {
	*mocks.MockManager
	mu     sync.RWMutex
	values map[string]interface{}
	cfg    *config.Config
	logger *zap.Logger
}

// NewMockConfigManager creates a new mock config manager with state tracking
func NewMockConfigManager(t *testing.T) *MockConfigManager {
	mockManager := mocks.NewMockManager(t)
	manager := &MockConfigManager{
		MockManager: mockManager,
		values:      make(map[string]interface{}),
		cfg:         &config.Config{},
	}

	// Setup the basic methods to use our state tracking instead of requiring explicit expectations
	mockManager.On("Get", mock.AnythingOfType("string")).
		Maybe().
		Return(nil)

	mockManager.On("Exists", mock.AnythingOfType("string")).
		Maybe().
		Return(false)

	mockManager.On("Update", mock.AnythingOfType("string"), mock.Anything).
		Maybe().
		Return(nil).
		Run(func(args mock.Arguments) {
			key := args.String(0)
			value := args.Get(1)
			manager.SetValue(key, value)
		})

	mockManager.On("All").
		Maybe().
		Return(manager.AllValues())

	mockManager.On("Config").
		Maybe().
		Return(manager.cfg)

	mockManager.On("SetLogger", mock.AnythingOfType("*zap.Logger")).
		Maybe().
		Run(func(args mock.Arguments) {
			manager.logger = args.Get(0).(*zap.Logger)
		})

	mockManager.On("IsEditable", mock.AnythingOfType("string")).
		Maybe().
		Return(true)

	mockManager.On("Flags", mock.AnythingOfType("string")).
		Maybe().
		Return([]string{})

	return manager
}

// GetValue gets a configuration value from the internal state
func (m *MockConfigManager) GetValue(key string) interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.values[key]
}

// SetValue sets a configuration value in the internal state
func (m *MockConfigManager) SetValue(key string, value interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[key] = value
}

// ExistsValue checks if a key exists in the internal state
func (m *MockConfigManager) ExistsValue(key string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, exists := m.values[key]
	return exists
}

// AllValues returns all configuration values from the internal state
func (m *MockConfigManager) AllValues() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Create a copy to avoid race conditions
	result := make(map[string]interface{}, len(m.values))
	for k, v := range m.values {
		result[k] = v
	}
	return result
}

// Get implements the Manager.Get method with state tracking
func (m *MockConfigManager) Get(key string) interface{} {
	// We're overriding the MockManager's Get method to use our state tracking
	return m.GetValue(key)
}

// Exists implements the Manager.Exists method with state tracking
func (m *MockConfigManager) Exists(key string) bool {
	// We're overriding the MockManager's Exists method to use our state tracking
	return m.ExistsValue(key)
}

// GetString gets a string configuration value
func (m *MockConfigManager) GetString(key string) string {
	// Set up an expectation for this call
	m.MockManager.On("GetString", key).Return("").Maybe()

	val := m.GetValue(key)
	if val == nil {
		return ""
	}
	if str, ok := val.(string); ok {
		return str
	}
	return ""
}

// GetInt gets an int configuration value
func (m *MockConfigManager) GetInt(key string) int {
	// Set up an expectation for this call
	m.MockManager.On("GetInt", key).Return(0).Maybe()

	val := m.GetValue(key)
	if val == nil {
		return 0
	}
	if i, ok := val.(int); ok {
		return i
	}
	return 0
}

// GetBool gets a bool configuration value
func (m *MockConfigManager) GetBool(key string) bool {
	// Set up an expectation for this call
	m.MockManager.On("GetBool", key).Return(false).Maybe()

	val := m.GetValue(key)
	if val == nil {
		return false
	}
	if b, ok := val.(bool); ok {
		return b
	}
	return false
}

// SetupPluginEntity is a convenience method to set up a plugin entity
func (m *MockConfigManager) SetupPluginEntity(pluginName string, entity *config.PluginEntity) {
	m.MockManager.On("GetPlugin", pluginName).Return(entity)
}

// WithService is a convenience method to set up a service config expectation
func (m *MockConfigManager) WithService(serviceName string, serviceConfig config.ServiceConfig) {
	m.MockManager.On("GetService", serviceName).Return(serviceConfig)
}

// WithProtocol is a convenience method to set up a protocol config expectation
func (m *MockConfigManager) WithProtocol(pluginName string, protocolConfig config.ProtocolConfig) {
	m.MockManager.On("GetProtocol", pluginName).Return(protocolConfig)
}

// WithAPI is a convenience method to set up an API config expectation
func (m *MockConfigManager) WithAPI(pluginName string, apiConfig config.APIConfig) {
	m.MockManager.On("GetAPI", pluginName).Return(apiConfig)
}

// DefaultExpectations sets up default expectations for common methods
func (m *MockConfigManager) DefaultExpectations() {
	m.MockManager.On("Init").Return(nil).Maybe()
	m.MockManager.On("Save").Return(nil).Maybe()
	m.MockManager.On("ConfigFile").Return("/mock/config.yaml").Maybe()
	m.MockManager.On("ConfigDir").Return("/mock").Maybe()
}

// Ensure the type implements the config.Manager interface
var _ config.Manager = (*MockConfigManager)(nil)