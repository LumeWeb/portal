package config

import (
	"fmt"
	"github.com/knadh/koanf/v2"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fatih/structs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
	"go.lumeweb.com/configmanager"
	"go.lumeweb.com/configmanager/source"
	"go.uber.org/zap"
)

// MockFileSystem for testing file operations
type MockFileSystem struct {
	Files         map[string][]byte
	Dirs          map[string]bool
	WriteError    error
	ReadError     error
	StatError     error
	MkdirAllError error
	OpenFileError error
	CreateError   error
	WritableFiles map[string]bool // Tracks which files are writable
}

func (m *MockFileSystem) Stat(name string) (os.FileInfo, error) {
	if m.StatError != nil {
		return nil, m.StatError
	}
	if _, ok := m.Files[name]; ok || m.Dirs[name] {
		return mockFileInfo{name: name, isDir: m.Dirs[name]}, nil
	}
	return nil, os.ErrNotExist
}

func (m *MockFileSystem) MkdirAll(path string, perm os.FileMode) error {
	if m.MkdirAllError != nil {
		return m.MkdirAllError
	}
	m.Dirs[path] = true
	return nil
}

func (m *MockFileSystem) OpenFile(name string, flag int, perm os.FileMode) (*os.File, error) {
	if m.OpenFileError != nil {
		return nil, m.OpenFileError
	}
	if flag&os.O_WRONLY != 0 {
		if writable, exists := m.WritableFiles[name]; exists && !writable {
			return nil, os.ErrPermission // Simulate non-writable file
		}
	}
	if content, ok := m.Files[name]; ok {
		tmpfile, err := os.CreateTemp("", "testfile")
		if err != nil {
			return nil, err
		}
		if _, err := tmpfile.Write(content); err != nil {
			return nil, err
		}
		if err := tmpfile.Close(); err != nil {
			return nil, err
		}
		return os.OpenFile(tmpfile.Name(), flag, perm)
	}
	return nil, os.ErrNotExist
}

func (m *MockFileSystem) Create(name string) (*os.File, error) {
	if m.CreateError != nil {
		return nil, m.CreateError
	}
	if m.Files == nil {
		m.Files = make(map[string][]byte)
	}
	tmpfile, err := os.CreateTemp("", "testfile")
	if err != nil {
		return nil, err
	}
	m.Files[name] = []byte{} // Initialize empty file content
	return tmpfile, nil
}

type mockFileInfo struct {
	name  string
	size  int64
	isDir bool
}

func (m mockFileInfo) Name() string       { return m.name }
func (m mockFileInfo) Size() int64        { return m.size }
func (m mockFileInfo) Mode() os.FileMode  { return 0644 }
func (m mockFileInfo) ModTime() time.Time { return time.Now() }
func (m mockFileInfo) IsDir() bool        { return m.isDir }
func (m mockFileInfo) Sys() interface{}   { return nil }

