package core_tests

import (
	"context"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/portal/build"
	coreTesting "go.lumeweb.com/portal/core/testing"
	coreMocks "go.lumeweb.com/portal/core/testing/mocks"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/db/models"
	"go.uber.org/zap/zapcore"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/testing/mocks"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// FatalLogInterceptor is a helper for testing that intercepts fatal log calls.
type FatalLogInterceptor struct {
	fatalCalled bool
}

// NewFatalLogInterceptor creates a new instance of FatalLogInterceptor.
func NewFatalLogInterceptor() *FatalLogInterceptor {
	return &FatalLogInterceptor{}
}

// OnWrite implements the zapcore.CheckWriteHook interface.
// It is called by zap when a fatal log entry is written.
func (i *FatalLogInterceptor) OnWrite(_ *zapcore.CheckedEntry, _ []zap.Field) {
	i.fatalCalled = true
}

// FatalWasCalled returns true if a fatal log was intercepted.
func (i *FatalLogInterceptor) FatalWasCalled() bool {
	return i.fatalCalled
}

// Hook returns a zap.Option that can be used to configure a logger
// to use this interceptor as its fatal hook.
func (i *FatalLogInterceptor) Hook() zap.Option {
	return zap.WithFatalHook(i)
}

func TestNewContext(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)

	ctx, err := core.NewContext(mockConfigManager, mockLogger)
	assert.NoError(t, err)
	assert.NotNil(t, ctx)
	assert.NotNil(t, ctx.Config())
	assert.NotNil(t, ctx.Logger())
	assert.NotNil(t, ctx.Event())
	assert.NotNil(t, ctx.GetContext())
	assert.Equal(t, 0, ctx.ExitCode())
	assert.NotEmpty(t, ctx.StartupFuncs()) // Logger and other subsystems may register startup funcs
	assert.Len(t, ctx.ExitFuncs(), 2)      // Event manager + telemetry shutdown

	mockConfigManager.AssertExpectations(t)
}

func TestContextWithService(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)
	mockService := mocks.NewMockService(t)

	ctx, err := core.NewContext(mockConfigManager, mockLogger, core.ContextWithService("test-service", mockService))
	assert.NoError(t, err)
	assert.NotNil(t, ctx)

	retrievedService := ctx.Service("test-service")
	assert.NotNil(t, retrievedService)
	assert.Equal(t, mockService, retrievedService)

	mockConfigManager.AssertExpectations(t)
}

func TestContextWithStartupFunc(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)

	startupCalled := false
	startupFunc := func(ctx core.Context) error {
		startupCalled = true
		return nil
	}

	ctx, err := core.NewContext(mockConfigManager, mockLogger, core.ContextWithStartupFunc(startupFunc))
	assert.NoError(t, err)
	assert.NotNil(t, ctx)

	startupFuncs := ctx.StartupFuncs()
	assert.GreaterOrEqual(t, len(startupFuncs), 1)

	// Manually call the startup function for testing
	err = startupFuncs[0](ctx)
	assert.NoError(t, err)
	assert.True(t, startupCalled)

	mockConfigManager.AssertExpectations(t)
}

func TestContextWithExitFunc(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)

	exitCalled := false
	exitFunc := func(ctx core.Context) error {
		exitCalled = true
		return nil
	}

	ctx, err := core.NewContext(mockConfigManager, mockLogger, core.ContextWithExitFunc(exitFunc))
	assert.NoError(t, err)
	assert.NotNil(t, ctx)

	exitFuncs := ctx.ExitFuncs()
	assert.Len(t, exitFuncs, 3) // Custom exit + event manager + telemetry shutdown

	// Manually call all exit functions for testing
	for _, f := range exitFuncs {
		err = f(ctx)
		assert.NoError(t, err)
	}

	assert.True(t, exitCalled, "exit function should have been called")
	mockConfigManager.AssertExpectations(t)
}

