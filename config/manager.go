package config

import (
	"fmt"
	"go.lumeweb.com/configmanager"
	"go.lumeweb.com/configmanager/source"
	"go.uber.org/zap"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"

	"github.com/urfave/cli/v3"
)

// findConfigFileOptions defines options for finding configuration files
type findConfigFileOptions struct {
	Paths           []string   // Search paths, defaults to DefaultConfigPaths if empty
	CreateIfMissing bool       // Create a default config if not found
	CheckWritable   bool       // Check if existing files are writable
	fs              fileSystem // Filesystem interface for operations
}

// findConfigFile searches for a config file in specified locations with more robust handling
func findConfigFile(options findConfigFileOptions, cm configmanager.Manager) (string, error) {
	paths := options.Paths
	if len(paths) == 0 {
		paths = DefaultConfigPaths
	}

	for _, _path := range paths {
		// Expand environment variables in both the path and filename
		expandedPath := path.Join(os.ExpandEnv(_path), CoreConfigFile)

		_, err := options.fs.Stat(expandedPath)
		if err == nil {
			// File exists
			if options.CheckWritable {
				file, err := options.fs.OpenFile(expandedPath, os.O_WRONLY, 0644)
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
			if err := createDefaultConfig(expandedPath, cm, options.fs); err != nil {
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
	cmd         *cli.Command
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

func newManagerDefault(cm configmanager.Manager, cmd *cli.Command) *ManagerDefault {
	return &ManagerDefault{
		Manager:             cm,
		logger:              zap.NewNop(),
		cmd:                 cmd,
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

type ManagerOption func(*ManagerDefault)

func withFileSystem(fs fileSystem) ManagerOption {
	return func(m *ManagerDefault) {
		m.fs = fs
	}
}

func WithLogger(logger *zap.Logger) ManagerOption {
	return func(m *ManagerDefault) {
		m.logger = logger
	}
}

func WithConfigPaths(paths []string) ManagerOption {
	return func(m *ManagerDefault) {
		m.configPaths = paths
	}
}

func NewManager(cmd *cli.Command, opts ...ManagerOption) (*ManagerDefault, error) {
	// First create a basic ConfigManager with just env source
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

	// Initialize ManagerDefault with basic values
	m := newManagerDefault(cm, cmd)

	// Apply options which may override defaults
	for _, opt := range opts {
		opt(m)
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
		fs:              m.fs, // Use the configured filesystem
	}, cm)
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
	// Load configuration from all sources
	if err := m.Manager.Load(); err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Now that all plugins have registered, persist full defaults
	if err := m.persistFullDefaults(); err != nil {
		return fmt.Errorf("failed to persist full defaults: %w", err)
	}

	m.initialized = true
	return nil
}

// persistFullDefaults ensures config directories exist and persists current config state
func (m *ManagerDefault) persistFullDefaults() error {
	// Ensure plugin directories exist
	dirs := []string{
		filepath.Join(m.configDir, ProtoDir),
		filepath.Join(m.configDir, APIDir),
		filepath.Join(m.configDir, ServiceDir),
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

func (m *ManagerDefault) ConfigureProtocol(pluginName string, cfg ProtocolConfig) error {
	if cfg == nil {
		return nil
	}

	key := fmt.Sprintf(ProtocolSpecifier, pluginName)
	filePath := filepath.Join(m.configDir, ProtoDir, pluginName+CONFIG_EXTENSION)

	// Register the protocol config struct
	if err := m.Manager.RegisterStruct(key, cfg); err != nil {
		return fmt.Errorf("failed to register protocol config: %w", err)
	}

	// Register namespace for this protocol
	fsSource := source.NewFileSource(filePath)
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

	key := fmt.Sprintf(APISpecifier, pluginName)
	filePath := filepath.Join(m.configDir, APIDir, pluginName+CONFIG_EXTENSION)

	// Register the API config struct
	if err := m.Manager.RegisterStruct(key, cfg); err != nil {
		return fmt.Errorf("failed to register API config: %w", err)
	}

	// Register namespace for this API
	fsSource := source.NewFileSource(filePath)
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

	key := fmt.Sprintf(ServiceSpecifier, pluginName, serviceName)
	filePath := filepath.Join(m.configDir, ServiceDir, pluginName+"_"+serviceName+CONFIG_EXTENSION)

	// Register the service config struct
	if err := m.Manager.RegisterStruct(key, cfg); err != nil {
		return fmt.Errorf("failed to register service config: %w", err)
	}

	// Register namespace for this service
	fsSource := source.NewFileSource(filePath)
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
	key := fmt.Sprintf(ServiceSpecifier, pluginName, serviceName)

	_, cfg, err := m.Manager.Get(key)

	if err != nil {
		return nil
	}

	return cfg.(ServiceConfig)
}

func (m *ManagerDefault) GetProtocol(pluginName string) ProtocolConfig {
	key := fmt.Sprintf(ProtocolSpecifier, pluginName)
	_, cfg, err := m.Manager.Get(key)

	if err != nil {
		if m.logger != nil {
			m.logger.Error("failed to get protocol config",
				zap.String("plugin", pluginName),
				zap.Error(err),
			)
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
			m.logger.Error("failed to get API config",
				zap.String("plugin", pluginName),
				zap.Error(err),
			)
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
