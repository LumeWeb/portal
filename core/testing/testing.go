// Package testing provides utilities for testing core components
package testing

import (
	"bytes"
	"context"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3afero"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/mock"
	"go.lumeweb.com/event/v2"
	"go.lumeweb.com/portal"
	router "go.lumeweb.com/portal-router"
	"go.lumeweb.com/portal/build"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	testingDb "go.lumeweb.com/portal/core/testing/db"
	"go.lumeweb.com/portal/core/testing/mocks"
	coreMocks "go.lumeweb.com/portal/core/testing/mocks"
	pevent "go.lumeweb.com/portal/event"
	pkgReflect "go.lumeweb.com/portal/internal/reflect"
	"go.sia.tech/coreutils/wallet"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"gorm.io/gorm"
	"io/fs"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"sync"
	"testing"
	"testing/fstest"
	"time"
)

func ensureValue(payload any) any {
	if payload == nil {
		return nil
	}
	val := reflect.ValueOf(payload)
	if val.Kind() == reflect.Ptr {
		// If it's already a pointer, return the pointed-to value
		return val.Elem().Interface()
	}
	// For non-pointer values, return as-is
	return payload
}

var (
	// Global options set at RunTests level
	globalTestCtxOpts   []TestContextBuilderOption
	globalTestCtxOptsMu sync.RWMutex

	// Per-test-case options set at RunTestCase level
	testCaseCtxOpts   []TestContextBuilderOption
	testCaseCtxOptsMu sync.RWMutex

	runDBMigrations   = false
	runDBMigrationsMu sync.RWMutex
	setupMockDB       = false
	setupMockDBMu     sync.RWMutex
	testContexts      sync.Map   // map[*testing.T]TestContext
	testMutex         sync.Mutex // Protects test execution
)

// TestMainOpts configures test main behavior
type TestMainOpts struct {
	WithDB         bool
	DBMigrations   bool
	CustomSetup    func()
	CustomTeardown func()
}

// RunTests is the standard way to run tests with automatic setup/teardown
func RunTests(m *testing.M, opts TestMainOpts) int {
	if opts.WithDB {
		EnableMockDB()
		if opts.DBMigrations {
			EnableDBMigrations()
		}
	}

	if opts.CustomSetup != nil {
		opts.CustomSetup()
	}

	code := m.Run()

	if opts.CustomTeardown != nil {
		opts.CustomTeardown()
	}

	if opts.WithDB {
		DisableDBMigrations()
		DisableMockDB()
	}

	// Clear global options at the end of RunTests
	ClearGlobalTestContextOptions()
	ResetAllState()

	return code
}

// WithDB runs tests with database support (migrations enabled by default)
func WithDB(m *testing.M) int {
	return RunTests(m, TestMainOpts{
		WithDB:       true,
		DBMigrations: true,
	})
}

// WithDBAndOptions runs tests with database support and custom builder options
func WithDBAndOptions(m *testing.M, opts ...TestContextBuilderOption) int {
	return RunTests(m, TestMainOpts{
		WithDB:       true,
		DBMigrations: true,
		CustomSetup: func() {
			AddGlobalTestContextOptions(opts...)
		},
	})
}

// WithDBNoMigrations runs tests with database support but skips migrations
func WithDBNoMigrations(m *testing.M) int {
	return RunTests(m, TestMainOpts{
		WithDB:       true,
		DBMigrations: false,
	})
}

// WithDBNoMigrationsAndOptions runs tests with database support (no migrations) and custom builder options
func WithDBNoMigrationsAndOptions(m *testing.M, opts ...TestContextBuilderOption) int {
	return RunTests(m, TestMainOpts{
		WithDB:       true,
		DBMigrations: false,
		CustomSetup: func() {
			AddGlobalTestContextOptions(opts...)
		},
	})
}

// WithOptions runs tests with custom builder options (no database by default)
func WithOptions(m *testing.M, opts ...TestContextBuilderOption) int {
	return RunTests(m, TestMainOpts{
		WithDB: false,
		CustomSetup: func() {
			AddGlobalTestContextOptions(opts...)
		},
	})
}

// WithoutDB runs tests without database support
func WithoutDB(m *testing.M) int {
	return RunTests(m, TestMainOpts{
		WithDB: false,
	})
}

// AddGlobalTestContextOptions adds options that persist for all test cases in a RunTests call
func AddGlobalTestContextOptions(opts ...TestContextBuilderOption) {
	globalTestCtxOptsMu.Lock()
	defer globalTestCtxOptsMu.Unlock()
	globalTestCtxOpts = append(globalTestCtxOpts, opts...)
}

// AddTestCaseContextOptions adds options specific to a single test case
func AddTestCaseContextOptions(opts ...TestContextBuilderOption) {
	testCaseCtxOptsMu.Lock()
	defer testCaseCtxOptsMu.Unlock()
	testCaseCtxOpts = append(testCaseCtxOpts, opts...)
}

// GetCombinedTestContextOptions returns all applicable options in order:
// 1. Default options
// 2. Global options (from RunTests)
// 3. Test case options (from RunTestCase)
func GetCombinedTestContextOptions(tb TB) []TestContextBuilderOption {
	globalTestCtxOptsMu.RLock()
	defer globalTestCtxOptsMu.RUnlock()
	testCaseCtxOptsMu.RLock()
	defer testCaseCtxOptsMu.RUnlock()

	// Get defaults first
	defaultOpts := DefaultTestContextOptions(tb)

	// Combine with global and test case options
	opts := make([]TestContextBuilderOption, 0, len(defaultOpts)+len(globalTestCtxOpts)+len(testCaseCtxOpts))
	opts = append(opts, defaultOpts...)
	opts = append(opts, globalTestCtxOpts...)
	opts = append(opts, testCaseCtxOpts...)

	return opts
}

// ClearGlobalTestContextOptions resets the global options collection
func ClearGlobalTestContextOptions() {
	globalTestCtxOptsMu.Lock()
	defer globalTestCtxOptsMu.Unlock()
	globalTestCtxOpts = nil
}

// ClearTestCaseContextOptions resets the test case options collection
func ClearTestCaseContextOptions() {
	testCaseCtxOptsMu.Lock()
	defer testCaseCtxOptsMu.Unlock()
	testCaseCtxOpts = nil
}

type TB = testing.TB

// TestContext extends core.Context with testing-specific methods
type TestContext interface {
	core.Context
	T() TB                                          // Access to the testing.TB instance (works with both *testing.T and *testing.B)
	RegisterService(id string, service interface{}) // Register a mock service
	Teardown()                                      // Clean up resources
	RegisterCleanup(fn func())                      // Register custom cleanup functions
	Router() router.Router
	SetDB(*gorm.DB)                                               // Set the database instance
	APISubdomain(id string, proto bool) string                    // Get formatted API subdomain URL
	NewAPIRequest(method, path string, body []byte) *http.Request // Create new API request with proper host header
	APIID() string                                                // Get the current API ID
	SetAPIID(id string)                                           // Set the API ID
}

// SetupTest creates and manages the test context for a specific test
// It does NOT boot the environment. BootEnvironment must be called separately.
func SetupTest(t TB) TestContext {
	t.Helper()

	// Check if we already have a context for this test
	if ctx, ok := testContexts.Load(t); ok {
		return ctx.(TestContext)
	}

	// Create new context with current options
	// Options are NOT processed here, they are added to the global list
	ctx := NewTestContext(t)

	// Store it
	testContexts.Store(t, ctx)

	// Automatically clean up when test finishes
	t.Cleanup(func() {
		ctx.Teardown()
		testContexts.Delete(t)
	})

	return ctx
}

// SetupTestWithDB creates a test context with database support
// It does NOT boot the environment. BootEnvironment must be called separately.
func SetupTestWithDB(t TB) TestContext {
	t.Helper()

	// Enable DB migrations
	EnableDBMigrations()

	// Setup test context
	ctx := SetupTest(t)

	return ctx
}

// GetTestContext retrieves the context for a test if it exists
func GetTestContext(t TB) TestContext {
	t.Helper()
	if ctx, ok := testContexts.Load(t); ok {
		return ctx.(TestContext)
	}
	t.Fatal("No test context found - did you call SetupTest()?")
	return nil
}

