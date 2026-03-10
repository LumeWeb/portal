package config

import (
	"fmt"
	"github.com/knadh/koanf/v2"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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

// conditionalMkdirAllFS is a mock filesystem that fails MkdirAll on specific paths
type conditionalMkdirAllFS struct {
	Files    map[string][]byte
	Dirs     map[string]bool
	failPath string // Path on which MkdirAll should fail
}

func (m *conditionalMkdirAllFS) Stat(name string) (os.FileInfo, error) {
	if _, ok := m.Files[name]; ok || m.Dirs[name] {
		return mockFileInfo{name: name, isDir: m.Dirs[name]}, nil
	}
	return nil, os.ErrNotExist
}

func (m *conditionalMkdirAllFS) MkdirAll(path string, perm os.FileMode) error {
	// Check if the path contains the failPath
	if m.failPath != "" && strings.HasPrefix(path, m.failPath) {
		return os.ErrPermission
	}
	m.Dirs[path] = true
	return nil
}

func (m *conditionalMkdirAllFS) OpenFile(name string, flag int, perm os.FileMode) (*os.File, error) {
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

func (m *conditionalMkdirAllFS) Create(name string) (*os.File, error) {
	if m.Files == nil {
		m.Files = make(map[string][]byte)
	}
	tmpfile, err := os.CreateTemp("", "testfile")
	if err != nil {
		return nil, err
	}
	m.Files[name] = []byte{}
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
	type testCase struct {
		name          string
		options       findConfigFileOptions
		setup         func(h *testHelper)
		expectedPath  string
		expectedError string
	}

	testCases := []testCase{
		{
			name: "File exists in default path",
			options: findConfigFileOptions{
				Paths: []string{"/etc/lumeweb/portal"},
				Core:  true,
			},
			setup: func(h *testHelper) {
				h.withMockFS(map[string][]byte{"/etc/lumeweb/portal/core.yaml": []byte("test")})
			},
			expectedPath: "/etc/lumeweb/portal/core.yaml",
		},
		{
			name: "File exists in home path",
			options: findConfigFileOptions{
				Paths: []string{"$HOME/.lumeweb/portal"},
				Core:  true,
			},
			setup: func(h *testHelper) {
				h.withMockFS(map[string][]byte{
					filepath.Join(os.Getenv("HOME"), ".lumeweb/portal/core.yaml"): []byte("test"),
				})
			},
			expectedPath: filepath.Join(os.Getenv("HOME"), ".lumeweb/portal/core.yaml"),
		},
		{
			name: "Create fails on first path with permission, succeeds on second",
			options: findConfigFileOptions{
				Paths:           []string{"/etc/lumeweb/portal", "/tmp"},
				CreateIfMissing: true,
				FS:              nil,
				Core:            true,
			},
			setup: func(h *testHelper) {
				h.fs = &conditionalMkdirAllFS{
					Files:    make(map[string][]byte),
					Dirs:     make(map[string]bool),
					failPath: "/etc/lumeweb/portal",
				}
			},
			expectedPath: "/tmp/core.yaml",
		},
		{
			name: "File exists in current directory",
			options: findConfigFileOptions{
				Paths: []string{"./"},
				Core:  true,
			},
			setup: func(h *testHelper) {
				h.withMockFS(map[string][]byte{"core.yaml": []byte("test")})
			},
			expectedPath: "core.yaml",
		},
		{
			name: "No file exists",
			options: findConfigFileOptions{
				Paths: []string{"/etc/lumeweb/portal", "$HOME/.lumeweb/portal", "./"},
				FS:    &MockFileSystem{},
				Core:  true,
			},
			expectedError: "no valid config file found in paths: [/etc/lumeweb/portal $HOME/.lumeweb/portal ./]",
		},
		{
			name: "Create if missing",
			options: findConfigFileOptions{
				Paths:           []string{"/tmp"},
				CreateIfMissing: true,
				FS:              &MockFileSystem{},
				Core:            true,
			},
			expectedPath: "/tmp/core.yaml",
		},
		{
			name: "Check writable - not writable",
			options: findConfigFileOptions{
				Paths:         []string{"/tmp"},
				CheckWritable: true,
				Core:          true,
			},
			setup: func(h *testHelper) {
				h.fs = &MockFileSystem{
					Files: map[string][]byte{
						"/tmp/core.yaml": []byte("test"),
					},
					Dirs: make(map[string]bool),
					WritableFiles: map[string]bool{
						"/tmp/core.yaml": false, // Explicitly mark file as not writable
					},
				}
			},
			expectedError: "no valid config file found in paths: [/tmp]",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			h := newTestHelper(t)
			if tc.setup != nil {
				tc.setup(h)
			}

			tc.options.FS = h.fs

			cm, err := configmanager.NewConfigManager(
				[]source.ConfigSource{source.NewEnvConfigSource(ENV_PREFIX, ENV_SEPARATOR)},
				configmanager.WithLogger(zap.NewNop()),
			)
			require.NoError(t, err)

			path, err := findConfigFile(tc.options, cm)

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

	// Mock ConfigManager (not used in this test but kept for consistency)
	_, err := configmanager.NewConfigManager(
		[]source.ConfigSource{source.NewEnvConfigSource(ENV_PREFIX, ENV_SEPARATOR)},
		configmanager.WithLogger(zap.NewNop()),
	)
	require.NoError(t, err)

	// Define test path
	testPath := "/tmp/test_config.yaml"

	// Execute CreateDefaultConfig
	err = CreateDefaultConfig(testPath, fs)
	require.NoError(t, err)

	// Verify that the file was created in the mock file system
	_, ok := fs.Files[testPath]
	assert.True(t, ok, "GetConfig file should be created")

	// Verify that the parent directory was created
	parentDir := filepath.Dir(testPath)
	assert.True(t, fs.Dirs[parentDir], "Parent directory should be created")
}

func TestManagerDefault_Init(t *testing.T) {
	// Create temp dir for test
	tempDir, err := os.MkdirTemp("", "config_test")
	require.NoError(t, err)
	defer func(path string) {
		err = os.RemoveAll(path)
		if err != nil {
			require.NoError(t, err)
		}
	}(tempDir)

	// Create test config file
	configFile := filepath.Join(tempDir, CoreConfigFile)
	err = os.WriteFile(configFile, []byte(`
domain: "example.com"
portal_name: "test_portal"
`), 0644)
	require.NoError(t, err)

	// Create manager with real filesystem
	m, err := NewManager(WithConfigPaths([]string{tempDir}))
	require.NoError(t, err)

	// Configure a test plugin first so directories will be created on Init
	err = m.ConfigureProtocol("test_plugin", newTestProtocolConfig())
	require.NoError(t, err)

	// Initialize
	err = m.Init()
	require.NoError(t, err)

	// Verify directories were created
	expectedDirs := []string{
		filepath.Join(m.configDir, PluginsDir, "test_plugin", ProtoDir),
		filepath.Join(m.configDir, PluginsDir, "test_plugin", APIDir),
		filepath.Join(m.configDir, PluginsDir, "test_plugin", ServiceDir),
	}
	for _, dir := range expectedDirs {
		_, err := os.Stat(dir)
		assert.NoError(t, err, fmt.Sprintf("Directory %s should exist", dir))
	}
}

func TestManagerDefault_ConfigureAndGet(t *testing.T) {
	// Create temp dir for test
	tempDir, err := os.MkdirTemp("", "config_test")
	require.NoError(t, err)
	defer func(path string) {
		err = os.RemoveAll(path)
		if err != nil {
			require.NoError(t, err)
		}
	}(tempDir)

	// Create core config file with required fields
	coreConfigPath := filepath.Join(tempDir, CoreConfigFile)
	err = os.WriteFile(coreConfigPath, []byte(`
domain: test.com
portal_name: test_portal
`), 0644)
	require.NoError(t, err)

	tests := []struct {
		name         string
		configType   string
		pluginName   string
		serviceName  string
		newConfig    func() interface{}
		setTestValue func(pluginName, serviceName string) string
		getKey       func(pluginName, serviceName string) string
		getFunc      func(m *ManagerDefault, name string) interface{}
	}{
		{
			name:       "Protocol",
			configType: "protocol",
			pluginName: "test_plugin",
			newConfig:  func() interface{} { return newTestProtocolConfig() },
			setTestValue: func(pluginName, _ string) string {
				return "value: test"
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
			setTestValue: func(pluginName, _ string) string {
				return "value: test"
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
			setTestValue: func(pluginName, serviceName string) string {
				return "value: test"
			},
			getKey: func(pluginName, serviceName string) string {
				// Strip plugin name prefix to match the new behavior
				cleanServiceName := StripPluginNamePrefix(pluginName, serviceName)
				return fmt.Sprintf(ServiceSpecifier, pluginName, cleanServiceName)
			},
			getFunc: func(m *ManagerDefault, pluginName string) interface{} {
				return m.GetService(pluginName, "test_service")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create manager with real filesystem
			m, err := NewManager(WithConfigPaths([]string{tempDir}))
			require.NoError(t, err)

			// Create test config
			config := tt.newConfig()

			// Create config file with test values before Configure calls
			configFile := ""
			switch tt.configType {
			case "protocol":
				configFile = filepath.Join(tempDir, PluginsDir, tt.pluginName, ProtoDir, SectionConfigFile)
			case "api":
				configFile = filepath.Join(tempDir, PluginsDir, tt.pluginName, APIDir, SectionConfigFile)
			case "service":
				configFile = filepath.Join(tempDir, PluginsDir, tt.pluginName, ServiceDir, tt.serviceName+CONFIG_EXTENSION)
			}

			// Write test config
			err = os.MkdirAll(filepath.Dir(configFile), 0755)
			require.NoError(t, err)
			err = os.WriteFile(configFile, []byte(tt.setTestValue(tt.pluginName, tt.serviceName)), 0644)
			require.NoError(t, err)

			// Configure the component
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

			// Test retrieval
			retrieved := tt.getFunc(m, tt.pluginName)
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

type testHelper struct {
	t      *testing.T
	fs     FileSystem
	cmd    *cli.Command
	logger *zap.Logger
}

func newTestHelper(t *testing.T) *testHelper {
	return &testHelper{
		t:      t,
		fs:     newMockFS(nil),
		cmd:    &cli.Command{},
		logger: zap.NewNop(),
	}
}

func (h *testHelper) newManager(opts ...ManagerOption) *ManagerDefault {
	baseOpts := []ManagerOption{
		withFileSystem(h.fs),
		WithLogger(h.logger),
	}
	baseOpts = append(baseOpts, opts...)

	m, err := NewManager(baseOpts...)
	require.NoError(h.t, err)
	return m
}

func (h *testHelper) newTestManager(configFile string, opts ...ManagerOption) *ManagerDefault {
	// Create a manager with the config file path explicitly set
	m := h.newManager(append(opts, WithConfigPaths([]string{filepath.Dir(configFile)}))...)

	return m
}

func (h *testHelper) withMockFS(files map[string][]byte) *testHelper {
	h.fs = newMockFS(files)
	return h
}

func (h *testHelper) withLogger(logger *zap.Logger) *testHelper {
	h.logger = logger
	return h
}

func newMockFS(files map[string][]byte) *MockFileSystem {
	if files == nil {
		files = make(map[string][]byte)
	}
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
		errorContains []string
		expectedPath  string
	}{
		{
			name: "SQLite - Missing File",
			config: DatabaseConfig{
				Type: "sqlite",
			},
			expectedError: "validation failed for struct db: configuration validation failed for db:",
			errorContains: []string{
				"db.File:", // Check that File field is mentioned
				"db: struct is invalid",
			},
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
			expectedError: "validation failed for struct db: configuration validation failed for db:",
			errorContains: []string{
				"db.Host:", // Check that Host field is mentioned
				"db: struct is invalid",
			},
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
			expectedError: "validation failed for struct db: configuration validation failed for db:",
			errorContains: []string{
				"db.Port:", // Check that Port field is mentioned
				"db: struct is invalid",
			},
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
			expectedError: "validation failed for struct db: configuration validation failed for db:",
			errorContains: []string{
				"db.Username:", // Check that Username field is mentioned
				"db: struct is invalid",
			},
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
			expectedError: "validation failed for struct db: configuration validation failed for db:",
			errorContains: []string{
				"db.Password:", // Check that Password field is mentioned
				"db: struct is invalid",
			},
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
			expectedError: "validation failed for struct db: configuration validation failed for db:",
			errorContains: []string{
				"db.Name:", // Check that Name field is mentioned
				"db: struct is invalid",
			},
		},
		{
			name: "Valid SQLite GetConfig",
			config: DatabaseConfig{
				Type: "sqlite",
				File: "portal.db",
			},
		},
		{
			name: "Valid MySQL GetConfig",
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
				if tc.errorContains != nil {
					for _, s := range tc.errorContains {
						assert.Contains(t, err.Error(), s)
					}
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
