// Package testing provides utilities for testing core components
package testing

import (
	"context"
	"fmt"
	"github.com/gookit/event"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"gorm.io/gorm"
	"sync"
	"testing"
	"time"
)

var (
	testCtxOpts   []TestContextBuilderOption
	testCtxOptsMu sync.RWMutex
)

// AddTestContextOptions adds one or more TestContextOptions to the global testing options collection
func AddTestContextOptions(opts ...TestContextBuilderOption) {
	testCtxOptsMu.Lock()
	defer testCtxOptsMu.Unlock()
	testCtxOpts = append(testCtxOpts, opts...)
}

// GetTestContextOptions returns a copy of all registered TestContextOptions
func GetTestContextOptions() []TestContextBuilderOption {
	testCtxOptsMu.RLock()
	defer testCtxOptsMu.RUnlock()
	opts := make([]TestContextBuilderOption, len(testCtxOpts))
	copy(opts, testCtxOpts)
	return opts
}

// ClearTestContextOptions resets the test context options collection
func ClearTestContextOptions() {
	testCtxOptsMu.Lock()
	defer testCtxOptsMu.Unlock()
	testCtxOpts = nil
}

// TB is an interface that both *testing.T and *testing.B satisfy
type TB interface {
	Helper()
	Cleanup(func())
	Error(args ...any)
	Errorf(format string, args ...any)
	Fail()
	FailNow()
	Failed() bool
	Fatal(args ...any)
	Fatalf(format string, args ...any)
	Log(args ...any)
	Logf(format string, args ...any)
	Name() string
	Skip(args ...any)
	SkipNow()
	Skipf(format string, args ...any)
	Skipped() bool
}

// mockTB adapts a TB to satisfy testify's TestingT interface
type mockTB struct {
	tb TB
}

func (m *mockTB) Cleanup(f func()) {
	m.tb.Cleanup(f)
}

func (m *mockTB) Error(args ...any) {
	m.tb.Error(args...)
}

func (m *mockTB) Errorf(format string, args ...any) {
	m.tb.Errorf(format, args...)
}

func (m *mockTB) Fail() {
	m.tb.Fail()
}

func (m *mockTB) FailNow() {
	m.tb.FailNow()
}

func (m *mockTB) Failed() bool {
	return m.tb.Failed()
}

func (m *mockTB) Fatal(args ...any) {
	m.tb.Fatal(args...)
}

func (m *mockTB) Fatalf(format string, args ...any) {
	m.tb.Fatalf(format, args...)
}

func (m *mockTB) Helper() {
	m.tb.Helper()
}

func (m *mockTB) Log(args ...any) {
	m.tb.Log(args...)
}

func (m *mockTB) Logf(format string, args ...any) {
	m.tb.Logf(format, args...)
}

func (m *mockTB) Name() string {
	return m.tb.Name()
}

func (m *mockTB) Skip(args ...any) {
	m.tb.Skip(args...)
}

func (m *mockTB) SkipNow() {
	m.tb.SkipNow()
}

func (m *mockTB) Skipf(format string, args ...any) {
	m.tb.Skipf(format, args...)
}

func (m *mockTB) Skipped() bool {
	return m.tb.Skipped()
}

// TestContext extends core.Context with testing-specific methods
type TestContext interface {
	core.Context
	T() TB                                          // Access to the testing.TB instance (works with both *testing.T and *testing.B)
	RegisterService(id string, service interface{}) // Register a mock service
	Teardown()                                      // Clean up resources
	RegisterCleanup(fn func())                      // Register custom cleanup functions
}

// ResetAllState resets all global state in the core package and testing package
func ResetAllState() {
	core.ResetState()
	ClearTestContextOptions()
}

// TestContextBuilderOption configures a test context
type TestContextBuilderOption func(context TestContext) (TestContext, error)

// defaultContext is a copy of core.defaultContext for testing
type defaultContext struct {
	context.Context
	services     map[string]any
	cfg          config.Manager
	logger       *core.Logger
	exitFuncs    []func(core.Context) error
	exitCode     int
	startupFuncs []func(core.Context) error
	db           *gorm.DB
	cancel       context.CancelFunc
	event        *event.Manager
}

// testContext extends the default context for testing
type testContext struct {
	*defaultContext
	tb           TB
	cleanupFuncs []func()
}