func TestContextWithEvents(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)

	// Create test context with event recorder
	ctx, err := core.NewContext(mockConfigManager, mockLogger)
	assert.NoError(t, err)
	assert.NotNil(t, ctx)

	// Setup event recorder
	recorder := coreTesting.NewEventRecorder()
	recorder.Listen(ctx, "test-event")

	// Reset events to ensure clean slate
	ctx.ResetEvents()

	// Add listener back
	recorder.Listen(ctx, "test-event")

	// Fire test event
	err = ctx.Fire("test-event", nil)
	assert.NoError(t, err)

	// Verify event was recorded
	assert.True(t, recorder.HasEvent("test-event"))

	mockConfigManager.AssertExpectations(t)
}

func setupTestDB(t *testing.T) (core.Context, *gorm.DB) {
	// Create temp dir for test DB
	tmpDir := t.TempDir()
	dbFile := filepath.Join(tmpDir, "test.db")

	// Create mock config with SQLite DB path
	mockConfigManager := newMockConfigManager(t, &config.Config{
		Core: config.CoreConfig{
			DB: config.DatabaseConfig{
				Type: "sqlite",
				File: dbFile,
			},
		},
	})

	mockLogger := core.NewLogger(mockConfigManager, nil)

	// Create context with real DB
	ctx, err := core.NewContext(mockConfigManager, mockLogger)
	require.NoError(t, err)
	require.NotNil(t, ctx)

	// Setup real database connection
	_db, ctxOpts := db.NewDatabase(ctx)
	require.NotNil(t, _db)

	// Apply DB context options using ProcessCtxOptions
	newCtx, err := core.ProcessCtxOptions(ctx, ctxOpts...)
	require.NoError(t, err)
	require.NotNil(t, newCtx)

	return newCtx, newCtx.DB()
}

func TestContextWithDB(t *testing.T) {
	core.ResetState()

	ctx, _db := setupTestDB(t)
	assert.NotNil(t, _db)

	// Test DB functionality by creating a table
	err := _db.AutoMigrate(&models.User{})
	assert.NoError(t, err)

	// Verify table was created
	var tables []string
	err = _db.Raw("SELECT name FROM sqlite_master WHERE type='table'").Scan(&tables).Error
	assert.NoError(t, err)
	assert.Contains(t, tables, "users")

	// Clean up
	for _, exitFunc := range ctx.ExitFuncs() {
		err := exitFunc(ctx)
		assert.NoError(t, err)
	}
}

func TestProcessCtxOptions(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)
	mockService1 := mocks.NewMockService(t)
	mockService2 := mocks.NewMockService(t)

	ctx, err := core.NewContext(mockConfigManager, mockLogger,
		core.ContextWithService("svc1", mockService1),
		core.ContextWithService("svc2", mockService2),
	)
	assert.NoError(t, err)
	assert.NotNil(t, ctx)

	assert.NotNil(t, ctx.Service("svc1"))
	assert.NotNil(t, ctx.Service("svc2"))

	mockConfigManager.AssertExpectations(t)
}

func TestService(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)
	mockService := mocks.NewMockService(t)

	ctx, err := core.NewContext(mockConfigManager, mockLogger, core.ContextWithService("test-service", mockService))
	assert.NoError(t, err)

	retrievedService := ctx.Service("test-service")
	assert.NotNil(t, retrievedService)
	assert.Equal(t, mockService, retrievedService)

	mockConfigManager.AssertExpectations(t)
}

func TestService_NotFound(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)

	ctx, err := core.NewContext(mockConfigManager, mockLogger)
	assert.NoError(t, err)

	retrievedService := ctx.Service("non-existent-service")
	assert.Nil(t, retrievedService)

	mockConfigManager.AssertExpectations(t)
}

func TestOnStartup(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)

	ctx, err := core.NewContext(mockConfigManager, mockLogger)
	assert.NoError(t, err)

	startupCalled := false
	startupFunc := func(ctx core.Context) error {
		startupCalled = true
		return nil
	}

	ctx.OnStartup(startupFunc)
	startupFuncs := ctx.StartupFuncs()
	assert.GreaterOrEqual(t, len(startupFuncs), 1)

	// Call all startup functions — system may register its own
	for _, f := range startupFuncs {
		err = f(ctx)
		assert.NoError(t, err)
	}
	assert.True(t, startupCalled)

	mockConfigManager.AssertExpectations(t)
}