func TestFindConfigFile(t *testing.T) {
	// Define test cases
	testCases := []struct {
		name          string
		options       findConfigFileOptions
		existingFiles map[string][]byte // Used when no custom fs provided
		fs            *MockFileSystem   // Optional custom filesystem configuration
		expectedPath  string
		expectedError string
	}{
		{
			name: "File exists in default path",
			options: findConfigFileOptions{
				Paths: []string{"/etc/lumeweb/portal"},
				fs:    &MockFileSystem{},
			},
			existingFiles: map[string][]byte{"/etc/lumeweb/portal/core.yaml": []byte("test")},
			expectedPath:  "/etc/lumeweb/portal/core.yaml",
		},
		{
			name: "File exists in home path",
			options: findConfigFileOptions{
				Paths: []string{"$HOME/.lumeweb/portal"},
				fs:    &MockFileSystem{},
			},
			existingFiles: map[string][]byte{filepath.Join(os.Getenv("HOME"), ".lumeweb/portal/core.yaml"): []byte("test")},
			expectedPath:  filepath.Join(os.Getenv("HOME"), ".lumeweb/portal/core.yaml"),
		},
		{
			name: "File exists in current directory",
			options: findConfigFileOptions{
				Paths: []string{"./"},
				fs:    &MockFileSystem{},
			},
			existingFiles: map[string][]byte{"core.yaml": []byte("test")},
			expectedPath:  "core.yaml",
		},
		{
			name: "No file exists",
			options: findConfigFileOptions{
				Paths: []string{"/etc/lumeweb/portal", "$HOME/.lumeweb/portal", "./"},
				fs:    &MockFileSystem{},
			},
			expectedError: "no valid config file found in paths: [/etc/lumeweb/portal $HOME/.lumeweb/portal ./]",
		},
		{
			name: "Create if missing",
			options: findConfigFileOptions{
				Paths:           []string{"/tmp"},
				CreateIfMissing: true,
				fs:              &MockFileSystem{},
			},
			expectedPath: "/tmp/core.yaml",
		},
		{
			name: "Check writable - not writable",
			options: findConfigFileOptions{
				Paths:         []string{"/tmp"},
				CheckWritable: true,
			},
			fs: &MockFileSystem{
				Files: map[string][]byte{
					"/tmp/core.yaml": []byte("test"),
				},
				Dirs: make(map[string]bool),
				WritableFiles: map[string]bool{
					"/tmp/core.yaml": false, // Explicitly mark file as not writable
				},
			},
			expectedError: "no valid config file found in paths: [/tmp]",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Use provided mock FS if specified, otherwise create basic one
			if tc.fs == nil {
				tc.options.fs = &MockFileSystem{
					Files: tc.existingFiles,
					Dirs:  make(map[string]bool),
				}
			} else {
				tc.options.fs = tc.fs
			}

			// Mock ConfigManager
			cm, err := configmanager.NewConfigManager(
				[]source.ConfigSource{source.NewEnvConfigSource(ENV_PREFIX, ENV_SEPARATOR)},
				configmanager.WithLogger(zap.NewNop()),
			)
			require.NoError(t, err)

			// Execute findConfigFile
			path, err := findConfigFile(tc.options, cm)

			// Verify results
			if tc.expectedError != "" {
				assert.Error(t, err)
				assert.EqualError(t, err, tc.expectedError)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedPath, path)
			}
		})
	}
}

func TestCreateDefaultConfig(t *testing.T) {
	// Setup mock file system
	fs := &MockFileSystem{
		Files: make(map[string][]byte),
		Dirs:  make(map[string]bool),
	}

	// Mock ConfigManager
	cm, err := configmanager.NewConfigManager(
		[]source.ConfigSource{source.NewEnvConfigSource(ENV_PREFIX, ENV_SEPARATOR)},
		configmanager.WithLogger(zap.NewNop()),
	)
	require.NoError(t, err)

	// Define test path
	testPath := "/tmp/test_config.yaml"

	// Execute createDefaultConfig
	err = createDefaultConfig(testPath, cm, fs)
	require.NoError(t, err)

	// Verify that the file was created in the mock file system
	_, ok := fs.Files[testPath]
	assert.True(t, ok, "Config file should be created")

	// Verify that the parent directory was created
	parentDir := filepath.Dir(testPath)
	assert.True(t, fs.Dirs[parentDir], "Parent directory should be created")
}