// RunTestCase provides a cleaner way to run tests with automatic context setup
func RunTestCase(t TB, testFunc func(tb TB, ctx TestContext), opts ...TestContextBuilderOption) {
	t.Helper()

	testMutex.Lock()
	defer testMutex.Unlock()

	// Reset test case state before test
	ResetAllState()
	defer ResetAllState()

	// Phase 1: Registration
	// Add test case specific options
	if len(opts) > 0 {
		AddTestCaseContextOptions(opts...)
	}

	// Create test context
	ctx := SetupTest(t)

	// Phases 2 & 3: Configuration & Initialization
	if err := BootEnvironment(t, ctx); err != nil {
		t.Fatalf("Failed to boot test environment: %v", err)
	}

	// Run the actual test
	testFunc(t, ctx)
}

// RunTestCaseWithDB provides a cleaner way to run tests with automatic context setup and database support
func RunTestCaseWithDB(t TB, testFunc func(tb TB, ctx TestContext), opts ...TestContextBuilderOption) {
	t.Helper()

	testMutex.Lock()
	defer testMutex.Unlock()

	// Reset test case state before test
	ResetAllState()
	defer ResetAllState()

	EnableDBMigrations()
	defer DisableDBMigrations()

	// Add any provided options to the test case collection
	if len(opts) > 0 {
		AddTestCaseContextOptions(opts...)
	}

	// Get or create the context (without booting the environment yet)
	ctx := SetupTestWithDB(t)

	// Boot the environment *after* the context is stored and before running the test function
	if err := BootEnvironment(t, ctx); err != nil {
		t.Fatalf("Failed to boot test environment: %v", err)
	}

	testFunc(t, ctx)

	// Test case options are cleared by deferred ResetAllState
}

// GetFreeListener finds and returns a listener on a TCP port
func GetFreeListener() (net.Listener, error) {
	addr, err := net.ResolveTCPAddr("tcp", "0.0.0.0:0")
	if err != nil {
		return nil, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return nil, err
	}

	return l, nil
}

// ResetAllState resets all global state in the core package and testing package
// while preserving package-level configuration like DB settings
func ResetAllState() {
	// Reset core state
	core.ResetState()

	// Reset testing state (only clears test case specific options)
	ClearTestCaseContextOptions()

	// Note: We intentionally don't reset runDBMigrations/setupMockDB here
	// as these are package-level settings that should persist across tests
	// They are only reset when TestMain completes
}

// EnableDBMigrations enables running DB migrations during test context initialization
// This also enables mock DB setup since migrations require a database connection
func EnableDBMigrations() {
	runDBMigrationsMu.Lock()
	defer runDBMigrationsMu.Unlock()
	runDBMigrations = true

	// Ensure DB is enabled since migrations require it
	EnableMockDB()
}

// DisableDBMigrations disables running DB migrations during test context initialization
func DisableDBMigrations() {
	runDBMigrationsMu.Lock()
	defer runDBMigrationsMu.Unlock()
	runDBMigrations = false
}

// EnableMockDB enables setting up a mock DB during test context initialization
func EnableMockDB() {
	setupMockDBMu.Lock()
	defer setupMockDBMu.Unlock()
	setupMockDB = true
}

// DisableMockDB disables setting up a mock DB during test context initialization
func DisableMockDB() {
	setupMockDBMu.Lock()
	defer setupMockDBMu.Unlock()
	setupMockDB = false
}

// ShouldRunDBMigrations returns whether DB migrations should run
func ShouldRunDBMigrations() bool {
	runDBMigrationsMu.RLock()
	defer runDBMigrationsMu.RUnlock()
	return runDBMigrations
}

// ShouldSetupMockDB returns whether mock DB should be setup
func ShouldSetupMockDB() bool {
	setupMockDBMu.RLock()
	defer setupMockDBMu.RUnlock()
	return setupMockDB
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
	event        event.EventManager[any]
	router       router.Router
}

// testContext extends the default context for testing
type testContext struct {
	*defaultContext
	tb               TB
	cleanupFuncs     []func()
	apiID            string // Stores the API ID for this context
	fireBootComplete bool   // Controls whether to fire boot complete event
}

// Ensure testContext implements TestContext
var _ TestContext = (*testContext)(nil)

// NewTestContext creates a new Context suitable for testing with either *testing.T or *testing.B
// It does NOT process the options immediately. Options are processed during BootEnvironment.
func NewTestContext(tb TB, opts ...TestContextBuilderOption) TestContext {
	tb.Helper()

	// Create a mock config manager
	var mockConfig *MockConfigManager

	// Default to firing boot complete event
	fireBootComplete := false

	// Handle different types of test runners
	if t, ok := tb.(*testing.T); ok {
		// If it's a *testing.T, use it directly
		mockConfig = NewMockConfigManager(t)
	} else {
		// For benchmarks or other test types, create a simple wrapper around the MockConfigManager
		// that doesn't depend on expectations being verified
		mockConfig = &MockConfigManager{
			cfg: &config.Config{},
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
			event:        event.NewM(""),
			cancel:       cancel,
			exitFuncs:    []func(core.Context) error{},
			startupFuncs: []func(core.Context) error{},
		},
		tb:               tb,
		cleanupFuncs:     []func(){},
		fireBootComplete: fireBootComplete,
	}

	AddTestCaseContextOptions(opts...)

	return testCtx // Return the context without processing options yet
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
	// Note: testing.TB.Cleanup also handles this, but keeping this for clarity
	// and potential future custom cleanup logic not tied to TB.Cleanup.
	for _, fn := range c.cleanupFuncs {
		fn()
	}
}

// WithMockService adds a mock service to the test context by calling the mock constructor
// during the test context's BootEnvironment phase.
func WithMockService(id string, mockConstructor func(tb TB, ctx TestContext) any) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		// Register a startup function that will create and register the mock later
		startupOpt := core.ContextWithStartupFunc(func(coreCtx core.Context) error {
			tctx, ok := coreCtx.(TestContext)
			if !ok {
				return fmt.Errorf("context is not a TestContext, cannot use WithMockService")
			}

			// Create and register the mock instance
			mockInstance := mockConstructor(tctx.T(), tctx)
			if err := registerServiceInstance(tctx, id, mockInstance); err != nil {
				return fmt.Errorf("failed to register mock service: %w", err)
			}

			return nil
		})

		return ProcessCtxOptions(ctx, WrapCoreOption(startupOpt))
	}
}

// WithRouter adds a router to the test context
func WithRouter(r router.Router) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		ctx.(*testContext).router = r
		return ctx, nil
	}
}

// WithConfig sets a configuration value
func WithConfig(key string, value interface{}) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		if ctx.(*testContext).cfg != nil {
			// Use Update method from config.Manager interface
			_ = ctx.(*testContext).cfg.Set(ctx, key, value)
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

func (ctx *testContext) SetDB(db *gorm.DB) {
	ctx.db = db
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

func (ctx *testContext) Event() event.EventManager[any] {
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

func (c *testContext) Fire(eventName string, payload any) error {
	val := ensureValue(payload)
	return core.Fire[any](c, eventName, &val)
}

func (c *testContext) MustFire(eventName string, payload any) {
	val := ensureValue(payload)
	core.MustFire[any](c, eventName, &val)
}

func (c *testContext) FireAsync(eventName string, payload any) {
	val := ensureValue(payload)
	core.FireAsync[any](c, eventName, &val)
}

func (c *testContext) ResetEvents() {
	c.Event().Reset()
}

func (c *testContext) Router() router.Router {
	return c.router
}

func (c *testContext) NewAPIRequest(method, path string, body []byte) *http.Request {
	c.tb.Helper()

	if c.apiID == "" {
		c.tb.Fatal("API ID not set in test context")
	}

	mockHttpSvc := core.GetService[core.HTTPService](c, core.HTTP_SERVICE)
	if mockHttpSvc == nil {
		c.tb.Fatal("HTTP service not available in context")
	}

	apiSubdomain := mockHttpSvc.APISubdomain(c.apiID, false)

	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Host = apiSubdomain

	return req
}

func (c *testContext) APIID() string {
	return c.apiID
}

func (c *testContext) SetAPIID(id string) {
	c.apiID = id
}

// FireBootComplete returns whether boot complete event should fire
func (c *testContext) FireBootComplete() bool {
	return c.fireBootComplete
}

// SetFireBootComplete sets whether boot complete event should fire
func (c *testContext) SetFireBootComplete(fire bool) {
	c.fireBootComplete = fire
}

// APISubdomain returns the formatted API subdomain URL
// id: The API identifier (e.g. "admin")
// proto: Whether to include https:// prefix
func (c *testContext) APISubdomain(id string, proto bool) string {
	formatter := ""
	if proto {
		formatter += "https://"
	}
	formatter += "%s.%s"

	api := core.GetAPI(id)
	if api == nil {
		c.tb.Fatalf("API with id '%s' not found - did you register it with WithAPI()?", id)
		return "" // unreachable but makes compiler happy
	}

	return fmt.Sprintf(formatter, api.Subdomain(), c.Config().Config().Core.Domain)
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
		// Ensure we maintain TestContext type
		if tc, ok := returnCtx.(TestContext); ok {
			newCtx = tc
		} else {
			// This should not happen if options are correctly implemented
			return newCtx, fmt.Errorf("context type changed unexpectedly after option application")
		}
	}

	return newCtx, nil
}

// CombineOptions combines multiple TestContextBuilderOptions into a single option
func CombineOptions(opts ...TestContextBuilderOption) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		var err error
		for _, opt := range opts {
			ctx, err = opt(ctx)
			if err != nil {
				return ctx, err
			}
		}
		return ctx, nil
	}
}