func TestOnExit(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)

	ctx, err := core.NewContext(mockConfigManager, mockLogger)
	assert.NoError(t, err)

	exitCalled := false
	exitFunc := func(ctx core.Context) error {
		exitCalled = true
		return nil
	}

	ctx.OnExit(exitFunc)
	exitFuncs := ctx.ExitFuncs()
	assert.Len(t, exitFuncs, 3) // Custom exit + event manager + telemetry shutdown

	// Manually call the exit function for testing
	for _, f := range exitFuncs {
		err = f(ctx)
		assert.NoError(t, err)
	}
	assert.True(t, exitCalled)

	mockConfigManager.AssertExpectations(t)
}

func TestDB(t *testing.T) {
	core.ResetState()

	// Test with mock DB
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)
	mockDB := &gorm.DB{}

	_, err := core.NewContext(mockConfigManager, mockLogger, core.ContextWithDB(mockDB))
	assert.NoError(t, err)
	mockConfigManager.AssertExpectations(t)

	// Test with real DB - only test basic DB access since setupTestDB already tests migrations
	realCtx, realDB := setupTestDB(t)
	assert.NotNil(t, realDB)
	assert.NotNil(t, realCtx.DB())

	// Clean up
	for _, exitFunc := range realCtx.ExitFuncs() {
		err := exitFunc(realCtx)
		assert.NoError(t, err)
	}
}

func TestLogger(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)

	ctx, err := core.NewContext(mockConfigManager, mockLogger)
	assert.NoError(t, err)

	assert.Equal(t, mockLogger, ctx.Logger())

	mockConfigManager.AssertExpectations(t)
}

func TestProtocolLogger(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)
	mockProtocol := mocks.NewMockProtocol(t)
	mockProtocol.On("Name").Return("test-protocol")

	ctx, err := core.NewContext(mockConfigManager, mockLogger)
	assert.NoError(t, err)

	protoLogger := ctx.ProtocolLogger(mockProtocol)
	assert.NotNil(t, protoLogger)
	// Further checks could involve inspecting the logger's name, but that's implementation detail

	mockConfigManager.AssertExpectations(t)
}

func TestAPILogger(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)
	mockAPI := mocks.NewMockAPI(t)
	mockAPI.On("Name").Return("test-api")

	ctx, err := core.NewContext(mockConfigManager, mockLogger)
	assert.NoError(t, err)

	apiLogger := ctx.APILogger(mockAPI)
	assert.NotNil(t, apiLogger)
	// Further checks could involve inspecting the logger's name, but that's implementation detail

	mockConfigManager.AssertExpectations(t)
}

func TestServiceLogger(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)
	mockService := mocks.NewMockService(t)
	mockService.On("ID").Return("test-service")

	ctx, err := core.NewContext(mockConfigManager, mockLogger)
	assert.NoError(t, err)

	serviceLogger := ctx.ServiceLogger(mockService)
	assert.NotNil(t, serviceLogger)
	// Further checks could involve inspecting the logger's name, but that's implementation detail

	mockConfigManager.AssertExpectations(t)
}

func TestNamedLogger(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)

	ctx, err := core.NewContext(mockConfigManager, mockLogger)
	assert.NoError(t, err)

	namedLogger := ctx.NamedLogger("my-name")
	assert.NotNil(t, namedLogger)
	// Further checks could involve inspecting the logger's name, but that's implementation detail

	mockConfigManager.AssertExpectations(t)
}

func TestWithLoggerOptions(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)

	ctx, err := core.NewContext(mockConfigManager, mockLogger)
	assert.NoError(t, err)

	loggerWithOptions := ctx.WithLoggerOptions(zap.Fields(zap.String("key", "value")))
	assert.NotNil(t, loggerWithOptions)
	// Cannot easily verify options applied without inspecting internal logger state

	mockConfigManager.AssertExpectations(t)
}

func TestWithLoggerLazy(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)

	ctx, err := core.NewContext(mockConfigManager, mockLogger)
	assert.NoError(t, err)

	loggerWithLazy := ctx.WithLoggerLazy(zap.String("key", "value"))
	assert.NotNil(t, loggerWithLazy)
	// Cannot easily verify fields applied without inspecting internal logger state

	mockConfigManager.AssertExpectations(t)
}

