package testing

import (
	"go.lumeweb.com/portal/config"
	"go.uber.org/zap"
	"sync"
)

var _ config.Manager = (*MockConfigManager)(nil)

// MockConfigManager implements config.Manager for testing
type MockConfigManager struct {
	mu     sync.RWMutex
	values map[string]interface{}
	cfg    *config.Config
	logger *zap.Logger
}

// NewMockConfigManager creates a new mock config manager
func NewMockConfigManager() *MockConfigManager {
	return &MockConfigManager{
		values: make(map[string]interface{}),
		cfg:    &config.Config{},
	}
}

// Init implements config.Manager
func (m *MockConfigManager) Init() error {
	return nil
}

// SetLogger implements config.Manager
func (m *MockConfigManager) SetLogger(logger *zap.Logger) {
	m.logger = logger
}

// RegisterConfigChangeCallback implements config.Manager
func (m *MockConfigManager) RegisterConfigChangeCallback(callback config.ConfigChangeCallback) {
	// No-op for testing
}

// FieldProcessor implements config.Manager
func (m *MockConfigManager) FieldProcessor(obj any, prefix string, processors ...config.FieldProcessor) error {
	return nil
}

// ConfigureProtocol implements config.Manager
func (m *MockConfigManager) ConfigureProtocol(pluginName string, cfg config.ProtocolConfig) error {
	return nil
}

// ConfigureAPI implements config.Manager
func (m *MockConfigManager) ConfigureAPI(pluginName string, cfg config.APIConfig) error {
	return nil
}

// ConfigureService implements config.Manager
func (m *MockConfigManager) ConfigureService(pluginName string, serviceName string, cfg config.ServiceConfig) error {
	return nil
}

// GetPlugin implements config.Manager
func (m *MockConfigManager) GetPlugin(pluginName string) *config.PluginEntity {
	return nil
}

// GetService implements config.Manager
func (m *MockConfigManager) GetService(serviceName string) config.ServiceConfig {
	return nil
}

// GetProtocol implements config.Manager
func (m *MockConfigManager) GetProtocol(pluginName string) config.ProtocolConfig {
	return nil
}

// GetAPI implements config.Manager
func (m *MockConfigManager) GetAPI(pluginName string) config.APIConfig {
	return nil
}

// Config returns the configuration
func (m *MockConfigManager) Config() *config.Config {
	return m.cfg
}

// Save implements config.Manager
func (m *MockConfigManager) Save() error {
	return nil
}

// ConfigFile implements config.Manager
func (m *MockConfigManager) ConfigFile() string {
	return ""
}

// ConfigDir implements config.Manager
func (m *MockConfigManager) ConfigDir() string {
	return ""
}

// Update implements config.Manager
func (m *MockConfigManager) Update(key string, value any) error {
	m.Set(key, value)
	return nil
}

// Exists implements config.Manager
func (m *MockConfigManager) Exists(key string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, exists := m.values[key]
	return exists
}

// Get gets a configuration value
func (m *MockConfigManager) Get(key string) any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.values[key]
}

// Set sets a configuration value
func (m *MockConfigManager) Set(key string, value interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[key] = value
}

// GetString gets a string configuration value
func (m *MockConfigManager) GetString(key string) string {
	val := m.Get(key)
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
	val := m.Get(key)
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
	val := m.Get(key)
	if val == nil {
		return false
	}
	if b, ok := val.(bool); ok {
		return b
	}
	return false
}

// All returns all configuration values
func (m *MockConfigManager) All() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Create a copy to avoid race conditions
	result := make(map[string]interface{}, len(m.values))
	for k, v := range m.values {
		result[k] = v
	}
	return result
}

// IsEditable implements config.Manager
func (m *MockConfigManager) IsEditable(key string) bool {
	return true
}

// Flags implements config.Manager
func (m *MockConfigManager) Flags(key string) []string {
	return []string{}
}