// ProcessStartupFuncs executes all registered startup functions in the TestContext.
// Returns the first error encountered, if any. Functions are executed in the order they were registered.
// This is typically called during test initialization to simulate the portal's startup sequence.
func ProcessStartupFuncs(ctx TestContext) error {
	var err error

	for _, fn := range ctx.StartupFuncs() {
		err = fn(ctx) // Use ctx directly
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
	var err error

	for _, fn := range ctx.ExitFuncs() {
		err = fn(ctx) // Use ctx directly
		if err != nil {
			// Log the error but continue with other exit functions
			ctx.Logger().Error("Error during exit function", zap.Error(err))
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
// NOTE: This function is now primarily used internally by BootEnvironment.
// For most test cases, use SetupTest or RunTestCase helpers.
// This function's role is now primarily to process options and run startup funcs.
func InitContext(tb TB, ctx TestContext, opts ...TestContextBuilderOption) error {
	// Combine provided options with any globally registered test options
	allOpts := append(opts, GetCombinedTestContextOptions(tb)...)

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

type MockServiceFactory[T any] func(interface {
	mock.TestingT
	Cleanup(func())
}) *T

// WithMockServiceFactory creates a TestContextBuilderOption that registers a service
// by calling a factory function immediately during the ProcessCtxOptions phase.
// This allows mocks that require the testing.TB instance to be created with the
// correct TB for each individual test run.
func WithMockServiceFactory[T any](id string, factory MockServiceFactory[T]) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		// Create the mock instance immediately using the test's TB
		serviceInstance := factory(ctx.T())

		// Ensure the created instance is not nil
		if serviceInstance == nil {
			return ctx, fmt.Errorf("mock service factory for '%s' returned nil", id)
		}

		// Register the created service in the context immediately
		ctx.RegisterService(id, serviceInstance)

		// The testing.TB.Cleanup registered by SetupTest should handle
		// verifying expectations on mocks created with ctx.T().

		return ctx, nil
	}
}

// WithServiceFactory creates a TestContextBuilderOption that registers a real service
// by calling a factory function during the startup phase. This allows services to be
// created with the fully initialized test context for each test run.
func WithServiceFactory(id string, factory core.ServiceFactory) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {

		// Create the service instance using the test context
		serviceInstance, ctxOpts, err := factory()
		if err != nil {
			return nil, fmt.Errorf("failed to create service '%s': %w", id, err)
		}

		// Register a startup function that will create and register the service later
		startupOpt := core.ContextWithStartupFunc(func(coreCtx core.Context) error {
			tctx, ok := coreCtx.(TestContext)
			if !ok {
				return fmt.Errorf("context is not a TestContext, cannot use WithServiceFactory")
			}
			// Ensure the created instance is not nil
			if serviceInstance == nil {
				return fmt.Errorf("service factory for '%s' returned nil", id)
			}

			// Register the created service in the context
			if err := registerServiceInstance(tctx, id, serviceInstance); err != nil {
				return fmt.Errorf("failed to register service: %w", err)
			}

			return nil
		})

		options, err := ProcessCtxOptions(ctx, WrapCoreOptions(ctxOpts)...)
		if err != nil {
			return nil, err
		}

		options, err = ProcessCtxOptions(ctx, WrapCoreOption(startupOpt))
		if err != nil {
			return nil, err
		}

		return options, nil
	}
}

// --- Modular Mock Service Setup Functions ---

// WithMockAccessService adds a mock AccessService to the test context.
func WithMockAccessService() TestContextBuilderOption {
	return WithMockService(core.ACCESS_SERVICE, func(tb TB, _ TestContext) any {
		return NewMockAccessService(tb)
	})
}

// WithMockAuthService adds a mock AuthService to the test context.
func WithMockAuthService() TestContextBuilderOption {
	return WithMockService(core.AUTH_SERVICE, func(tb TB, _ TestContext) any {
		return mocks.NewMockAuthService(tb)
	})
}

// WithMockContentScannerService adds a mock ContentScannerService to the test context.
func WithMockContentScannerService() TestContextBuilderOption {
	return WithMockService(core.CONTENT_SCANNER_SERVICE, func(tb TB, _ TestContext) any {
		return mocks.NewMockContentScannerService(tb)
	})
}

// WithMockCronService adds a mock CronService to the test context.
// Note: CronService mock comes with pre-configured expectations for common operations
// to simplify test setup.
func WithMockCronService() TestContextBuilderOption {
	return WithMockService(core.CRON_SERVICE, func(tb TB, _ TestContext) any {
		_mock := mocks.NewMockCronService(tb)
		_mock.EXPECT().RegisterEntity(mock.MatchedBy(func(arg interface{}) bool {
			_, ok := arg.(core.Cronable)
			return ok
		})).Maybe().Return()
		return _mock
	})
}

// WithMockHTTPService adds a mock HTTPService to the test context.
func WithMockHTTPService() TestContextBuilderOption {
	return WithMockService(core.HTTP_SERVICE, func(tb TB, ctx TestContext) any {
		mockSvc := NewMockHTTPService(tb)
		mockSvc.router = ctx.Router()
		mockSvc.cmanager = ctx.Config()
		return mockSvc
	})
}

// WithMockHashMappingService adds a mock HashMappingService to the test context.
func WithMockHashMappingService() TestContextBuilderOption {
	return WithMockService(core.HASH_MAPPING_SERVICE, func(tb TB, _ TestContext) any {
		return mocks.NewMockHashMappingService(tb)
	})
}

// WithMockMailerService adds a mock MailerService to the test context.
func WithMockMailerService() TestContextBuilderOption {
	return WithMockService(core.MAILER_SERVICE, func(tb TB, _ TestContext) any {
		return mocks.NewMockMailerService(tb)
	})
}

// WithMockOTPService adds a mock OTPService to the test context.
func WithMockOTPService() TestContextBuilderOption {
	return WithMockService(core.OTP_SERVICE, func(tb TB, _ TestContext) any {
		return mocks.NewMockOTPService(tb)
	})
}

// WithMockPasswordResetService adds a mock PasswordResetService to the test context.
func WithMockPasswordResetService() TestContextBuilderOption {
	return WithMockService(core.PASSWORD_RESET_SERVICE, func(tb TB, _ TestContext) any {
		return mocks.NewMockPasswordResetService(tb)
	})
}

// WithMockPinService adds a mock PinService to the test context.
func WithMockPinService() TestContextBuilderOption {
	return WithMockService(core.PIN_SERVICE, func(tb TB, _ TestContext) any {
		return mocks.NewMockPinService(tb)
	})
}

// WithMockRequestService adds a mock RequestService to the test context.
func WithMockRequestService() TestContextBuilderOption {
	return WithMockService(core.REQUEST_SERVICE, func(tb TB, _ TestContext) any {
		return mocks.NewMockRequestService(tb)
	})
}

// WithMockRenterService adds a mock RenterService to the test context.
func WithMockRenterService() TestContextBuilderOption {
	return WithMockService(core.RENTER_SERVICE, func(tb TB, _ TestContext) any {
		return NewMockRenterService(tb)
	})
}

// WithMockStorageService adds a mock StorageService to the test context.
func WithMockStorageService() TestContextBuilderOption {
	return WithMockService(core.STORAGE_SERVICE, func(tb TB, _ TestContext) any {
		return mocks.NewMockStorageService(tb)
	})
}

// WithMockTUSService adds a mock TUSService to the test context.
func WithMockTUSService() TestContextBuilderOption {
	return WithMockService(core.TUS_SERVICE, func(tb TB, _ TestContext) any {
		return mocks.NewMockTUSService(tb)
	})
}

// WithMockUserService adds a mock UserService to the test context.
func WithMockUserService() TestContextBuilderOption {
	return WithMockService(core.USER_SERVICE, func(tb TB, _ TestContext) any {
		return mocks.NewMockUserService(tb)
	})
}

// WithMockWorkflowService adds a mock WorkflowService to the test context.
func WithMockWorkflowService() TestContextBuilderOption {
	return WithMockService(core.WORKFLOW_SERVICE, func(tb TB, _ TestContext) any {
		return mocks.NewMockWorkflowService(tb)
	})
}

// WithAPIID sets the API ID for the test context, overriding any default value.
func WithAPIID(apiID string) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		ctx.SetAPIID(apiID)
		return ctx, nil
	}
}