func TestWithLogger(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)

	ctx, err := core.NewContext(mockConfigManager, mockLogger)
	assert.NoError(t, err)

	loggerWithFields := ctx.WithLogger(zap.String("key", "value"))
	assert.NotNil(t, loggerWithFields)
	// Cannot easily verify fields applied without inspecting internal logger state

	mockConfigManager.AssertExpectations(t)
}

func TestConfig(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)

	ctx, err := core.NewContext(mockConfigManager, mockLogger)
	assert.NoError(t, err)

	assert.Equal(t, mockConfigManager, ctx.Config())

	mockConfigManager.AssertExpectations(t)
}

func TestCancel(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)

	ctx, err := core.NewContext(mockConfigManager, mockLogger)
	assert.NoError(t, err)

	ctx.Cancel()

	select {
	case <-ctx.Done():
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Fatal("context was not cancelled")
	}
	assert.ErrorIs(t, ctx.Err(), context.Canceled)

	mockConfigManager.AssertExpectations(t)
}

func TestExitCode(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)

	ctx, err := core.NewContext(mockConfigManager, mockLogger)
	assert.NoError(t, err)

	assert.Equal(t, 0, ctx.ExitCode())

	mockConfigManager.AssertExpectations(t)
}

func TestSetExitCode(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)

	ctx, err := core.NewContext(mockConfigManager, mockLogger)
	assert.NoError(t, err)

	ctx.SetExitCode(1)
	assert.Equal(t, 1, ctx.ExitCode())

	mockConfigManager.AssertExpectations(t)
}

func TestEvent(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)

	ctx, err := core.NewContext(mockConfigManager, mockLogger)
	assert.NoError(t, err)

	assert.NotNil(t, ctx.Event())

	mockConfigManager.AssertExpectations(t)
}

func TestValue(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)

	// Create a context with a value
	baseCtx := context.WithValue(context.Background(), "testKey", "testValue")
	ctx, err := core.NewContext(mockConfigManager, mockLogger)
	assert.NoError(t, err)

	// Manually set the base context with value (this is not how it's intended to be used,
	// but necessary for testing the Value method on the defaultContext struct)
	defaultCtx, ok := ctx.(*core.DefaultContext)
	assert.True(t, ok)
	defaultCtx.Context = baseCtx

	value := ctx.Value("testKey")
	assert.Equal(t, "testValue", value)

	nonExistentValue := ctx.Value("nonExistentKey")
	assert.Nil(t, nonExistentValue)

	mockConfigManager.AssertExpectations(t)
}

func TestGetService(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)
	mockService := mocks.NewMockService(t)

	ctx, err := core.NewContext(mockConfigManager, mockLogger, core.ContextWithService("test-service", mockService))
	assert.NoError(t, err)

	retrievedService := core.GetService[core.Service](ctx, "test-service")
	assert.NotNil(t, retrievedService)
	assert.Equal(t, mockService, retrievedService)

	mockConfigManager.AssertExpectations(t)
}

func TestGetService_NotFound(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)

	flogger := NewFatalLogInterceptor()

	ctx, err := core.NewContext(mockConfigManager, mockLogger, core.ContextWithLoggerOptions(flogger.Hook()))
	assert.NoError(t, err)

	core.GetService[core.Service](ctx, "non-existent-service")
	assert.True(t, flogger.FatalWasCalled(), "Expected fatal log")

	mockConfigManager.AssertExpectations(t)
}

func TestGetService_TypeMismatch(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)
	mockService := mocks.NewMockService(t) // MockService implements core.Service

	flogger := NewFatalLogInterceptor()

	ctx, err := core.NewContext(mockConfigManager, mockLogger, core.ContextWithService("test-service", mockService), core.ContextWithLoggerOptions(flogger.Hook()))
	assert.NoError(t, err)

	core.GetService[*mocks.MockAuthService](ctx, "test-service")
	assert.True(t, flogger.FatalWasCalled(), "Expected fatal log for type mismatch")

	mockConfigManager.AssertExpectations(t)
}