func TestManagerDefault_Init(t *testing.T) {
	// Setup mock file system
	fs := &MockFileSystem{
		Files: map[string][]byte{
			"/tmp/core.yaml": []byte{},
		},
		Dirs: make(map[string]bool),
	}

	// Mock ConfigManager
	cm, err := configmanager.NewConfigManager(
		[]source.ConfigSource{source.NewEnvConfigSource(ENV_PREFIX, ENV_SEPARATOR)},
		configmanager.WithLogger(zap.NewNop()),
	)
	require.NoError(t, err)

	// Create a mock command
	cmd := &cli.Command{}

	// Create and initialize ManagerDefault instance
	m := &ManagerDefault{
		Manager:    cm,
		logger:     zap.NewNop(),
		configFile: "/tmp/core.yaml",
		configDir:  "/tmp",
		cmd:        cmd,
		lock:       sync.RWMutex{},
		fs:         fs,
	}
	err = m.Init()
	require.NoError(t, err)

	// Verify that the plugin directories were created
	expectedDirs := []string{
		filepath.Join(m.configDir, ProtoDir),
		filepath.Join(m.configDir, APIDir),
		filepath.Join(m.configDir, ServiceDir),
	}
	for _, dir := range expectedDirs {
		assert.True(t, fs.Dirs[dir], fmt.Sprintf("Directory %s should be created", dir))
	}
}

func TestManagerDefault_ConfigureAndGet(t *testing.T) {
	tests := []struct {
		name         string
		configType   string
		pluginName   string
		serviceName  string
		newConfig    func() interface{}
		setTestValue func(source *source.MemoryConfigSource, pluginName, serviceName string)
		getKey       func(pluginName, serviceName string) string
		getFunc      func(m *ManagerDefault, name string) interface{}
	}{
		{
			name:       "Protocol",
			configType: "protocol",
			pluginName: "test_plugin",
			newConfig:  func() interface{} { return newTestProtocolConfig() },
			setTestValue: func(s *source.MemoryConfigSource, pluginName, _ string) {
				s.Set(fmt.Sprintf("plugin.%s.protocol.value", pluginName), "test")
			},
			getKey: func(pluginName, _ string) string {
				return fmt.Sprintf(ProtocolSpecifier, pluginName)
			},
			getFunc: func(m *ManagerDefault, pluginName string) interface{} {
				return m.GetProtocol(pluginName)
			},
		},
		{
			name:       "API",
			configType: "api",
			pluginName: "test_api",
			newConfig:  func() interface{} { return newTestAPIConfig() },
			setTestValue: func(s *source.MemoryConfigSource, pluginName, _ string) {
				s.Set(fmt.Sprintf("plugin.%s.api.value", pluginName), "test")
			},
			getKey: func(pluginName, _ string) string {
				return fmt.Sprintf(APISpecifier, pluginName)
			},
			getFunc: func(m *ManagerDefault, pluginName string) interface{} {
				return m.GetAPI(pluginName)
			},
		},
		{
			name:        "Service",
			configType:  "service",
			pluginName:  "test_plugin",
			serviceName: "test_service",
			newConfig:   func() interface{} { return newTestServiceConfig() },
			setTestValue: func(s *source.MemoryConfigSource, pluginName, serviceName string) {
				s.Set(fmt.Sprintf("plugin.%s.service.%s.value", pluginName, serviceName), "test")
			},
			getKey: func(pluginName, serviceName string) string {
				return fmt.Sprintf(ServiceSpecifier, pluginName, serviceName)
			},
			getFunc: func(m *ManagerDefault, pluginName string) interface{} {
				return m.GetService(pluginName, "test_service")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock file system with empty config file
			fs := newMockFS(map[string][]byte{
				"/tmp/core.yaml": []byte{},
			})

			// Create memory source for testing
			memSource := source.NewMemoryConfigSource(nil)

			// Create manager with memory source
			m := newTestManager(t, fs, func(m *ManagerDefault) {
				m.Manager.RegisterSource(memSource)
			})

			// Create test config
			config := tt.newConfig()

			// Configure the component
			var err error
			switch tt.configType {
			case "protocol":
				err = m.ConfigureProtocol(tt.pluginName, config.(ProtocolConfig))
			case "api":
				err = m.ConfigureAPI(tt.pluginName, config.(APIConfig))
			case "service":
				err = m.ConfigureService(tt.pluginName, tt.serviceName, config.(ServiceConfig))
			}
			require.NoError(t, err)

			// Initialize to load all configs
			err = m.Init()
			require.NoError(t, err)

			// Set test values
			tt.setTestValue(memSource, tt.pluginName, tt.serviceName)

			// Verify registration
			var key string
			if tt.configType == "service" {
				key = tt.getKey(tt.pluginName, tt.serviceName)
			} else {
				key = tt.getKey(tt.pluginName, "")
			}
			registeredStructs := m.Manager.GetRegisteredStructs()
			_, ok := registeredStructs[key]
			assert.True(t, ok, "%s config struct should be registered", tt.configType)

			// Verify namespace registration
			var expectedFilePath string
			switch tt.configType {
			case "protocol":
				expectedFilePath = filepath.Join(m.configDir, ProtoDir, tt.pluginName+CONFIG_EXTENSION)
			case "api":
				expectedFilePath = filepath.Join(m.configDir, APIDir, tt.pluginName+CONFIG_EXTENSION)
			case "service":
				expectedFilePath = filepath.Join(m.configDir, ServiceDir, tt.pluginName+"_"+tt.serviceName+CONFIG_EXTENSION)
			}
			_, fileExists := fs.Files[expectedFilePath]
			assert.False(t, fileExists, "File source should be created")

			// Verify config was set
			var val interface{}
			switch tt.configType {
			case "protocol":
				var pc MockProtocolConfig
				_, _, err = m.Manager.Get(key, &pc)
				val = pc
			case "api":
				var ac MockAPIConfig
				_, _, err = m.Manager.Get(key, &ac)
				val = ac
			case "service":
				var sc MockServiceConfig
				_, _, err = m.Manager.Get(key, &sc)
				val = sc
			}
			require.NoError(t, err)
			require.NotNil(t, val)
			assert.Equal(t, "test", reflect.ValueOf(val).FieldByName("Value").String())

			// Test retrieval - pass appropriate name based on config type
			var retrieved interface{}
			retrieved = tt.getFunc(m, tt.pluginName)
			assert.Equal(t, config, retrieved, "%s should be retrieved correctly", strings.ToUpper(tt.configType))
		})
	}
}