// WithAPIExtension registers an API extension and automatically creates and registers
// a mock version of its target API.
func WithAPIExtension(extFactory core.APIExtensionFactory) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		// First create the extension to get its target API
		ext, extOpts, err := extFactory()
		if err != nil {
			return ctx, fmt.Errorf("failed to create API extension: %w", err)
		}

		targetAPI := ext.TargetAPI()
		tb := ctx.T()

		// Create mock API for the target
		mockAPI := coreMocks.NewMockAPI(tb)

		// Configure basic mock API expectations
		mockAPI.On("Name").Return(targetAPI).Maybe()
		mockAPI.On("Subdomain").Return(targetAPI).Maybe()
		mockAPI.On("AuthTokenName").Return(targetAPI + "_token").Maybe()
		mockAPI.On("Config").Return(&config.Config{}).Maybe()
		mockAPI.On("OpenAPIInfo").Return(router.APIInfo()).Maybe()
		mockAPI.On("Configure", mock.Anything, mock.Anything).Return(nil).Maybe()

		// Register the mock API using existing API registration mechanism
		apiRegOpt := WithAPI(targetAPI, func() (core.API, []core.ContextBuilderOption, error) {
			return mockAPI, nil, nil
		})

		// Process API registration first
		ctx, err = ProcessCtxOptions(ctx, apiRegOpt)
		if err != nil {
			return ctx, fmt.Errorf("failed to register mock API: %w", err)
		}

		// Set the API ID in the context
		ctx.SetAPIID(targetAPI)

		// Register the extension
		core.RegisterAPIExtension(ext)

		// Process any extension options
		if len(extOpts) > 0 {
			wrappedOpts := WrapCoreOptions(extOpts)
			ctx, err = ProcessCtxOptions(ctx, wrappedOpts...)
			if err != nil {
				return ctx, fmt.Errorf("failed to process extension options: %w", err)
			}
		}

		return ctx, nil
	}
}

// --- Default Test Context Options ---

// WithDBMigrations adds a startup function that runs migrations if enabled
func WithDBMigrations() TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		startupOpt := core.ContextWithStartupFunc(func(coreCtx core.Context) error {
			if ShouldRunDBMigrations() {
				if coreCtx.DB() == nil {
					return fmt.Errorf("migrations enabled but no database connection available")
				}
				return RunMigrations(coreCtx.(TestContext))
			}
			return nil
		})
		return ProcessCtxOptions(ctx, WrapCoreOption(startupOpt))
	}
}

// DefaultTestContextOptions returns the default options used for new test contexts.
// These include mock implementations of common core services.
func DefaultTestContextOptions(tb TB) []TestContextBuilderOption {
	_router, err := router.NewRouter(router.APIInfo().Title("Test API").Version("1.0.0"))
	if err != nil {
		panic(fmt.Sprintf("failed to create test router: %v", err))
	}

	opts := []TestContextBuilderOption{
		WithDomain("test.local"),
		WithRandomSeedPhrase(),
		WithCoreEvents(),
	}

	// Add DB setup first if enabled
	if ShouldSetupMockDB() {
		opts = append(opts, WithSQLite(tb))
		if ShouldRunDBMigrations() {
			opts = append(opts, WithDBMigrations())
		}
	}

	// Add remaining mock services
	opts = append(opts,
		WithMockAccessService(),
		WithRouter(_router),
		WithMockAuthService(),
		WithMockContentScannerService(),
		WithMockCronService(),
		WithMockHTTPService(),
		WithMockHashMappingService(),
		WithMockMailerService(),
		WithMockOTPService(),
		WithMockPasswordResetService(),
		WithMockPinService(),
		WithMockUploadService(),
		WithMockRequestService(),
		WithMockRenterService(),
		WithMockStorageService(),
		WithMockTUSService(),
		WithMockUserService(),
		WithMockWorkflowService(),
	)

	return opts
}

// ConfigBuilder is a generic builder for any config type that implements
// the Defaults pattern (map[string]any)
type ConfigBuilder struct {
	values map[string]any
}

// NewConfigBuilder creates a new ConfigBuilder
func NewConfigBuilder() *ConfigBuilder {
	return &ConfigBuilder{
		values: make(map[string]any),
	}
}

// With adds a configuration key-value pair
func (b *ConfigBuilder) With(key string, value any) *ConfigBuilder {
	b.values[key] = value
	return b
}

// Build creates a config object implementing the Defaults interface
func (b *ConfigBuilder) Build() config.Defaults {
	// Create a copy to avoid shared reference issues
	return &genericConfig{values: maps.Clone(b.values)}
}

// genericConfig implements config.Defaults with custom values
type genericConfig struct {
	values map[string]any
}

func (c *genericConfig) Defaults() map[string]any {
	return c.values
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

// WithAPIConfig sets an expectation on the mock ConfigManager
// to return the provided config when GetAPI is called with the given ID.
// The expectation is set to Maybe() to allow but not require the call.
func WithAPIConfig(apiID string, apiConfig config.APIConfig) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		mockConfig := GetMockConfig(ctx)
		err := mockConfig.ConfigureAPI(apiID, apiConfig)
		if err != nil {
			panic(fmt.Errorf("failed to configure API %s: %w", apiID, err))
		}
		return ctx, nil
	}
}

// WithCustomAPIConfig creates API config using builder pattern
func WithCustomAPIConfig(apiID string, builder *ConfigBuilder) TestContextBuilderOption {
	return WithAPIConfig(apiID, builder.Build())
}

// WithProtocolConfig configures a protocol with the given config using the config manager
func WithProtocolConfig(protocolID string, protocolConfig config.ProtocolConfig) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		mockConfig := GetMockConfig(ctx)
		err := mockConfig.ConfigureProtocol(protocolID, protocolConfig)
		if err != nil {
			panic(fmt.Errorf("failed to configure protocol %s: %w", protocolID, err))
		}
		return ctx, nil
	}
}

// WithCustomProtocolConfig creates protocol config using builder pattern
func WithCustomProtocolConfig(protocolID string, builder *ConfigBuilder) TestContextBuilderOption {
	return WithProtocolConfig(protocolID, builder.Build())
}

// WithServiceConfig configures a service with the given config using the config manager
func WithServiceConfig(pluginName string, serviceName string, serviceConfig config.ServiceConfig) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		mockConfig := GetMockConfig(ctx)
		err := mockConfig.ConfigureService(pluginName, serviceName, serviceConfig)
		if err != nil {
			panic(fmt.Errorf("failed to configure service %s for plugin %s: %w", serviceName, pluginName, err))
		}
		return ctx, nil
	}
}