func TestGetServiceConfig(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)

	svc := coreMocks.NewMockService(t)

	core.RegisterPlugin(core.PluginInfo{
		ID:      "test",
		Version: build.New("1.0", "", "", "", "", "", ""),
		Services: func() ([]core.ServiceInfo, error) {
			return []core.ServiceInfo{
				{
					ID: "test-service",
					Factory: func() (core.Service, []core.ContextBuilderOption, error) {
						return svc, nil, nil
					},
				},
			}, nil
		},
	})

	core.RegisterServicesFromPlugins()

	mockConfigManager.On("GetService", "test", "test-service").Return(mockServiceConfig).Once()

	ctx, err := core.NewContext(mockConfigManager, mockLogger, core.ContextWithService("non-existent-service", svc))
	assert.NoError(t, err)

	retrievedConfig := core.GetServiceConfig[config.ServiceConfig](ctx, "test-service")
	assert.NotNil(t, retrievedConfig)
	assert.Equal(t, mockServiceConfig, retrievedConfig)

	mockConfigManager.AssertExpectations(t)
}

func TestGetServiceConfig_NotFound(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)

	flogger := NewFatalLogInterceptor()

	ctx, err := core.NewContext(mockConfigManager, mockLogger, core.ContextWithLoggerOptions(flogger.Hook()))
	assert.NoError(t, err)

	core.GetServiceConfig[config.ServiceConfig](ctx, "non-existent-service")
	assert.True(t, flogger.FatalWasCalled(), "Expected fatal log for missing service config")

	mockConfigManager.AssertExpectations(t)
}

func TestGetServiceConfig_TypeMismatch(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)

	core.RegisterPlugin(core.PluginInfo{
		ID:      "test",
		Version: build.New("1.0", "", "", "", "", "", ""),
		Services: func() ([]core.ServiceInfo, error) {
			return []core.ServiceInfo{
				{
					ID: "test-service",
					Factory: func() (core.Service, []core.ContextBuilderOption, error) {
						return nil, nil, nil
					},
				},
			}, nil
		},
	})

	core.RegisterServicesFromPlugins()

	// Create a concrete service config that implements ServiceConfig but not APIConfig
	type testServiceConfig struct {
		config.ServiceConfig
	}
	serviceCfg := &testServiceConfig{}
	mockConfigManager.On("GetService", "test", "test-service").Return(serviceCfg).Once()

	ctx, err := core.NewContext(mockConfigManager, mockLogger)
	assert.NoError(t, err)

	// Try to get it as CacheConfig which should return zero value (type mismatch)
	retrievedConfig := core.GetServiceConfig[config.CacheConfig](ctx, "test-service")
	assert.Equal(t, config.CacheConfig{}, retrievedConfig, "Expected zero value for config type mismatch")

	mockConfigManager.AssertExpectations(t)
}

func TestGetAPIConfig(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)

	mockConfigManager.On("GetAPI", "test-api").Return(mockAPIConfig).Once()

	ctx, err := core.NewContext(mockConfigManager, mockLogger)
	assert.NoError(t, err)

	retrievedConfig := core.GetAPIConfig[config.APIConfig](ctx, "test-api")
	assert.NotNil(t, retrievedConfig)

	mockConfigManager.AssertExpectations(t)
}

func TestGetAPIConfig_NotFound(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)
	mockConfigManager.On("GetAPI", "non-existent-api").Return(nil).Once()

	ctx, err := core.NewContext(mockConfigManager, mockLogger)
	assert.NoError(t, err)

	retrievedConfig := core.GetAPIConfig[config.APIConfig](ctx, "non-existent-api")
	assert.Nil(t, retrievedConfig, "Expected nil for missing API config")

	mockConfigManager.AssertExpectations(t)
}

func TestGetAPIConfig_TypeMismatch(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)
	mockConfigManager.On("GetAPI", "test-api").Return(mockAPIConfig).Once()

	ctx, err := core.NewContext(mockConfigManager, mockLogger)
	assert.NoError(t, err)

	retrievedConfig := core.GetAPIConfig[config.CacheConfig](ctx, "test-api")
	assert.Equal(t, config.CacheConfig{}, retrievedConfig, "Expected zero value for API config type mismatch")

	mockConfigManager.AssertExpectations(t)
}