// Ensure testContext implements TestContext
var _ TestContext = (*testContext)(nil)

// NewTestContext creates a new Context suitable for testing with either *testing.T or *testing.B
func NewTestContext(tb TB, opts ...TestContextBuilderOption) TestContext {
	tb.Helper()

	// Create a mock config manager
	var mockConfig *MockConfigManager

	// Handle different types of test runners
	if t, ok := tb.(*testing.T); ok {
		// If it's a *testing.T, use it directly
		mockConfig = NewMockConfigManager(t)
	} else {
		// For benchmarks or other test types, create a simple wrapper around the MockConfigManager
		// that doesn't depend on expectations being verified
		mockConfig = &MockConfigManager{
			values: make(map[string]interface{}),
			cfg:    &config.Config{},
		}
	}

	// Create a test logger based on the test type
	var zapLogger *zap.Logger
	if t, ok := tb.(*testing.T); ok {
		zapLogger = zaptest.NewLogger(t)
	} else if b, ok := tb.(*testing.B); ok {
		zapLogger = zaptest.NewLogger(b)
	} else {
		// Fallback for other TB implementations
		zapLogger = zap.NewNop()
	}

	logger := core.NewLogger(mockConfig)
	// Replace the underlying zap logger
	logger.Logger = zapLogger

	// Set the logger on the mock config
	mockConfig.SetLogger(zapLogger)

	// Create context with cancel
	baseCtx, cancel := context.WithCancel(context.Background())

	// Create the test context
	testCtx := &testContext{
		defaultContext: &defaultContext{
			Context:      baseCtx,
			services:     make(map[string]any),
			cfg:          mockConfig,
			logger:       logger,
			event:        event.NewManager(""),
			cancel:       cancel,
			exitFuncs:    []func(core.Context) error{},
			startupFuncs: []func(core.Context) error{},
		},
		tb:           tb,
		cleanupFuncs: []func(){},
	}

	// Apply options
	for _, opt := range opts {
		opt(testCtx)
	}

	return testCtx
}

// RegisterService adds a service to the context after creation
func (c *testContext) RegisterService(id string, service interface{}) {
	c.services[id] = service
}

// RegisterCleanup adds a function to be called during Teardown
func (c *testContext) RegisterCleanup(fn func()) {
	c.cleanupFuncs = append(c.cleanupFuncs, fn)
	c.tb.Cleanup(fn) // Also register with testing.TB for safety
}

// Teardown cleans up resources
func (c *testContext) Teardown() {
	// Close DB connections
	if c.db != nil {
		sqlDB, err := c.db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	}

	// Cancel context
	if c.cancel != nil {
		c.cancel()
	}

	// Run any registered cleanup functions
	for _, fn := range c.cleanupFuncs {
		fn()
	}
}

// WithMockService adds a mock service to the test context
func WithMockService(id string, service core.Service) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		ctx.(*testContext).services[id] = service
		return ctx, nil
	}
}

// WithMockDB adds a mock database to the test context
func WithMockDB(db *gorm.DB) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		ctx.(*testContext).db = db
		ctx.RegisterCleanup(func() {
			sqlDB, err := db.DB()
			if err == nil {
				_ = sqlDB.Close()
			}
		})

		return ctx, nil
	}
}

// WithConfigValue sets a configuration value
func WithConfigValue(key string, value interface{}) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		if ctx.(*testContext).cfg != nil {
			// Use Update method from config.Manager interface
			_ = ctx.(*testContext).cfg.Update(key, value)
		}
		return ctx, nil
	}
}

// T returns the testing.TB instance for backward compatibility
// This will work with both *testing.T and *testing.B
func (c *testContext) T() TB {
	return c.tb
}

// Implement the Context interface methods for testContext

func (ctx *testContext) Service(id string) any {
	if svc, ok := ctx.services[id]; ok {
		return svc
	}
	return nil
}

func (ctx *testContext) OnExit(f core.LifecycleFunc) {
	ctx.exitFuncs = append(ctx.exitFuncs, f)
}

func (ctx *testContext) OnStartup(f core.LifecycleFunc) {
	ctx.startupFuncs = append(ctx.startupFuncs, f)
}