// Mock configurations for testing
type MockProtocolConfig struct {
	Value string `config:"value"`
}

func (m *MockProtocolConfig) Defaults() map[string]any {
	return map[string]any{"value": "default"}
}

type MockAPIConfig struct {
	Value string `config:"value"`
}

func (m *MockAPIConfig) Defaults() map[string]any {
	return map[string]any{"value": "default"}
}

type MockServiceConfig struct {
	Value string `config:"value"`
}

func (m *MockServiceConfig) Defaults() map[string]any {
	return map[string]any{"value": "default"}
}

// Helper function to check if two maps are equal
func mapsAreEqual(map1, map2 map[string]interface{}) bool {
	if len(map1) != len(map2) {
		return false
	}
	for key, val1 := range map1 {
		val2, ok := map2[key]
		if !ok {
			return false
		}

		if reflect.DeepEqual(val1, val2) {
			continue
		}

		return false
	}
	return true
}

// Helper function to create a temporary directory
func createTempDir(t *testing.T) string {
	tempDir, err := os.MkdirTemp("", "config_test")
	require.NoError(t, err)
	return tempDir
}

// Helper function to remove a temporary directory
func removeTempDir(t *testing.T, dir string) {
	err := os.RemoveAll(dir)
	require.NoError(t, err)
}

// Helper function to create a temporary file
func createTempFile(t *testing.T, dir, content string) string {
	tmpFile, err := os.CreateTemp(dir, "config_test")
	require.NoError(t, err)

	_, err = tmpFile.WriteString(content)
	require.NoError(t, err)

	err = tmpFile.Close()
	require.NoError(t, err)

	return tmpFile.Name()
}