func TestGetProtocolConfig(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)

	mockConfigManager.On("GetProtocol", "test-protocol").Return(mockProtocolConfig).Once()

	ctx, err := core.NewContext(mockConfigManager, mockLogger)
	assert.NoError(t, err)

	retrievedConfig := core.GetProtocolConfig[config.ProtocolConfig](ctx, "test-protocol")
	assert.NotNil(t, retrievedConfig)

	mockConfigManager.AssertExpectations(t)
}

func TestGetProtocolConfig_NotFound(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)
	mockConfigManager.On("GetProtocol", "non-existent-protocol").Return(nil).Once()

	ctx, err := core.NewContext(mockConfigManager, mockLogger)
	assert.NoError(t, err)

	retrievedConfig := core.GetProtocolConfig[config.ProtocolConfig](ctx, "non-existent-protocol")
	assert.Nil(t, retrievedConfig, "Expected nil for missing protocol config")

	mockConfigManager.AssertExpectations(t)
}

func TestGetProtocolConfig_TypeMismatch(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)
	mockConfigManager.On("GetProtocol", "test-protocol").Return(nil).Once()

	ctx, err := core.NewContext(mockConfigManager, mockLogger)
	assert.NoError(t, err)

	retrievedConfig := core.GetProtocolConfig[config.ServiceConfig](ctx, "test-protocol")
	assert.Nil(t, retrievedConfig, "Expected nil for protocol config not found/type mismatch")

	mockConfigManager.AssertExpectations(t)
}

func TestServiceExists(t *testing.T) {
	core.ResetState()
	mockConfigManager := newMockConfigManager(t, nil)
	mockLogger := core.NewLogger(mockConfigManager, nil)
	mockService := mocks.NewMockService(t)

	ctx, err := core.NewContext(mockConfigManager, mockLogger, core.ContextWithService("test-service", mockService))
	assert.NoError(t, err)

	assert.True(t, core.ServiceExists(ctx, "test-service"))
	assert.False(t, core.ServiceExists(ctx, "non-existent-service"))

	mockConfigManager.AssertExpectations(t)
}

func TestResetState(t *testing.T) {
	// Add some dummy data to the global state
	core.RegisterProtocol("test-protocol", mocks.NewMockProtocol(t))
	core.RegisterAPI("test-api", mocks.NewMockAPI(t))
	core.RegisterService(core.ServiceInfo{ID: "test-svc", Factory: func() (core.Service, []core.ContextBuilderOption, error) { return mocks.NewMockService(t), nil, nil }})
	core.RegisterPlugin(newTestPluginInfo("test-plugin"))
	// Create mock services that events depend on
	mockAccess := mocks.NewMockAccessService(t)
	mockHTTP := mocks.NewMockHTTPService(t)

	core.RegisterService(core.ServiceInfo{
		ID: "access",
		Factory: func() (core.Service, []core.ContextBuilderOption, error) {
			return mockAccess, nil, nil
		},
	})
	core.RegisterService(core.ServiceInfo{
		ID: "http",
		Factory: func() (core.Service, []core.ContextBuilderOption, error) {
			return mockHTTP, nil, nil
		},
	})

	// Register some test hash algorithms
	err := core.GetHashRegistry().RegisterHashAlgorithm(core.HashAlgorithm{
		Type:     0x12, // SHA2-256
		Name:     "SHA-256",
		Priority: 100,
		Protocol: "test",
	})
	assert.NoError(t, err)

	// Check that the state is not empty
	assert.NotEmpty(t, core.GetProtocols())
	assert.NotEmpty(t, core.GetAPIs())
	assert.NotEmpty(t, core.GetServices())
	assert.NotEmpty(t, core.GetPlugins())
	assert.NotEmpty(t, core.GetHashRegistry().GetHashAlgorithms())

	// Reset the state
	core.ResetState()

	// Check that the state is now empty
	assert.Empty(t, core.GetProtocols())
	assert.Empty(t, core.GetAPIs())
	assert.Empty(t, core.GetServices())
	assert.Empty(t, core.GetPlugins())
	assert.Empty(t, core.GetHashRegistry().GetHashAlgorithms())
}
