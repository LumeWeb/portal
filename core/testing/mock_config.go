package testing

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"go.lumeweb.com/configmanager"
	"go.lumeweb.com/configmanager/source"
	"go.lumeweb.com/portal/config"
	"go.uber.org/zap"
)

const (
	// Key format constants for configuration
	ProtocolKeyFormat = "%s.protocol"  // Format for protocol configuration keys
	APIKeyFormat      = "%s.api"       // Format for API configuration keys
	ServiceKeyFormat  = "%s.service.%s" // Format for service configuration keys
	CoreKeyFormat     = "core"         // Core configuration key
)

const mapStructureTag = "config"

// MockConfigManager implements config.Manager for testing
// It builds on top of the mockery-generated mock and adds state tracking
type MockConfigManager struct {
	*config.MockManager
	mu     sync.RWMutex
	cm     configmanager.Manager // Internal configmanager
	logger *zap.Logger
	cfg    *config.Config
}

// NewMockConfigManager creates a new mock config manager with state tracking
func NewMockConfigManager(t *testing.T) *MockConfigManager {
	mockManager := config.NewMockManager(t)

	// Create a confmap provider for initial values (can be empty)
	initialValues := map[string]interface{}{
		CoreKeyFormat: map[string]interface{}{},
	}

	// Initialize ConfigManager with the confmap source
	cm, err := configmanager.NewConfigManager(configmanager.UsingSources(source.NewMemoryConfigSource(initialValues)), configmanager.WithLogger(zap.NewNop()))
	if err != nil {
		panic(err) // Handle error appropriately in tests
	}

	// Register core config structs
	if err := cm.RegisterStruct("core", config.CoreConfig{}); err != nil {
		panic(err)
	}

	manager := &MockConfigManager{
		MockManager: mockManager,
		cm:          cm,
		logger:      zap.NewNop(),
		cfg:         &config.Config{Plugin: make(map[string]config.PluginEntity)},
	}

	// Setup the basic methods to use our state tracking instead of requiring explicit expectations
	mockManager.On("Get", mock.AnythingOfType("string"), mock.Anything).
		Maybe().
		Return(nil, nil, nil) // Return nil, nil, nil

	mockManager.On("Exists", mock.AnythingOfType("string")).
		Maybe().
		Return(false)

	mockManager.On("Set", mock.AnythingOfType("context.Context"), mock.AnythingOfType("string"), mock.Anything).
		Maybe().
		Return(nil).
		Run(func(args mock.Arguments) {
			ctx := args.Get(0).(context.Context)
			key := args.String(1)
			value := args.Get(2)
			_ = manager.cm.Set(ctx, key, value) // Use cm.Set
		})

	mockManager.On("All").
		Maybe().
		Return(manager.cm.All()) // Delegate to cm.All()

	mockManager.On("Config").
		Maybe().
		Return(manager.Config())

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

	// Setup default expectations for configuration methods
	mockManager.On("ConfigureAPI", mock.AnythingOfType("string"), mock.Anything).
		Maybe().
		Return(nil)

	mockManager.On("ConfigureProtocol", mock.AnythingOfType("string"), mock.Anything).
		Maybe().
		Return(nil)

	mockManager.On("ConfigureService", mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.Anything).
		Maybe().
		Return(nil)

	mockManager.On("GetString", mock.AnythingOfType("string")).
		Maybe().
		Return("", nil)

	mockManager.On("GetInt", mock.AnythingOfType("string")).
		Maybe().
		Return(int64(0), nil)

	mockManager.On("GetBool", mock.AnythingOfType("string")).
		Maybe().
		Return(false, nil)

	mockManager.On("GetDuration", mock.AnythingOfType("string")).
		Maybe().
		Return(time.Duration(0), nil)

	mockManager.On("GetRegisteredStructs").
		Maybe().
		Return(map[string]reflect.Type{})

	return manager
}

// Get implements the Manager.Get method with state tracking
func (m *MockConfigManager) Get(key string, target ...any) (any, any, error) {
	return m.cm.Get(key, target...)
}

// Exists implements the Manager.Exists method with state tracking
func (m *MockConfigManager) Exists(key string) bool {
	return m.cm.Exists(key)
}

// Init implements config.Manager
func (m *MockConfigManager) Init() error {
	if m.MockManager != nil {
		return m.MockManager.Init()
	}
	return nil
}

// SetLogger implements config.Manager
func (m *MockConfigManager) SetLogger(logger *zap.Logger) {
	if m.MockManager != nil {
		m.MockManager.SetLogger(logger)
	}
	m.logger = logger
}