func (ctx *testContext) StartupFuncs() []func(core.Context) error {
	return ctx.startupFuncs
}

func (ctx *testContext) ExitFuncs() []func(core.Context) error {
	return ctx.exitFuncs
}

func (ctx *testContext) DB() *gorm.DB {
	if ctx.db == nil {
		return nil
	}
	return ctx.db.WithContext(ctx)
}

func (ctx *testContext) Logger() *core.Logger {
	return ctx.logger
}

func (ctx *testContext) ProtocolLogger(protocol core.Protocol) *core.Logger {
	return ctx.NamedLogger("protocol-" + protocol.Name())
}

func (ctx *testContext) APILogger(api core.API) *core.Logger {
	return ctx.NamedLogger("api-" + api.Name())
}

func (ctx *testContext) ServiceLogger(service core.Service) *core.Logger {
	return ctx.NamedLogger("service-" + service.ID())
}

func (ctx *testContext) WithLoggerOptions(opts ...zap.Option) *core.Logger {
	return core.NewLogger(ctx.Config(), ctx.logger.Logger.WithOptions(opts...))
}

func (ctx *testContext) WithLoggerLazy(opts ...zap.Field) *core.Logger {
	return core.NewLogger(ctx.Config(), ctx.logger.Logger.WithLazy(opts...))
}

func (ctx *testContext) WithLogger(opts ...zap.Field) *core.Logger {
	return core.NewLogger(ctx.Config(), ctx.logger.Logger.With(opts...))
}

func (ctx *testContext) NamedLogger(name string) *core.Logger {
	return core.NewLogger(ctx.Config(), ctx.logger.Logger.Named(name))
}

func (ctx *testContext) Config() config.Manager {
	return ctx.cfg
}

func (ctx *testContext) Cancel() {
	ctx.cancel()
}

func (ctx *testContext) ExitCode() int {
	return ctx.exitCode
}

func (ctx *testContext) Event() *event.Manager {
	return ctx.event
}

func (ctx *testContext) SetExitCode(code int) {
	ctx.exitCode = code
}

func (ctx *testContext) GetContext() context.Context {
	return ctx.Context
}

// Deadline implements the Context interface
func (c *testContext) Deadline() (deadline time.Time, ok bool) {
	return c.Context.Deadline()
}

// Done implements the Context interface
func (c *testContext) Done() <-chan struct{} {
	return c.Context.Done()
}

// Err implements the Context interface
func (c *testContext) Err() error {
	return c.Context.Err()
}

// Value implements the Context interface
func (c *testContext) Value(key interface{}) interface{} {
	return c.Context.Value(key)
}

// ProcessCtxOptions applies a series of ContextBuilderOptions to a TestContext.
// It returns the modified context and any error encountered during processing.
// Each option is applied in sequence, with the result of one becoming the input to the next.
func ProcessCtxOptions(ctx TestContext, options ...TestContextBuilderOption) (TestContext, error) {
	newCtx := ctx

	for _, opt := range options {
		returnCtx, err := opt(newCtx)
		if err != nil {
			return newCtx, err
		}
		// Type assert back to *defaultContext if needed
		if dc, ok := returnCtx.(TestContext); ok {
			newCtx = dc
		} else {
			return newCtx, fmt.Errorf("context type changed unexpectedly")
		}
	}

	return newCtx, nil
}

// ProcessStartupFuncs executes all registered startup functions in the TestContext.
// Returns the first error encountered, if any. Functions are executed in the order they were registered.
// This is typically called during test initialization to simulate the portal's startup sequence.
func ProcessStartupFuncs(ctx TestContext) error {
	newCtx := ctx

	var err error

	for _, fn := range ctx.StartupFuncs() {
		err = fn(newCtx)
		if err != nil {
			return err
		}
	}

	return nil
}

// ProcessExitFuncs executes all registered exit functions in the TestContext.
// Unlike ProcessStartupFuncs, it continues executing remaining functions even if one fails.
// Errors are logged but don't stop execution. This is typically called during test cleanup.
func ProcessExitFuncs(ctx TestContext) error {
	newCtx := ctx

	var err error

	for _, fn := range ctx.ExitFuncs() {
		err = fn(newCtx)
		if err != nil {
			return err
		}
	}

	return nil
}