// WithCustomServiceConfig creates service config using builder pattern
func WithCustomServiceConfig(pluginName string, serviceName string, builder *ConfigBuilder) TestContextBuilderOption {
	return WithServiceConfig(pluginName, serviceName, builder.Build())
}

// WithService creates a TestContextBuilderOption that wraps RegisterService
// to register and configure a Service for testing purposes.
func WithService(id string, factory core.ServiceFactory) TestContextBuilderOption {
	return func(tctx TestContext) (TestContext, error) {
		opts, err := RegisterService(tctx, id, factory)
		if err != nil {
			return tctx, err
		}
		return ProcessCtxOptions(tctx, opts...)
	}
}

// RegisterService registers a Service and wraps any returned context options for test context
func RegisterService(ctx TestContext, id string, factory core.ServiceFactory) (ctxOpts []TestContextBuilderOption, err error) {
	service, opts, err := factory()
	if err != nil {
		ctx.Logger().Error("Error building Service", zap.String("service", id), zap.Error(err))
		return nil, err
	}

	if service == nil {
		return nil, fmt.Errorf("service factory returned nil service")
	}

	// Register the instance locally and globally
	if err := registerServiceInstance(ctx, id, service); err != nil {
		return nil, fmt.Errorf("failed to register service: %w", err)
	}

	return WrapCoreOptions(opts), nil
}

// GetMockAccessService returns the mock access service from the context for testing
// Panics if the access service is not a mock
func GetMockAccessService(ctx core.Context) *MockAccessService {
	accessSvc := ctx.Service(core.ACCESS_SERVICE)
	if accessSvc == nil {
		panic("access service not found in context")
	}
	mockAccess, ok := accessSvc.(*MockAccessService)
	if !ok {
		// Note: NewMockAccessService returns a *testing.MockAccessService, not mocks.MockAccessService
		// The type assertion should be to the wrapper type.
		panic(fmt.Sprintf("access service is not a mock - expected *testing.MockAccessService, got %T", accessSvc))
	}
	return mockAccess
}

// GetMockAuthService returns the mock auth service from the context for testing
// Panics if the auth service is not a mock
func GetMockAuthService(ctx core.Context) *mocks.MockAuthService {
	authSvc := ctx.Service(core.AUTH_SERVICE)
	if authSvc == nil {
		panic("auth service not found in context")
	}
	mockAuth, ok := authSvc.(*mocks.MockAuthService)
	if !ok {
		panic(fmt.Sprintf("auth service is not a mock - expected *mocks.MockAuthService, got %T", authSvc))
	}
	return mockAuth
}

// GetMockContentScannerService returns the mock content scanner service from the context for testing
// Panics if the content scanner service is not a mock
func GetMockContentScannerService(ctx core.Context) *mocks.MockContentScannerService {
	contentScannerSvc := ctx.Service(core.CONTENT_SCANNER_SERVICE)
	if contentScannerSvc == nil {
		panic("content scanner service not found in context")
	}
	mockContentScanner, ok := contentScannerSvc.(*mocks.MockContentScannerService)
	if !ok {
		panic(fmt.Sprintf("content scanner service is not a mock - expected *mocks.MockContentScannerService, got %T", contentScannerSvc))
	}
	return mockContentScanner
}

// GetMockCronService returns the mock cron service from the context for testing
// Panics if the cron service is not a mock
func GetMockCronService(ctx core.Context) *mocks.MockCronService {
	cronSvc := ctx.Service(core.CRON_SERVICE)
	if cronSvc == nil {
		panic("cron service not found in context")
	}
	mockCron, ok := cronSvc.(*mocks.MockCronService)
	if !ok {
		panic(fmt.Sprintf("cron service is not a mock - expected *mocks.MockCronService, got %T", cronSvc))
	}
	return mockCron
}

// GetMockHTTPService returns the mock http service from the context for testing
// Panics if the http service is not a mock
func GetMockHTTPService(ctx core.Context) *MockHTTPService {
	httpSvc := ctx.Service(core.HTTP_SERVICE)
	if httpSvc == nil {
		panic("http service not found in context")
	}
	mockHTTP, ok := httpSvc.(*MockHTTPService)
	if !ok {
		panic(fmt.Sprintf("http service is not a mock - expected *testing.MockHTTPService, got %T", httpSvc))
	}
	return mockHTTP
}

// GetMockHashMappingService returns the mock hash mapping service from the context for testing
// Panics if the hash mapping service is not a mock
func GetMockHashMappingService(ctx core.Context) *mocks.MockHashMappingService {
	hashMappingSvc := ctx.Service(core.HASH_MAPPING_SERVICE)
	if hashMappingSvc == nil {
		panic("hash mapping service not found in context")
	}
	mockHashMapping, ok := hashMappingSvc.(*mocks.MockHashMappingService)
	if !ok {
		panic(fmt.Sprintf("hash mapping service is not a mock - expected *mocks.MockHashMappingService, got %T", hashMappingSvc))
	}
	return mockHashMapping
}

// GetMockMailerService returns the mock mailer service from the context for testing
// Panics if the mailer service is not a mock
func GetMockMailerService(ctx core.Context) *mocks.MockMailerService {
	mailerSvc := ctx.Service(core.MAILER_SERVICE)
	if mailerSvc == nil {
		panic("mailer service not found in context")
	}
	mockMailer, ok := mailerSvc.(*mocks.MockMailerService)
	if !ok {
		panic(fmt.Sprintf("mailer service is not a mock - expected *mocks.MockMailerService, got %T", mailerSvc))
	}
	return mockMailer
}

// GetMockOTPService returns the mock otp service from the context for testing
// Panics if the otp service is not a mock
func GetMockOTPService(ctx core.Context) *mocks.MockOTPService {
	otpSvc := ctx.Service(core.OTP_SERVICE)
	if otpSvc == nil {
		panic("otp service not found in context")
	}
	mockOTP, ok := otpSvc.(*mocks.MockOTPService)
	if !ok {
		panic(fmt.Sprintf("otp service is not a mock - expected *mocks.MockOTPService, got %T", otpSvc))
	}
	return mockOTP
}

// GetMockPasswordResetService returns the mock password reset service from the context for testing
// Panics if the password reset service is not a mock
func GetMockPasswordResetService(ctx core.Context) *mocks.MockPasswordResetService {
	passwordResetSvc := ctx.Service(core.PASSWORD_RESET_SERVICE)
	if passwordResetSvc == nil {
		panic("password reset service not found in context")
	}
	mockPasswordReset, ok := passwordResetSvc.(*mocks.MockPasswordResetService)
	if !ok {
		panic(fmt.Sprintf("password reset service is not a mock - expected *mocks.MockPasswordResetService, got %T", passwordResetSvc))
	}
	return mockPasswordReset
}

// GetMockPinService returns the mock pin service from the context for testing
// Panics if the pin service is not a mock
func GetMockPinService(ctx core.Context) *mocks.MockPinService {
	pinSvc := ctx.Service(core.PIN_SERVICE)
	if pinSvc == nil {
		panic("pin service not found in context")
	}
	mockPin, ok := pinSvc.(*mocks.MockPinService)
	if !ok {
		panic(fmt.Sprintf("pin service is not a mock - expected *mocks.MockPinService, got %T", pinSvc))
	}
	return mockPin
}

// GetMockRequestService returns the mock request service from the context for testing
// Panics if the request service is not a mock
func GetMockRequestService(ctx core.Context) *mocks.MockRequestService {
	requestSvc := ctx.Service(core.REQUEST_SERVICE)
	if requestSvc == nil {
		panic("request service not found in context")
	}
	mockRequest, ok := requestSvc.(*mocks.MockRequestService)
	if !ok {
		panic(fmt.Sprintf("request service is not a mock - expected *mocks.MockRequestService, got %T", requestSvc))
	}
	return mockRequest
}

// GetMockRenterService returns the mock renter service from the context for testing
// Panics if the renter service is not a mock
func GetMockRenterService(ctx core.Context) *mocks.MockRenterService {
	renterSvc := ctx.Service(core.RENTER_SERVICE)
	if renterSvc == nil {
		panic("renter service not found in context")
	}
	mockRenter, ok := renterSvc.(*mocks.MockRenterService)
	if !ok {
		panic(fmt.Sprintf("renter service is not a mock - expected *mocks.MockRenterService, got %T", renterSvc))
	}
	return mockRenter
}