// Helper to create a test ManagerDefault instance
func newTestManager(t *testing.T, fs fileSystem, opts ...ManagerOption) *ManagerDefault {
	// Ensure fs is not nil
	if fs == nil {
		fs = newMockFS(map[string][]byte{})
	}

	// Start with basic options
	baseOpts := []ManagerOption{
		withFileSystem(fs),
		WithLogger(zap.NewNop()),
	}

	// Add any additional options
	baseOpts = append(baseOpts, opts...)

	m, err := NewManager(&cli.Command{}, baseOpts...)
	require.NoError(t, err)

	return m
}

// Helper to create a mock file system with initial state
func newMockFS(files map[string][]byte) *MockFileSystem {
	return &MockFileSystem{
		Files: files,
		Dirs:  make(map[string]bool),
	}
}

// Helper to create a test protocol config
func newTestProtocolConfig() *MockProtocolConfig {
	return &MockProtocolConfig{Value: "test"}
}

// Helper to create a test API config
func newTestAPIConfig() *MockAPIConfig {
	return &MockAPIConfig{Value: "test"}
}

// Helper to create a test service config
func newTestServiceConfig() *MockServiceConfig {
	return &MockServiceConfig{Value: "test"}
}

func TestDatabaseConfigValidation(t *testing.T) {
	testCases := []struct {
		name          string
		config        DatabaseConfig
		expectedError string
		expectedPath  string
	}{
		{
			name: "SQLite - Missing File",
			config: DatabaseConfig{
				Type: "sqlite",
			},
			expectedError: "core.db.file is required for sqlite",
		},
		{
			name: "MySQL - Missing Host",
			config: DatabaseConfig{
				Type:     "mysql",
				Port:     3306,
				Username: "user",
				Password: "password",
				Name:     "db",
			},
			expectedError: "core.db.host is required for mysql",
		},
		{
			name: "MySQL - Invalid Port",
			config: DatabaseConfig{
				Type:     "mysql",
				Host:     "localhost",
				Port:     0,
				Username: "user",
				Password: "password",
				Name:     "db",
			},
			expectedError: "core.db.port must be greater than 0",
		},
		{
			name: "MySQL - Missing Username",
			config: DatabaseConfig{
				Type:     "mysql",
				Host:     "localhost",
				Port:     3306,
				Password: "password",
				Name:     "db",
			},
			expectedError: "core.db.username is required for mysql",
		},
		{
			name: "MySQL - Missing Password",
			config: DatabaseConfig{
				Type:     "mysql",
				Host:     "localhost",
				Port:     3306,
				Username: "user",
				Name:     "db",
			},
			expectedError: "core.db.password is required for mysql",
		},
		{
			name: "MySQL - Missing Name",
			config: DatabaseConfig{
				Type:     "mysql",
				Host:     "localhost",
				Port:     3306,
				Username: "user",
				Password: "password",
			},
			expectedError: "core.db.name is required for mysql",
			expectedPath:  "db.Name",
		},
		{
			name: "Valid SQLite Config",
			config: DatabaseConfig{
				Type: "sqlite",
				File: "portal.db",
			},
		},
		{
			name: "Valid MySQL Config",
			config: DatabaseConfig{
				Type:     "mysql",
				Host:     "localhost",
				Port:     3306,
				Username: "user",
				Password: "password",
				Name:     "db",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cm, err := configmanager.NewConfigManager(
				[]source.ConfigSource{},
				configmanager.WithLogger(zap.NewNop()),
			)
			require.NoError(t, err)

			err = cm.RegisterStruct("db", DatabaseConfig{})
			require.NoError(t, err)

			// Convert struct to map using structs
			s := structs.New(&tc.config)
			s.TagName = "config"

			mapData := s.Map()

			tmpK := koanf.New(".")

			err = tmpK.Set("db", mapData)
			require.NoError(t, err)

			memSource := source.NewMemoryConfigSource(tmpK.All())
			cm.RegisterSource(memSource)

			err = cm.Load()

			if tc.expectedError != "" {
				assert.Error(t, err)
				// The error message includes the full validation path
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