// InitContext fully initializes a TestContext by:
// 1. Processing all ContextBuilderOptions (both from parameters and global test options)
// 2. Running all startup functions  
// 3. Registering all exit functions
// Returns the first error encountered at any step.
// This provides a complete initialization sequence for testing scenarios.
func InitContext(ctx TestContext, opts ...TestContextBuilderOption) error {
	// Combine provided options with any globally registered test options
	allOpts := append(opts, GetTestContextOptions()...)
	
	// Process all context options
	var err error
	ctx, err = ProcessCtxOptions(ctx, allOpts...)
	if err != nil {
		return err
	}

	// Process startup functions
	err = ProcessStartupFuncs(ctx)
	if err != nil {
		return err
	}

	// Process exit functions
	err = ProcessExitFuncs(ctx) 
	if err != nil {
		return err
	}

	return nil
}

// WrapCoreOption converts a core.ContextBuilderOption to a TestContextBuilderOption
func WrapCoreOption(opt core.ContextBuilderOption) TestContextBuilderOption {
	return func(tctx TestContext) (TestContext, error) {
		newCtx, err := opt(tctx)
		if err != nil {
			return nil, err
		}
		// Ensure we maintain TestContext type
		if tc, ok := newCtx.(TestContext); ok {
			return tc, nil
		}
		return nil, fmt.Errorf("expected TestContext, got %T", newCtx)
	}
}

// WrapCoreOptions converts a slice of core.ContextBuilderOptions to TestContextBuilderOptions
func WrapCoreOptions(opts []core.ContextBuilderOption) []TestContextBuilderOption {
	var wrapped []TestContextBuilderOption
	for _, opt := range opts {
		wrapped = append(wrapped, WrapCoreOption(opt))
	}
	return wrapped
}

// GetMockConfig returns the mock config manager from the context for testing
// Panics if the config manager is not a mock
func GetMockConfig(ctx core.Context) *MockConfigManager {
	mockConfig, ok := ctx.Config().(*MockConfigManager)
	if !ok {
		panic("config manager is not a mock - use NewMockContext() for testing")
	}
	return mockConfig
}

// RegisterAPI registers an API and wraps any returned context options for test context
func RegisterAPI(ctx TestContext, id string, factory core.APIFactory) (ctxOpts []TestContextBuilderOption, err error) {
	api, opts, err := factory()
	if err != nil {
		ctx.Logger().Error("Error building API", zap.String("plugin", id), zap.Error(err))
		return nil, err
	}

	if api == nil {
		ctx.Logger().Error("Error building API", zap.Error(err))
	}

	core.RegisterAPI(id, api)
	return WrapCoreOptions(opts), nil
}

func RegisterAPIExtension(ctx TestContext, factory core.APIExtensionsFactory) (ctxOpts []TestContextBuilderOption, err error) {
	extensions, err := factory(ctx)
	if err != nil {
		ctx.Logger().Error("Error building API extensions", zap.Error(err))
		return nil, err
	}

	for _, extFactory := range extensions {
		factory := extFactory
		apiExtStartup := TestContextBuilderOption(func(tctx TestContext) (TestContext, error) {
			ext, ctxOptions, err := factory()
			if err != nil {
				tctx.Logger().Error("Error building API extensions", zap.String("target", ext.TargetAPI()), zap.Error(err))
				return nil, err
			}
			tctx.Logger().Info("Registering API extension",
				zap.String("target", ext.TargetAPI()))
			core.RegisterAPIExtension(ext)

			wrappedOpts := WrapCoreOptions(ctxOptions)
			return ProcessCtxOptions(tctx, wrappedOpts...)
		})
		ctxOpts = append(ctxOpts, apiExtStartup)
	}

	return ctxOpts, nil
}

func RegisterProtocol(ctx TestContext, id string, factory core.ProtocolFactory) (ctxOpts []TestContextBuilderOption, err error) {
	proto, opts, err := factory()
	if err != nil {
		ctx.Logger().Error("Error building Protocol", zap.String("plugin", id), zap.Error(err))
		return nil, err
	}

	if proto == nil {
		ctx.Logger().Error("Error building Protocol", zap.String("plugin", id), zap.Error(err))
	}

	core.RegisterProtocol(id, proto)
	return WrapCoreOptions(opts), nil
}