// GetMockStorageService returns the mock storage service from the context for testing
// Panics if the storage service is not a mock
func GetMockStorageService(ctx core.Context) *mocks.MockStorageService {
	storageSvc := ctx.Service(core.STORAGE_SERVICE)
	if storageSvc == nil {
		panic("storage service not found in context")
	}
	mockStorage, ok := storageSvc.(*mocks.MockStorageService)
	if !ok {
		panic(fmt.Sprintf("storage service is not a mock - expected *mocks.MockStorageService, got %T", storageSvc))
	}
	return mockStorage
}

// GetMockTUSService returns the mock tus service from the context for testing
// Panics if the tus service is not a mock
func GetMockTUSService(ctx core.Context) *mocks.MockTUSService {
	tusSvc := ctx.Service(core.TUS_SERVICE)
	if tusSvc == nil {
		panic("tus service not found in context")
	}
	mockTUS, ok := tusSvc.(*mocks.MockTUSService)
	if !ok {
		panic(fmt.Sprintf("tus service is not a mock - expected *mocks.MockTUSService, got %T", tusSvc))
	}
	return mockTUS
}

// GetMockUserService returns the mock user service from the context for testing
// Panics if the user service is not a mock
func GetMockUserService(ctx core.Context) *mocks.MockUserService {
	userSvc := ctx.Service(core.USER_SERVICE)
	if userSvc == nil {
		panic("user service not found in context")
	}
	mockUser, ok := userSvc.(*mocks.MockUserService)
	if !ok {
		panic(fmt.Sprintf("user service is not a mock - expected *mocks.MockUserService, got %T", userSvc))
	}
	return mockUser
}

// GetMockWorkflowService returns the mock workflow service from the context for testing
// Panics if the workflow service is not a mock
func GetMockWorkflowService(ctx core.Context) *mocks.MockWorkflowService {
	workflowSvc := ctx.Service(core.WORKFLOW_SERVICE)
	if workflowSvc == nil {
		panic("workflow service not found in context")
	}
	mockWorkflow, ok := workflowSvc.(*mocks.MockWorkflowService)
	if !ok {
		panic(fmt.Sprintf("workflow service is not a mock - expected *mocks.MockWorkflowService, got %T", workflowSvc))
	}
	return mockWorkflow
}

// WithMockUploadService adds a mock UploadService to the test context.
func WithMockUploadService() TestContextBuilderOption {
	return WithMockService(core.UPLOAD_SERVICE, func(tb TB, _ TestContext) any {
		return mocks.NewMockUploadService(tb)
	})
}

// WithMockS3 configures a test context to use a gofakes3 instance.
// It launches a gofakes3 server, configures the S3 config, and registers cleanup.
// bucketName: The name of the S3 bucket to create.
// configureS3Config: Optional function to further configure the S3Config.
func WithMockS3() TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		// Create a gofakes3 instance
		tempDir, err := os.MkdirTemp("", "portal-s3-")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp dir: %w", err)
		}

		backend, err := s3afero.SingleBucket("fakebucket", afero.NewBasePathFs(afero.NewOsFs(), tempDir), nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create s3 backend: %w", err)
		}
		faker := gofakes3.New(backend)

		// Launch the gofakes3 server
		httpHandler := faker.Server()
		server := &http.Server{Handler: httpHandler}

		// Find an available port
		listener, err := GetFreeListener()
		if err != nil {
			return nil, fmt.Errorf("failed to get free port: %w", err)
		}

		endpoint := fmt.Sprintf("http://%s", listener.Addr().String())

		// Channel to signal server is ready
		ready := make(chan struct{})

		go func() {
			close(ready) // Signal server is starting
			if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
				ctx.Logger().Error("gofakes3 server failed", zap.Error(err))
			}
		}()

		// Wait for server to be ready
		select {
		case <-ready:
			// Brief delay to ensure server is listening
			time.Sleep(10 * time.Millisecond)
		case <-time.After(100 * time.Millisecond):
			return nil, fmt.Errorf("timeout waiting for S3 server to start")
		}

		ctx.RegisterCleanup(func() {
			// Shutdown server
			if err = server.Shutdown(ctx.GetContext()); err != nil {
				ctx.Logger().Error("failed to shutdown gofakes3 server",
					zap.Error(err))
			}

			// Remove temp directory
			if err = os.RemoveAll(tempDir); err != nil {
				ctx.Logger().Error("failed to remove temp directory",
					zap.String("path", tempDir),
					zap.Error(err))
			}
		})

		// Configure the S3 config
		s3Config := &config.S3Config{
			BufferBucket: "fakebucket",
			Endpoint:     endpoint,
			Region:       "us-east-1",
			AccessKey:    "FAKEACCESSKEY",
			SecretKey:    "FAKESECRETKEY",
		}

		// Set the S3 config in the test context
		mockConfig := GetMockConfig(ctx)
		configValues := map[string]interface{}{
			"core.s3.buffer_bucket": s3Config.BufferBucket,
			"core.s3.endpoint":      s3Config.Endpoint,
			"core.s3.region":        s3Config.Region,
			"core.s3.access_key":    s3Config.AccessKey,
			"core.s3.secret_key":    s3Config.SecretKey,
		}

		for key, value := range configValues {
			if err := mockConfig.Set(ctx, key, value); err != nil {
				return nil, fmt.Errorf("failed to set s3 config: %w", err)
			}
		}

		return ctx, nil
	}
}

// WithSQLitePluginMigrations registers a mock plugin with the given ID and SQLite migrations.
func WithSQLitePluginMigrations(pluginID string, migrationsFS fs.FS) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		pluginInfo := core.PluginInfo{
			ID:      pluginID,
			Version: build.New("", "", "", "", "", "", ""),
			Migrations: core.DBMigration{
				core.DB_TYPE_SQLITE: migrationsFS,
			},
			WebBundles: core.NewWebBundles(core.NewWebBundle(fstest.MapFS{})),
		}

		core.RegisterPlugin(pluginInfo)
		return ctx, nil
	}
}

// WithAPI creates a TestContextBuilderOption that wraps RegisterAPI
// to register and configure an API for testing purposes.
func WithAPI(id string, factory core.APIFactory) TestContextBuilderOption {
	return func(tctx TestContext) (TestContext, error) {
		opts, err := RegisterAPI(tctx, id, factory)
		if err != nil {
			return tctx, err
		}
		return ProcessCtxOptions(tctx, opts...)
	}
}

// registerProtocolWithHelper is a helper function to register a protocol.
func registerProtocolWithHelper(tctx TestContext, id string, factory core.ProtocolFactory) (TestContext, error) {
	opts, err := RegisterProtocol(tctx, id, factory)
	if err != nil {
		return tctx, err
	}
	return ProcessCtxOptions(tctx, opts...)
}

// WithProtocol creates a TestContextBuilderOption that registers and configures a Protocol.
func WithProtocol(id string, factory core.ProtocolFactory) TestContextBuilderOption {
	return func(tctx TestContext) (TestContext, error) {
		return registerProtocolWithHelper(tctx, id, factory)
	}
}

// WithMockProtocol creates a TestContextBuilderOption that registers a mock protocol.
// It takes a protocol name and a callback function that allows configuring the mock protocol.
func WithMockProtocol(protocolName string, configureMock ...func(protocol *MockProtocol)) TestContextBuilderOption {
	return func(tctx TestContext) (TestContext, error) {
		// Create a new mock protocol
		mockProtocol := NewMockProtocol(tctx.T(), protocolName)

		// Configure the mock using the provided callback
		if len(configureMock) > 0 {
			for _, v := range configureMock {
				if v != nil {
					v(mockProtocol)
				}
			}

		}

		// Create a protocol factory that returns the configured mock
		protocolFactory := func() (core.Protocol, []core.ContextBuilderOption, error) {
			return mockProtocol, nil, nil // No additional context options for mock protocols
		}

		return registerProtocolWithHelper(tctx, protocolName, protocolFactory)
	}
}

