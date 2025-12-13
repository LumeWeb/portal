package config

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"go.lumeweb.com/configmanager"
	"go.lumeweb.com/configmanager/source"
	"go.uber.org/zap"
)

// findConfigFileOptions defines options for finding configuration files
type findConfigFileOptions struct {
	Paths           []string   // Search paths, defaults to DefaultConfigPaths if empty
	CreateIfMissing bool       // Create a default config if not found
	CheckWritable   bool       // Check if existing files are writable
	FS              fileSystem // Filesystem interface for operations
	Core            bool
}

// findConfigFile searches for a config file in specified locations with more robust handling
func findConfigFile(options findConfigFileOptions, cm configmanager.Manager) (string, error) {
	paths := options.Paths
	if len(paths) == 0 {
		paths = DefaultConfigPaths
	}

	for _, _path := range paths {
		// Expand environment variables in path
		expandedPath := os.ExpandEnv(_path)

		// Check if path is a directory
		if options.Core {
			expandedPath = path.Join(expandedPath, CoreConfigFile)
		}

		_, err := options.FS.Stat(expandedPath)
		if err == nil {
			// File exists
			if options.CheckWritable {
				file, err := options.FS.OpenFile(expandedPath, os.O_WRONLY, 0644)
				if err != nil {
					continue // Skip unwritable files
				}
				err = file.Close()
				if err != nil {
					return "", err
				}
			}
			return expandedPath, nil
		}

		if os.IsNotExist(err) && options.CreateIfMissing {
			// File doesn't exist and we should create it
			if err := createDefaultConfig(expandedPath, cm, options.FS); err != nil {
				return "", fmt.Errorf("failed to create default config at %s: %w", expandedPath, err)
			}
			return expandedPath, nil
		}
	}

	return "", fmt.Errorf("no valid config file found in paths: %v", paths)
}

// createDefaultConfig creates an empty config file at the specified path
func createDefaultConfig(path string, cm configmanager.Manager, fs fileSystem) error {
	// Create parent directories if they don't exist
	if err := fs.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Create empty file
	file, err := fs.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	return file.Close()
}

type fileSystem interface {
	Stat(name string) (os.FileInfo, error)
	MkdirAll(path string, perm os.FileMode) error
	OpenFile(name string, flag int, perm os.FileMode) (*os.File, error)
	Create(name string) (*os.File, error)
}

type osFS struct{}

func (osFS) Stat(name string) (os.FileInfo, error)        { return os.Stat(name) }
func (osFS) MkdirAll(path string, perm os.FileMode) error { return os.MkdirAll(path, perm) }
func (osFS) OpenFile(name string, flag int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(name, flag, perm)
}
func (osFS) Create(name string) (*os.File, error) { return os.Create(name) }

type ManagerDefault struct {
	configmanager.Manager
	logger      *zap.Logger
	configFile  string
	configDir   string
	configPaths []string

	syncEnabled bool
	fs          fileSystem
	initialized bool

	// Tracking fields
	configuredPlugins   map[string]bool
	configuredAPIs      map[string]bool
	configuredProtocols map[string]bool
	configuredServices  map[string]map[string]bool // plugin -> services
	lock                sync.RWMutex
}

func newManagerDefault(cm configmanager.Manager) *ManagerDefault {
	return &ManagerDefault{
		Manager:             cm,
		logger:              zap.NewNop(),
		fs:                  osFS{}, // Default to OS filesystem
		configuredPlugins:   make(map[string]bool),
		configuredAPIs:      make(map[string]bool),
		configuredProtocols: make(map[string]bool),
		configuredServices:  make(map[string]map[string]bool),
		lock:                sync.RWMutex{},
	}
}

func (m *ManagerDefault) setConfigPaths(configFile string) {
	m.configFile = configFile
	m.configDir = filepath.Dir(configFile)
}

var _ Manager = (*ManagerDefault)(nil)

// ManagerConfig holds configuration options for creating a Manager
type ManagerConfig struct {
	ConfigManager configmanager.Manager
	Logger        *zap.Logger
	ConfigPaths   []string
	FS            fileSystem
}

// newManagerConfig creates a new ManagerConfig with defaults
func newManagerConfig() *ManagerConfig {
	return &ManagerConfig{}
}

