package testing

import (
	"context"
	"fmt"
	z "github.com/Oudwins/zog"
	"os"
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
	ProtocolKeyFormat = "plugin.%s.protocol"   // Format for protocol configuration keys
	APIKeyFormat      = "plugin.%s.api"        // Format for API configuration keys
	ServiceKeyFormat  = "plugin.%s.service.%s" // Format for service configuration keys
	CoreKeyFormat     = "core"                 // Core configuration key
)

const mapStructureTag = "config"

var _ config.Defaults = (*mockConfigEntry)(nil)
var _ config.ConfigSchemaProvider = (*mockConfigEntry)(nil)

type mockConfigEntry struct {
}

func (c mockConfigEntry) Schema() z.ZogSchema {
	return nil
}

func (c mockConfigEntry) Defaults() map[string]any {
	return make(map[string]any)
}

// MockConfigManager implements config.Manager for testing
// It builds on top of the mockery-generated mock and adds state tracking
type MockConfigManager struct {
	*config.MockManager
	mu     sync.RWMutex
	cm     configmanager.Manager // Internal configmanager
	logger *zap.Logger
	cfg    *config.Config

	// Tracking fields to match real implementation
	configuredPlugins   map[string]bool
	configuredAPIs      map[string]bool
	configuredProtocols map[string]bool
	configuredServices  map[string]map[string]bool // plugin -> services
}