// RegisterAPI registers an API and wraps any returned context options for test context
func RegisterAPI(ctx TestContext, id string, factory core.APIFactory) (ctxOpts []TestContextBuilderOption, err error) {
	api, opts, err := factory()
	if err != nil {
		ctx.Logger().Error("Error building API", zap.String("plugin", id), zap.Error(err))
		return nil, err
	}

	if api == nil {
		ctx.Logger().Error("Error building API", zap.String("plugin", id), zap.Error(err))
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
				zap.String("api", ext.TargetAPI()),
				zap.String("extension", fmt.Sprintf("%T", ext)))

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

// ConfigureProtocols configures all registered protocols. Unlike ConfigureAPIs,
// protocols don't return context options during initialization.
func ConfigureProtocols(ctx TestContext) error {
	for name, proto := range core.GetProtocols() {
		// Configure protocol through config manager
		err := ctx.Config().ConfigureProtocol(name, proto.Config())
		if err != nil {
			ctx.Logger().Error("Error configuring protocol",
				zap.String("protocol", proto.Name()),
				zap.Error(err))
			return err
		}

		// Initialize protocol if it implements ProtocolInit
		if initProto, ok := proto.(core.ProtocolInit); ok {
			err = initProto.Init(ctx)
			if err != nil {
				ctx.Logger().Error("Error initializing protocol",
					zap.String("protocol", proto.Name()),
					zap.Error(err))
				return err
			}
		}
	}

	return nil
}

func ConfigureAPIs(ctx TestContext) ([]TestContextBuilderOption, error) {
	var opts []TestContextBuilderOption

	for name, api := range core.GetAPIs() {
		// Configure API through config manager
		err := ctx.Config().ConfigureAPI(name, api.Config())
		if err != nil {
			ctx.Logger().Error("Error configuring API",
				zap.String("api", api.Name()),
				zap.Error(err))
			return nil, err
		}

		// Initialize API if it implements APIInit
		if initApi, ok := api.(core.APIInit); ok {
			apiOpts, err := initApi.Init()
			if err != nil {
				ctx.Logger().Error("Error initializing API",
					zap.String("api", api.Name()),
					zap.Error(err))
				return nil, err
			}
			opts = append(opts, WrapCoreOptions(apiOpts)...)
		}
	}

	return opts, nil
}

// RunMigrations executes database migrations for the test context
func RunMigrations(ctx TestContext) error {
	migrationManager, err := portal.NewMigrationManager(ctx)
	if err != nil {
		return fmt.Errorf("failed to create migration manager: %w", err)
	}

	if err = migrationManager.RunMigrations(ctx.DB()); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

var (
	serviceConfigType = pkgReflect.GetInterfaceType((*config.ServiceConfig)(nil))
)

// configureService handles the configuration of a single service, including:
// - Type checking and interface compliance verification
// - Pointer/non-pointer conversion handling
// - Plugin association validation
// - Actual configuration through the config manager
func configureService(ctx TestContext, svcInfo core.ServiceInfo, svc any) error {
	// Check if service implements ServiceConfig interface
	if !pkgReflect.CheckInterface(svc, serviceConfigType) {
		return nil // Skip services without config
	}

	// Get the config object from the service
	cfgResult := svc.(config.ServiceConfig)

	// Ensure the config type is compliant with ServiceConfig interface
	// This handles cases where the service returns a non-pointer but needs pointer semantics
	compliantCfg, isCompliant := pkgReflect.EnsureCompliantType(cfgResult, serviceConfigType)
	if !isCompliant {
		ctx.Logger().Error(config.ErrInvalidServiceConfig.Error()+" (type does not implement interface)",
			zap.String("service", svcInfo.ID),
			zap.Any("config_type", reflect.TypeOf(cfgResult)))
		return config.ErrInvalidServiceConfig
	}

	// Log if we had to use a pointer to a copy due to non-addressable value
	if reflect.ValueOf(cfgResult).Kind() != reflect.Pointer &&
		reflect.ValueOf(compliantCfg).Kind() == reflect.Pointer &&
		!reflect.ValueOf(cfgResult).CanAddr() {
		ctx.Logger().Warn("Config value was not addressable; using pointer to a copy for configuration.",
			zap.String("service", svcInfo.ID))
	}

	// Final type assertion after compliance check
	svcConfig, ok := compliantCfg.(config.ServiceConfig)
	if !ok {
		ctx.Logger().Error("Internal error: compliant config object could not be cast to ServiceConfig",
			zap.String("service", svcInfo.ID),
			zap.Any("config_type", reflect.TypeOf(compliantCfg)))
		return config.ErrInvalidServiceConfig
	}

	// Skip core services (they're configured differently)
	if core.IsCoreService(svcInfo.ID) {
		return nil
	}

	// Get plugin association for the service
	pluginName := core.GetPluginForService(svcInfo.ID)
	if pluginName == "" {
		ctx.Logger().Error("Service has no plugin association",
			zap.String("service", svcInfo.ID))
		return config.ErrInvalidServiceConfig
	}

	// Actually configure the service through the config manager
	if err := ctx.Config().ConfigureService(pluginName, svcInfo.ID, svcConfig); err != nil {
		ctx.Logger().Error("Error configuring service",
			zap.String("service", svcInfo.ID),
			zap.Error(err))
		return err
	}

	return nil
}

// ConfigureServices iterates through all registered services and configures them
// using their ServiceConfig implementations. Handles core services differently
// from plugin services.
func ConfigureServices(ctx TestContext) error {
	for _, svcInfo := range core.GetServices() {
		svc := ctx.Service(svcInfo.ID)
		if svc == nil {
			continue // Skip unregistered services
		}

		if err := configureService(ctx, svcInfo, svc); err != nil {
			return err
		}

		if _, ok := svc.(core.ServiceInit); ok {
			err := svc.(core.ServiceInit).Init()
			if err != nil {
				ctx.Logger().Error("Error initializing service",
					zap.String("service", svcInfo.ID),
					zap.Error(err))
				return err
			}
		}
	}

	return nil
}

func ConfigureAPIRoutes(ctx TestContext) error {
	accessSvc := core.GetService[core.AccessService](ctx, core.ACCESS_SERVICE)
	if accessSvc == nil {
		return fmt.Errorf("AccessService not found in context, cannot configure API routes")
	}

	gRouter := ctx.Router()

	for _, api := range core.GetAPIs() {
		subdomain := api.Subdomain()
		domain := fmt.Sprintf("%s.%s", api.Subdomain(), ctx.Config().Config().Core.Domain)

		if subdomain == "" {
			domain = ctx.Config().Config().Core.Domain
		}

		// Create a subrouter for this API's domain
		hostRouter, err := gRouter.Host(domain)
		if err != nil {
			return fmt.Errorf("failed to create host router for API %s: %w", api.Name(), err)
		}

		// Configure the main API using the gswagger router
		err = api.Configure(hostRouter, accessSvc)
		if err != nil {
			return err
		}

		// Apply any registered extensions using the *same* gswagger router
		for _, ext := range core.GetAPIExtensions(api.Name()) {
			ctx.Logger().Info("Applying API extension",
				zap.String("api", ext.TargetAPI()),
				zap.String("extension", fmt.Sprintf("%T", ext)))

			// The APIExtension.Configure method signature needs to change
			// This part seems like it might be related to a different system or a work-in-progress.
			// For the purpose of providing the requested testing.go file, I'll keep the existing logic
			// but note that the Configure method signature in MockAPIExtension doesn't match core.APIExtension.
			// If this needs to be functional, the mock or the interface might need adjustment.
			if err = ext.Configure(hostRouter, accessSvc); err != nil {
				return fmt.Errorf("failed to configure API extension: %w", err)
			}
		}
	}

	return nil
}

// BootEnvironment creates and initializes a TestContext with common services and configurations.
// It handles resetting state, creating the context, applying options, and booting the environment.
// This function is called internally by RunTestCase and RunTestCaseWithDB.
// WithNoFireBootCompleteEvent disables automatic firing of boot complete event
func WithNoFireBootCompleteEvent() TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		if tctx, ok := ctx.(*testContext); ok {
			tctx.SetFireBootComplete(false)
		}
		return ctx, nil
	}
}