// createDefaultConfigManager creates a default config manager with basic setup
func createDefaultConfigManager() (configmanager.Manager, error) {
	cm, err := configmanager.NewConfigManager(
		[]source.ConfigSource{source.NewEnvConfigSource(ENV_PREFIX, ENV_SEPARATOR, source.WithEnvSourceGlobal())},
		configmanager.WithLogger(zap.NewNop()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create configmanager: %w", err)
	}

	// Register core config structs
	if err := cm.RegisterStruct("core", CoreConfig{}); err != nil {
		return nil, fmt.Errorf("failed to register core config struct: %w", err)
	}

	return cm, nil
}

type ManagerOption func(*ManagerConfig)

func WithConfigManager(cm configmanager.Manager) ManagerOption {
	return func(c *ManagerConfig) {
		c.ConfigManager = cm
	}
}

func WithLogger(logger *zap.Logger) ManagerOption {
	return func(c *ManagerConfig) {
		c.Logger = logger
	}
}

func WithConfigPaths(paths []string) ManagerOption {
	return func(c *ManagerConfig) {
		c.ConfigPaths = paths
	}
}

func withFileSystem(fs fileSystem) ManagerOption {
	return func(c *ManagerConfig) {
		c.FS = fs
	}
}

func NewManager(opts ...ManagerOption) (*ManagerDefault, error) {
	// Create config with defaults and apply options
	config := newManagerConfig()
	for _, opt := range opts {
		opt(config)
	}

	// Use provided config manager or create default one
	var cm configmanager.Manager
	if config.ConfigManager != nil {
		cm = config.ConfigManager
	} else {
		var err error
		cm, err = createDefaultConfigManager()
		if err != nil {
			return nil, err
		}
	}

	// Initialize ManagerDefault with config manager
	m := newManagerDefault(cm)

	// Apply additional configuration
	if config.Logger != nil {
		m.logger = config.Logger
	}
	if config.ConfigPaths != nil {
		m.configPaths = config.ConfigPaths
	}
	if config.FS != nil {
		m.fs = config.FS
	}

	// Determine config file and directory
	// Get paths - use custom paths if set, otherwise check env var, then fall back to defaults
	var paths []string
	if len(m.configPaths) > 0 {
		paths = m.configPaths
	} else if customPaths := os.Getenv(ENV_PREFIX + "CONFIG_PATHS"); customPaths != "" {
		paths = strings.Split(customPaths, string(os.PathListSeparator))
	} else {
		paths = DefaultConfigPaths
	}

	configFile, err := findConfigFile(findConfigFileOptions{
		Paths:           paths,
		CreateIfMissing: true,
		CheckWritable:   true,
		FS:              m.fs, // Use the configured filesystem
		Core:            true,
	}, m.Manager)
	if err != nil {
		return nil, fmt.Errorf("failed to find or create config file: %w", err)
	}

	// Set final config paths
	m.setConfigPaths(configFile)

	fileSource := source.NewFileSource(configFile)

	// Register the file source now that we know the path
	m.Manager.RegisterSource(fileSource)
	m.Manager.RegisterNamespace("core", fileSource)

	m.Manager.RegisterSource(source.NewDefaultConfigSource(m, source.WithDefaultSourceGlobal()))
	err = m.Manager.RegisterStruct("", Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to register config: %w", err)
	}

	return m, nil
}

func (m *ManagerDefault) Init() error {
	m.lock.Lock()
	defer m.lock.Unlock()

	if m.initialized {
		return nil
	}

	var loadErr error

	// Load configuration from all sources
	loadErr = m.Manager.Load()

	// Now that all plugins have registered, persist full defaults
	if err := m.persistFullDefaults(); err != nil {
		return fmt.Errorf("failed to persist full defaults: %w", err)
	}

	if loadErr != nil {
		return fmt.Errorf("failed to load config: %w", loadErr)
	}

	m.initialized = true
	return nil
}

// persistFullDefaults ensures config directories exist and persists current config state
func (m *ManagerDefault) persistFullDefaults() error {
	// Ensure per-plugin directories exist
	var dirs []string
	for pluginName := range m.configuredPlugins {
		dirs = append(dirs,
			filepath.Join(m.configDir, PluginsDir, pluginName, ProtoDir),
			filepath.Join(m.configDir, PluginsDir, pluginName, APIDir),
			filepath.Join(m.configDir, PluginsDir, pluginName, ServiceDir),
		)
	}

	for _, dir := range dirs {
		if err := m.fs.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create config directory %s: %w", dir, err)
		}
	}

	// Persist current config state (defaults already handled by default source)
	return m.Manager.Persist()
}