// Config returns the configuration
func (m *MockConfigManager) Config() *config.Config {
	var cfg config.Config
	if _, _, err := m.cm.Get(CoreKeyFormat, &cfg); err != nil {
		panic(err)
	}
	return &cfg
}

// ConfigureProtocol implements config.Manager
func (m *MockConfigManager) ConfigureProtocol(pluginName string, cfg config.ProtocolConfig) error {
	if m.MockManager != nil {
		err := m.MockManager.ConfigureProtocol(pluginName, cfg)
		if err != nil {
			return err
		}
	}

	key := fmt.Sprintf(ProtocolKeyFormat, pluginName)
	err := m.cm.RegisterStruct(key, cfg)
	if err != nil {
		return err
	}
	return nil
}

// ConfigureAPI implements config.Manager
func (m *MockConfigManager) ConfigureAPI(pluginName string, cfg config.APIConfig) error {
	if m.MockManager != nil {
		err := m.MockManager.ConfigureAPI(pluginName, cfg)
		if err != nil {
			return err
		}
	}
	key := fmt.Sprintf(APIKeyFormat, pluginName)
	err := m.cm.RegisterStruct(key, cfg)
	if err != nil {
		return err
	}
	return nil
}

// ConfigureService implements config.Manager
func (m *MockConfigManager) ConfigureService(pluginName string, serviceName string, cfg config.ServiceConfig) error {
	if m.MockManager != nil {
		err := m.MockManager.ConfigureService(pluginName, serviceName, cfg)
		if err != nil {
			return err
		}
	}
	key := fmt.Sprintf(ServiceKeyFormat, pluginName, serviceName)
	err := m.cm.RegisterStruct(key, cfg)
	if err != nil {
		return err
	}
	return nil
}

// GetString gets a string configuration value
func (m *MockConfigManager) GetString(key string) (string, error) {
	// Set up an expectation for this call if MockManager is initialized
	if m.MockManager != nil {
		m.MockManager.On("GetString", key).Return("", nil).Maybe()
	}

	_, val, err := m.cm.Get(key)
	if err != nil {
		return "", err
	}
	if val == nil {
		return "", nil
	}
	str, ok := val.(string)
	if !ok {
		return "", nil
	}
	return str, nil
}

// GetInt gets an int configuration value
func (m *MockConfigManager) GetInt(key string) (int64, error) {
	// Set up an expectation for this call if MockManager is initialized
	if m.MockManager != nil {
		m.MockManager.On("GetInt", key).Return(int64(0), nil).Maybe()
	}

	_, val, err := m.cm.Get(key)
	if err != nil {
		return 0, err
	}
	if val == nil {
		return 0, nil
	}

	switch v := val.(type) {
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case float64:
		return int64(v), nil
	case string:
		i, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("failed to parse string as int64: %w", err)
		}
		return i, nil
	default:
		return 0, nil
	}
}

// GetBool gets a bool configuration value
func (m *MockConfigManager) GetBool(key string) (bool, error) {
	// Set up an expectation for this call if MockManager is initialized
	if m.MockManager != nil {
		m.MockManager.On("GetBool", key).Return(false, nil).Maybe()
	}

	_, val, err := m.cm.Get(key)
	if err != nil {
		return false, err
	}
	if val == nil {
		return false, nil
	}
	b, ok := val.(bool)
	if !ok {
		return false, nil
	}
	return b, nil
}

// GetDuration gets a duration configuration value
func (m *MockConfigManager) GetDuration(key string) (time.Duration, error) {
	// Set up an expectation for this call if MockManager is initialized
	if m.MockManager != nil {
		m.MockManager.On("GetDuration", key).Return(time.Duration(0), nil).Maybe()
	}

	_, val, err := m.cm.Get(key)
	if err != nil {
		return 0, err
	}
	if val == nil {
		return 0, nil
	}
	d, ok := val.(time.Duration)
	if !ok {
		return 0, nil
	}
	return d, nil
}

// GetRegisteredStructs gets the registered structs
func (m *MockConfigManager) GetRegisteredStructs() map[string]reflect.Type {
	// Set up an expectation for this call if MockManager is initialized
	if m.MockManager != nil {
		m.MockManager.On("GetRegisteredStructs").Return(m.cm.GetRegisteredStructs()).Maybe()
	}
	return m.cm.GetRegisteredStructs()
}

// Ensure the type implements the config.Manager interface
var _ config.Manager = (*MockConfigManager)(nil)