// NewMockConfigManager creates a new mock config manager with state tracking
func NewMockConfigManager(t *testing.T) *MockConfigManager {
	mockManager := config.NewMockManager(t)

	// Initialize ConfigManager with the confmap source
	cm, err := configmanager.NewConfigManager(configmanager.UsingSources(source.NewMemoryConfigSource(map[string]any{
		"core.domain":      "portal.local",
		"core.portal_name": "portal",
	})), configmanager.WithLogger(zap.NewNop()))
	if err != nil {
		panic(err) // Handle error appropriately in tests
	}

	cm.RegisterSource(source.NewDefaultConfigSource(cm, source.WithDefaultSourceGlobal()))

	// Register config struct
	if err = cm.RegisterStruct("", config.Config{}); err != nil {
		panic(err)
	}

	// Register core config struct
	if err = cm.RegisterStruct(CoreKeyFormat, config.CoreConfig{}); err != nil {
		panic(err)
	}

	err = cm.Load()
	if err != nil {
		panic(fmt.Sprintf("failed to load config manager: %v", err))
	}

	manager := &MockConfigManager{
		MockManager:         mockManager,
		cm:                  cm,
		logger:              zap.NewNop(),
		cfg:                 &config.Config{Plugin: make(map[string]config.PluginEntity)},
		configuredPlugins:   make(map[string]bool),
		configuredAPIs:      make(map[string]bool),
		configuredProtocols: make(map[string]bool),
		configuredServices:  make(map[string]map[string]bool),
	}

	mockManager.EXPECT().Get(mock.AnythingOfType("string"), mock.Anything).
		Maybe().
		Return(nil, nil, nil) // Return nil, nil, nil

	mockManager.EXPECT().Exists(mock.AnythingOfType("string")).
		Maybe().
		Return(false)

	mockManager.EXPECT().Set(
		mock.MatchedBy(func(ctx interface{}) bool {
			_, ok := ctx.(context.Context)
			return ok
		}),
		mock.AnythingOfType("string"),
		mock.Anything,
	).Maybe().Return(nil).Run(func(args mock.Arguments) {
		ctx := args.Get(0).(context.Context)
		key := args.String(1)
		value := args.Get(2)
		_ = manager.cm.Set(ctx, key, value) // Use cm.Set
	})

	mockManager.EXPECT().All().
		Maybe().
		Return(manager.cm.All()) // Delegate to cm.All()

	mockManager.EXPECT().Config().
		Maybe().
		Return(manager.Config())

	mockManager.EXPECT().SetLogger(mock.AnythingOfType("*zap.Logger")).
		Maybe().
		Run(func(args mock.Arguments) {
			manager.logger = args.Get(0).(*zap.Logger)
		})

	// Setup default expectations for configuration methods
	mockManager.EXPECT().ConfigureAPI(mock.AnythingOfType("string"), mock.Anything).
		Maybe().
		Return(nil)

	mockManager.EXPECT().ConfigureProtocol(mock.AnythingOfType("string"), mock.Anything).
		Maybe().
		Return(nil)

	mockManager.EXPECT().ConfigureService(mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.Anything).
		Maybe().
		Return(nil)

	mockManager.EXPECT().GetString(mock.AnythingOfType("string")).
		Maybe().
		Return("", nil)

	mockManager.EXPECT().GetInt(mock.AnythingOfType("string")).
		Maybe().
		Return(int64(0), nil)

	mockManager.EXPECT().GetBool(mock.AnythingOfType("string")).
		Maybe().
		Return(false, nil)

	mockManager.EXPECT().GetDuration(mock.AnythingOfType("string")).
		Maybe().
		Return(time.Duration(0), nil)

	mockManager.EXPECT().GetRegisteredStructs().
		Maybe().
		Return(map[string]reflect.Type{})

	tempDir, err := os.MkdirTemp("", "portal-test-")
	if err != nil {
		panic(fmt.Sprintf("failed to create temp dir: %v", err))
	}
	t.Cleanup(func() {
		os.RemoveAll(tempDir)
	})
	mockManager.EXPECT().ConfigDir().
		Maybe().
		Return(tempDir, nil)

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
	cfg := &config.Config{
		Core:   config.CoreConfig{},
		Plugin: make(map[string]config.PluginEntity),
	}

	// Get core config first
	if _, _, err := m.cm.Get("core", &cfg.Core); err != nil {
		m.logger.Error("failed to get core config", zap.Error(err))
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Process protocols
	for pluginName, configured := range m.configuredProtocols {
		if !configured {
			continue
		}
		protoCfg := m.GetProtocol(pluginName)
		if protoCfg != nil {
			if _, exists := cfg.Plugin[pluginName]; !exists {
				cfg.Plugin[pluginName] = config.PluginEntity{}
			}
			pe := cfg.Plugin[pluginName]
			pe.Protocol = protoCfg
			cfg.Plugin[pluginName] = pe
		}
	}

	// Process APIs
	for pluginName, configured := range m.configuredAPIs {
		if !configured {
			continue
		}
		apiCfg := m.GetAPI(pluginName)
		if apiCfg != nil {
			if _, exists := cfg.Plugin[pluginName]; !exists {
				cfg.Plugin[pluginName] = config.PluginEntity{}
			}
			pe := cfg.Plugin[pluginName]
			pe.API = apiCfg
			cfg.Plugin[pluginName] = pe
		}
	}

	// Process services
	for pluginName, services := range m.configuredServices {
		if len(services) == 0 {
			continue
		}
		if _, exists := cfg.Plugin[pluginName]; !exists {
			cfg.Plugin[pluginName] = config.PluginEntity{
				Service: make(map[string]config.ServiceConfig),
			}
		}
		pe := cfg.Plugin[pluginName]
		if pe.Service == nil {
			pe.Service = make(map[string]config.ServiceConfig)
		}

		for serviceName := range services {
			svcCfg := m.GetService(pluginName, serviceName)
			if svcCfg != nil {
				pe.Service[serviceName] = svcCfg
			}
		}
		cfg.Plugin[pluginName] = pe
	}

	return cfg
}

// ConfigureProtocol implements config.Manager
func (m *MockConfigManager) ConfigureProtocol(pluginName string, cfg config.ProtocolConfig) error {
	key := fmt.Sprintf(ProtocolKeyFormat, pluginName)
	err := m.cm.RegisterStruct(key, cfg)
	if err != nil {
		return err
	}

	defSource := source.NewDefaultConfigSource(m.cm, source.WithDefaultSourceGlobal())
	m.cm.RegisterNamespace(key, defSource)
	m.cm.RegisterSource(defSource)
	err = m.cm.LoadNamespace(key)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.configuredProtocols[pluginName] = true
	m.configuredPlugins[pluginName] = true

	if m.MockManager != nil {
		_, newCfg, err := m.cm.Get(key)
		// Setup Maybe expectation for GetProtocol
		m.MockManager.EXPECT().GetProtocol(pluginName).Maybe().Return(newCfg.(config.ProtocolConfig), nil)
		err = m.MockManager.ConfigureProtocol(pluginName, cfg)
		if err != nil {
			return err
		}
	}
	return nil
}

// ConfigureAPI implements config.Manager
func (m *MockConfigManager) ConfigureAPI(pluginName string, cfg config.APIConfig) error {
	key := fmt.Sprintf(APIKeyFormat, pluginName)
	err := m.cm.RegisterStruct(key, cfg)
	if err != nil {
		return err
	}

	defSource := source.NewDefaultConfigSource(m.cm, source.WithDefaultSourceGlobal())
	m.cm.RegisterNamespace(key, defSource)
	m.cm.RegisterSource(defSource)
	err = m.cm.LoadNamespace(key)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.configuredAPIs[pluginName] = true
	m.configuredPlugins[pluginName] = true

	if m.MockManager != nil {
		// Setup Maybe expectation for GetAPI
		m.MockManager.EXPECT().GetAPI(pluginName).Maybe().Return(cfg, nil)
		err := m.MockManager.ConfigureAPI(pluginName, cfg)
		if err != nil {
			return err
		}
	}

	return nil
}

// ConfigureService implements config.Manager
func (m *MockConfigManager) ConfigureService(pluginName string, serviceName string, cfg config.ServiceConfig) error {
	key := fmt.Sprintf(ServiceKeyFormat, pluginName, serviceName)
	err := m.cm.RegisterStruct(key, cfg)
	if err != nil {
		return err
	}

	defSource := source.NewDefaultConfigSource(m.cm, source.WithDefaultSourceGlobal())
	m.cm.RegisterNamespace(key, defSource)
	m.cm.RegisterSource(defSource)
	err = m.cm.LoadNamespace(key)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.configuredServices[pluginName]; !exists {
		m.configuredServices[pluginName] = make(map[string]bool)
	}
	m.configuredServices[pluginName][serviceName] = true
	m.configuredPlugins[pluginName] = true

	if m.MockManager != nil {
		// Setup Maybe expectation for GetService
		m.MockManager.EXPECT().GetService(pluginName, serviceName).Maybe().Return(cfg, nil)
		err := m.MockManager.ConfigureService(pluginName, serviceName, cfg)
		if err != nil {
			return err
		}
	}
	return nil
}

// GetString gets a string configuration value
func (m *MockConfigManager) GetString(key string) (string, error) {
	// Set up an expectation for this call if MockManager is initialized
	if m.MockManager != nil {
		m.MockManager.EXPECT().GetString(key).Return("", nil).Maybe()
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
		m.MockManager.EXPECT().GetInt(key).Return(int64(0), nil).Maybe()
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
		m.MockManager.EXPECT().GetBool(key).Return(false, nil).Maybe()
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
		m.MockManager.EXPECT().GetDuration(key).Return(time.Duration(0), nil).Maybe()
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
		m.MockManager.EXPECT().GetRegisteredStructs().Return(m.cm.GetRegisteredStructs()).Maybe()
	}
	return m.cm.GetRegisteredStructs()
}

// Ensure the type implements the config.Manager interface
var _ config.Manager = (*MockConfigManager)(nil)