func (m *ManagerDefault) SetLogger(logger *zap.Logger) {
	m.logger = logger
}

// EnableSync enables configuration synchronization
func (m *ManagerDefault) EnableSync(opts ...configmanager.ConfigOption) error {
	if m.syncEnabled {
		return nil // Already enabled
	}

	if err := m.SetupSync(opts...); err != nil {
		return fmt.Errorf("failed to setup sync: %w", err)
	}

	m.syncEnabled = true
	return nil
}

func (m *ManagerDefault) Config() *Config {
	cfg := &Config{
		Core:   CoreConfig{},
		Plugin: make(map[string]PluginEntity),
	}

	// Get core config first
	if _, _, err := m.Manager.Get("core", &cfg.Core); err != nil {
		m.logger.Error("failed to get core config", zap.Error(err))
	}

	m.lock.RLock()
	defer m.lock.RUnlock()

	// Process protocols
	for pluginName, configured := range m.configuredProtocols {
		if !configured {
			continue
		}
		protoCfg := m.GetProtocol(pluginName)
		if protoCfg != nil {
			if _, exists := cfg.Plugin[pluginName]; !exists {
				cfg.Plugin[pluginName] = PluginEntity{}
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
				cfg.Plugin[pluginName] = PluginEntity{}
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
			cfg.Plugin[pluginName] = PluginEntity{
				Service: make(map[string]ServiceConfig),
			}
		}
		pe := cfg.Plugin[pluginName]
		if pe.Service == nil {
			pe.Service = make(map[string]ServiceConfig)
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

func (m *ManagerDefault) getPluginConfigFile(pluginName, subDir string, configType string) (string, error) {
	filePath := filepath.Join(m.configDir, PluginsDir, pluginName, subDir, SectionConfigFile)
	configFile, err := findConfigFile(findConfigFileOptions{
		Paths:           []string{filePath},
		CreateIfMissing: true,
		CheckWritable:   true,
		FS:              m.fs,
	}, m)
	if err != nil {
		return "", fmt.Errorf("failed to find/create config file for %s '%s': %w", configType, pluginName, err)
	}
	return configFile, nil
}

func (m *ManagerDefault) ConfigureProtocol(pluginName string, cfg ProtocolConfig) error {
	if cfg == nil {
		return nil
	}

	pluginName = strings.ToLower(pluginName)
	key := fmt.Sprintf(ProtocolSpecifier, pluginName)

	// Register the protocol config struct
	if err := m.Manager.RegisterStruct(key, cfg); err != nil {
		return fmt.Errorf("failed to register protocol config: %w", err)
	}

	configFile, err := m.getPluginConfigFile(pluginName, ProtoDir, "Protocol")
	if err != nil {
		return err
	}

	// Register namespace for this protocol
	fsSource := source.NewFileSource(configFile)
	m.Manager.RegisterNamespace(key, fsSource)
	m.Manager.RegisterSource(fsSource)

	m.lock.Lock()
	defer m.lock.Unlock()
	m.configuredProtocols[pluginName] = true
	m.configuredPlugins[pluginName] = true

	return nil
}

func (m *ManagerDefault) ConfigureAPI(pluginName string, cfg APIConfig) error {
	if cfg == nil {
		return nil
	}

	pluginName = strings.ToLower(pluginName)
	key := fmt.Sprintf(APISpecifier, pluginName)

	// Register the API config struct
	if err := m.Manager.RegisterStruct(key, cfg); err != nil {
		return fmt.Errorf("failed to register API config: %w", err)
	}

	configFile, err := m.getPluginConfigFile(pluginName, APIDir, "API")
	if err != nil {
		return err
	}

	// Register namespace for this API
	fsSource := source.NewFileSource(configFile)
	m.Manager.RegisterNamespace(key, fsSource)
	m.Manager.RegisterSource(fsSource)

	m.lock.Lock()
	defer m.lock.Unlock()
	m.configuredAPIs[pluginName] = true
	m.configuredPlugins[pluginName] = true

	return nil
}

func (m *ManagerDefault) ConfigureService(pluginName string, serviceName string, cfg ServiceConfig) error {
	if cfg == nil {
		return nil
	}

	pluginName = strings.ToLower(pluginName)
	key := fmt.Sprintf(ServiceSpecifier, pluginName, serviceName)
	filePath := filepath.Join(m.configDir, PluginsDir, pluginName, ServiceDir, serviceName+CONFIG_EXTENSION)

	// Register the service config struct
	if err := m.Manager.RegisterStruct(key, cfg); err != nil {
		return fmt.Errorf("failed to register service config: %w", err)
	}

	configFile, err := findConfigFile(findConfigFileOptions{
		Paths:           []string{filePath},
		CreateIfMissing: true,
		CheckWritable:   true,
		FS:              m.fs,
	}, m)
	if err != nil {
		return fmt.Errorf("failed to find/create config file for Service '%s/%s': %w", pluginName, serviceName, err)
	}

	// Register namespace for this service
	fsSource := source.NewFileSource(configFile)
	m.Manager.RegisterNamespace(key, fsSource)
	m.Manager.RegisterSource(fsSource)

	m.lock.Lock()
	defer m.lock.Unlock()
	if _, exists := m.configuredServices[pluginName]; !exists {
		m.configuredServices[pluginName] = make(map[string]bool)
	}
	m.configuredServices[pluginName][serviceName] = true
	m.configuredPlugins[pluginName] = true

	return nil
}

func (m *ManagerDefault) GetService(pluginName string, serviceName string) ServiceConfig {
	pluginName = strings.ToLower(pluginName)
	key := fmt.Sprintf(ServiceSpecifier, pluginName, serviceName)

	_, cfg, err := m.Manager.Get(key)

	if err != nil {
		if m.logger != nil {
			if strings.Contains(err.Error(), "not found") {
				m.logger.Warn("failed to get service config",
					zap.String("plugin", pluginName),
					zap.String("service", serviceName),
					zap.Error(err),
				)
			} else {
				m.logger.Error("failed to get service config",
					zap.String("plugin", pluginName),
					zap.String("service", serviceName),
					zap.Error(err),
				)
			}
		}
		return nil
	}

	svcCfg, ok := cfg.(ServiceConfig)
	if !ok {
		if m.logger != nil {
			m.logger.Error("invalid service config type",
				zap.String("plugin", pluginName),
				zap.String("service", serviceName),
				zap.String("expected", "ServiceConfig"),
				zap.Any("actual", cfg),
			)
		}
		return nil
	}

	return svcCfg
}

func (m *ManagerDefault) GetProtocol(pluginName string) ProtocolConfig {
	key := fmt.Sprintf(ProtocolSpecifier, pluginName)
	_, cfg, err := m.Manager.Get(key)

	if err != nil {
		if m.logger != nil {
			if strings.Contains(err.Error(), "not found") {
				m.logger.Warn("failed to get protocol config",
					zap.String("plugin", pluginName),
					zap.Error(err),
				)
			} else {
				m.logger.Error("failed to get protocol config",
					zap.String("plugin", pluginName),
					zap.Error(err),
				)
			}
		}
		return nil
	}

	protoCfg, ok := cfg.(ProtocolConfig)
	if !ok {
		if m.logger != nil {
			m.logger.Error("invalid protocol config type",
				zap.String("plugin", pluginName),
				zap.String("expected", "ProtocolConfig"),
				zap.Any("actual", cfg),
			)
		}
		return nil
	}

	return protoCfg
}

func (m *ManagerDefault) GetAPI(pluginName string) APIConfig {
	key := fmt.Sprintf(APISpecifier, pluginName)

	_, cfg, err := m.Manager.Get(key)

	if err != nil {
		if m.logger != nil {
			if strings.Contains(err.Error(), "not found") {
				m.logger.Warn("failed to get API config",
					zap.String("plugin", pluginName),
					zap.Error(err),
				)
			} else {
				m.logger.Error("failed to get API config",
					zap.String("plugin", pluginName),
					zap.Error(err),
				)
			}
		}
		return nil
	}

	apiCfg, ok := cfg.(APIConfig)
	if !ok {
		if m.logger != nil {
			m.logger.Error("invalid API config type",
				zap.String("plugin", pluginName),
				zap.String("expected", "APIConfig"),
				zap.Any("actual", cfg),
			)
		}
		return nil
	}

	return apiCfg
}