func BootEnvironment(tb TB, ctx TestContext) error {
	// Phase 1: Registration
	// Process all context options (default, global, and test case specific)
	ctx, err := ProcessCtxOptions(ctx, GetCombinedTestContextOptions(tb)...)
	if err != nil {
		return fmt.Errorf("registration phase failed: %w", err)
	}

	// Process startup functions (DB connection, etc)
	if err := ProcessStartupFuncs(ctx); err != nil {
		return fmt.Errorf("startup functions failed: %w", err)
	}

	// Phase 2: Configuration
	if err := ConfigureProtocols(ctx); err != nil {
		return fmt.Errorf("protocol configuration failed: %w", err)
	}

	apiOpts, err := ConfigureAPIs(ctx)
	if err != nil {
		return fmt.Errorf("API configuration failed: %w", err)
	}

	if err := ConfigureServices(ctx); err != nil {
		return fmt.Errorf("service configuration failed: %w", err)
	}

	// Phase 3: Initialization
	if _, err := ProcessCtxOptions(ctx, apiOpts...); err != nil {
		return fmt.Errorf("API initialization failed: %w", err)
	}

	if err := ConfigureAPIRoutes(ctx); err != nil {
		return fmt.Errorf("API route configuration failed: %w", err)
	}

	// Register workflows from all protocols
	if err := ConfigureProtocolWorkflows(ctx); err != nil {
		return fmt.Errorf("failed to configure protocol workflows: %w", err)
	}

	// Fire boot complete event if enabled
	if tctx, ok := ctx.(*testContext); ok && tctx.FireBootComplete() {
		if err := ctx.Fire(pevent.EVENT_BOOT_COMPLETE, pevent.NewBootCompleteEvent(ctx)); err != nil {
			return fmt.Errorf("failed to fire boot complete event: %w", err)
		}
	}

	return nil
}

// WithDomain configures the test context with a specific domain
func WithDomain(domain string) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		if err := ctx.Config().Set(ctx, "core.domain", domain); err != nil {
			return nil, err
		}
		return ctx, nil
	}
}

// WithSeedPhrase configures the test context with a specific seed phrase
func WithSeedPhrase(seedPhrase string) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		if err := ctx.Config().Set(ctx, "core.identity", seedPhrase); err != nil {
			return nil, err
		}
		return ctx, nil
	}
}

// WithRandomSeedPhrase configures the test context with a randomly generated seed phrase
func WithRandomSeedPhrase() TestContextBuilderOption {
	return WithSeedPhrase(wallet.NewSeedPhrase())
}

// WithMockDB adds a mock database to the test context
func WithMockDB(db *gorm.DB) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		ctx.SetDB(db)
		ctx.RegisterCleanup(func() {
			sqlDB, err := db.DB()
			if err == nil {
				_ = sqlDB.Close()
			}
		})

		return ctx, nil
	}
}

// SetupSQLMock creates a new sqlmock and configures a test context with it.
// It returns a test context with the mock database and the sqlmock interface.
func SetupSQLMock(t TB) (TestContext, sqlmock.Sqlmock) {
	// Create a mock database and gorm instance
	mockDB, _mock := testingDb.NewSQLMock(t.(*testing.T))

	// Create the test context with the mock DB
	ctx := NewTestContext(t, WithMockDB(mockDB))

	return ctx, _mock
}

// WithInMemorySQLite configures the test context to use an in-memory SQLite database
// ShutdownTestContext is a helper for tests that calls all registered exit functions
// and cleans up resources without exiting the process.
func ShutdownTestContext(ctx TestContext) {
	// Cancel the context first to signal shutdown
	ctx.Cancel()

	// Wait for context cancellation to propagate
	<-ctx.Done()

	// Run all registered exit functions
	if err := ProcessExitFuncs(ctx); err != nil {
		ctx.Logger().Error("Error during test shutdown", zap.Error(err))
	}

	// Perform any additional cleanup
	ctx.Teardown()
}

func WithSQLite(tb TB) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		// Generate temp file path
		tempFile, err := os.CreateTemp("", "portal-test-*.db")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp db file: %w", err)
		}
		err = tempFile.Close()
		if err != nil {
			return nil, err
		}

		// Update config with SQLite type and temp file path
		err = ctx.Config().Set(ctx, "core.db.type", "sqlite")
		if err != nil {
			return nil, fmt.Errorf("failed to set database type: %w", err)
		}
		err = ctx.Config().Set(ctx, "core.db.file", tempFile.Name())
		if err != nil {
			return nil, fmt.Errorf("failed to set database file: %w", err)
		}

		// Register cleanup to remove temp file
		ctx.RegisterCleanup(func() {
			err = os.Remove(tempFile.Name())
			if err != nil {
				ctx.Logger().Error("Failed to remove temp SQLite file",
					zap.String("path", tempFile.Name()),
					zap.Error(err))
			}
		})

		// Add a startup function that will create and connect the DB
		startupOpt := core.ContextWithStartupFunc(func(ctx core.Context) error {
			provider := testingDb.NewTestSQLiteProvider(ctx.(TestContext))

			// Connect to the database
			db, err := provider.Connect(ctx.Logger())
			if err != nil {
				return fmt.Errorf("failed to connect to SQLite: %w", err)
			}

			// Set the DB on the context
			ctx.(TestContext).SetDB(db)

			// Register cleanup
			ctx.OnExit(func(c core.Context) error {
				return provider.Close()
			})

			return nil
		})

		// Apply the startup option
		return ProcessCtxOptions(ctx, WrapCoreOption(startupOpt))
	}
}

// SetupTestEnvironment creates and initializes a TestContext with common services and configurations.
// It handles resetting state, creating the context, applying options, and booting the environment.
func SetupTestEnvironment(tb TB) (TestContext, error) {
	tb.Helper()

	// Reset test case state (global state remains)
	ResetAllState()

	// Create base test context
	ctx := SetupTest(tb)

	// Process all options (defaults + global + test case)
	ctx, err := ProcessCtxOptions(ctx, GetCombinedTestContextOptions(tb)...)
	if err != nil {
		return ctx, fmt.Errorf("failed to apply test options: %w", err)
	}

	// Boot the environment
	err = BootEnvironment(tb, ctx)
	if err != nil {
		return ctx, fmt.Errorf("failed to boot test environment: %w", err)
	}

	return ctx, nil
}

// WithDefaultRouter creates a default router and adds it to the context.
func WithDefaultRouter(tb TB) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		_router, err := router.NewRouter(router.APIInfo().Title("Test API").Version("1.0.0"))
		if err != nil {
			return nil, fmt.Errorf("failed to create test router: %v", err)
		}
		return WithRouter(_router)(ctx)
	}
}

// WithCoreEvents ensures core events are registered in the test environment
func WithCoreEvents() TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		// Reset events
		ctx.ResetEvents()

		return ctx, nil
	}
}

// ConfigureProtocolWorkflows registers all workflows from all protocols with the workflow service
func ConfigureProtocolWorkflows(ctx core.Context) error {
	workflowSvc := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)
	for _, proto := range core.GetProtocols() {
		for _, workflow := range proto.Workflows() {
			if err := workflowSvc.RegisterWorkflow(workflow.Name, workflow.Steps, workflow.AutoTriggerFirstStep); err != nil {
				return fmt.Errorf("failed to register workflow %s for protocol %s: %w", workflow.Name, proto.Name(), err)
			}
		}
	}
	return nil
}

// registerServiceInstance registers a service instance both locally in the test context
// and globally with the core framework.
func registerServiceInstance(ctx TestContext, id string, instance any) error {
	if instance == nil {
		return fmt.Errorf("service instance for '%s' is nil", id)
	}

	// Register locally in test context
	ctx.RegisterService(id, instance)

	// Register globally if it implements core.Service
	if svc, ok := instance.(core.Service); ok {
		core.UnregisterService(id)
		core.RegisterService(core.ServiceInfo{
			ID: id,
			Factory: func() (core.Service, []core.ContextBuilderOption, error) {
				return svc, nil, nil
			},
		})
	} else {
		ctx.Logger().Warn("Service instance does not implement core.Service; global registration skipped",
			zap.String("service", id),
			zap.Any("type", reflect.TypeOf(instance)))
	}

	return nil
}
