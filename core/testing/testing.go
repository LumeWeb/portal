// Package testing provides utilities for testing core components
package testing

import (
	"context"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gookit/event"
	"go.lumeweb.com/portal"
	router "go.lumeweb.com/portal-router"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/testing/db"
	"go.lumeweb.com/portal/core/testing/mocks"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"gorm.io/gorm"
	"sync"
	"testing"
	"time"
)

var (
	testCtxOpts       []TestContextBuilderOption
	testCtxOptsMu     sync.RWMutex
	runDBMigrations   bool = false
	runDBMigrationsMu sync.RWMutex
	setupMockDB       bool = false
	setupMockDBMu     sync.RWMutex
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
	Router() router.Router
	SetDB(*gorm.DB) // Set the database instance
}

// ResetAllState resets all global state in the core package and testing package
func ResetAllState() {
	core.ResetState()
	ClearTestContextOptions()
	DisableDBMigrations()
	DisableMockDB()
}

// EnableDBMigrations enables running DB migrations during test context initialization
func EnableDBMigrations() {
	runDBMigrationsMu.Lock()
	defer runDBMigrationsMu.Unlock()
	runDBMigrations = true
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
	event        *event.Manager
	router       router.Router
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

	// Apply default options first, then any custom options
	finalCtx, err := ProcessCtxOptions(testCtx, append(DefaultTestContextOptions(tb), opts...)...)
	if err != nil {
		// Log the error and return a context that will cause subsequent test failures
		testCtx.Logger().Error("Failed to initialize test context", zap.Error(err))
		return testCtx // Return the partially built context to allow test failures
	}

	return finalCtx
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

// WithMockService adds a mock service to the test context
func WithMockService(id string, service core.Service) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		ctx.RegisterService(id, service)
		return ctx, nil
	}
}

// WithRouter adds a router to the test context
func WithRouter(r router.Router) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		ctx.(*testContext).router = r
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

func (c *testContext) Router() router.Router {
	return c.router
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
		// Type assert back to TestContext
		if tc, ok := returnCtx.(TestContext); ok {
			newCtx = tc
		} else {
			// This should not happen if options are correctly implemented
			return newCtx, fmt.Errorf("context type changed unexpectedly after option application")
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

	// Process exit functions - these are registered during startup, so call them after startup
	// Note: The original code called ProcessExitFuncs here, which is incorrect as exit funcs are meant for teardown.
	// I'm removing the call here, assuming teardown is handled by TB.Cleanup or explicit calls.
	// If you intended to run exit funcs *during* InitContext for some reason, please clarify.

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

// --- Modular Mock Service Setup Functions ---

// WithMockAccessService adds a mock AccessService to the test context.
func WithMockAccessService(tb TB) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		mockAccessService := NewMockAccessService(tb)
		ctx.RegisterService(core.ACCESS_SERVICE, mockAccessService)
		return ctx, nil
	}
}

// WithMockAuthService adds a mock AuthService to the test context.
func WithMockAuthService(tb TB) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		mockAuthService := mocks.NewMockAuthService(tb)
		ctx.RegisterService(core.AUTH_SERVICE, mockAuthService)
		return ctx, nil
	}
}

// WithMockConfigService adds a mock ConfigService to the test context.
func WithMockConfigService(tb TB) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		mockConfigService := mocks.NewMockConfigService(tb)
		ctx.RegisterService(core.CONFIG_SERVICE, mockConfigService)
		return ctx, nil
	}
}

// WithMockContentScannerService adds a mock ContentScannerService to the test context.
func WithMockContentScannerService(tb TB) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		mockContentScannerService := mocks.NewMockContentScannerService(tb)
		ctx.RegisterService(core.CONTENT_SCANNER_SERVICE, mockContentScannerService)
		return ctx, nil
	}
}

// WithMockCronService adds a mock CronService to the test context.
func WithMockCronService(tb TB) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		mockCronService := mocks.NewMockCronService(tb)
		ctx.RegisterService(core.CRON_SERVICE, mockCronService)
		return ctx, nil
	}
}

// WithMockHTTPService adds a mock HTTPService to the test context.
func WithMockHTTPService(tb TB) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		mockHTTPService := NewMockHTTPService(tb).WithRouter(ctx.Router()).WithConfigManager(ctx.Config())
		ctx.RegisterService(core.HTTP_SERVICE, mockHTTPService)
		return ctx, nil
	}
}

// WithMockHashMappingService adds a mock HashMappingService to the test context.
func WithMockHashMappingService(tb TB) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		mockHashMappingService := mocks.NewMockHashMappingService(tb)
		ctx.RegisterService(core.HASH_MAPPING_SERVICE, mockHashMappingService)
		return ctx, nil
	}
}

// WithMockMailerService adds a mock MailerService to the test context.
func WithMockMailerService(tb TB) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		mockMailerService := mocks.NewMockMailerService(tb)
		ctx.RegisterService(core.MAILER_SERVICE, mockMailerService)
		return ctx, nil
	}
}

// WithMockOTPService adds a mock OTPService to the test context.
func WithMockOTPService(tb TB) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		mockOTPService := mocks.NewMockOTPService(tb)
		ctx.RegisterService(core.OTP_SERVICE, mockOTPService)
		return ctx, nil
	}
}

// WithMockPasswordResetService adds a mock PasswordResetService to the test context.
func WithMockPasswordResetService(tb TB) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		mockPasswordResetService := mocks.NewMockPasswordResetService(tb)
		ctx.RegisterService(core.PASSWORD_RESET_SERVICE, mockPasswordResetService)
		return ctx, nil
	}
}

// WithMockPinService adds a mock PinService to the test context.
func WithMockPinService(tb TB) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		mockPinService := mocks.NewMockPinService(tb)
		ctx.RegisterService(core.PIN_SERVICE, mockPinService)
		return ctx, nil
	}
}

// WithMockRequestService adds a mock RequestService to the test context.
func WithMockRequestService(tb TB) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		mockRequestService := mocks.NewMockRequestService(tb)
		ctx.RegisterService(core.REQUEST_SERVICE, mockRequestService)
		return ctx, nil
	}
}

// WithMockRenterService adds a mock RenterService to the test context.
func WithMockRenterService(tb TB) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		mockRenterService := mocks.NewMockRenterService(tb)
		ctx.RegisterService(core.RENTER_SERVICE, mockRenterService)
		return ctx, nil
	}
}

// WithMockStorageService adds a mock StorageService to the test context.
func WithMockStorageService(tb TB) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		mockStorageService := mocks.NewMockStorageService(tb)
		ctx.RegisterService(core.STORAGE_SERVICE, mockStorageService)
		return ctx, nil
	}
}

// WithMockTUSService adds a mock TUSService to the test context.
func WithMockTUSService(tb TB) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		mockTUSService := mocks.NewMockTUSService(tb)
		ctx.RegisterService(core.TUS_SERVICE, mockTUSService)
		return ctx, nil
	}
}

// WithMockUserService adds a mock UserService to the test context.
func WithMockUserService(tb TB) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		mockUserService := mocks.NewMockUserService(tb)
		ctx.RegisterService(core.USER_SERVICE, mockUserService)
		return ctx, nil
	}
}

// WithMockWorkflowService adds a mock WorkflowService to the test context.
func WithMockWorkflowService(tb TB) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		mockWorkflowService := mocks.NewMockWorkflowService(tb)
		ctx.RegisterService(core.WORKFLOW_SERVICE, mockWorkflowService)
		return ctx, nil
	}
}

// --- Default Test Context Options ---

// DefaultTestContextOptions returns the default options used for new test contexts.
// These options include mock implementations of common core services.
func DefaultTestContextOptions(tb TB) []TestContextBuilderOption {
	_router, err := router.NewRouter(router.APIInfo().Title("Test API").Version("1.0.0"))
	if err != nil {
		panic(fmt.Sprintf("failed to create test router: %v", err))
	}

	opts := []TestContextBuilderOption{
		WithMockAccessService(tb),
		WithRouter(_router),
		WithMockAuthService(tb),
		WithMockConfigService(tb),
		WithMockContentScannerService(tb),
		WithMockCronService(tb),
		WithMockHTTPService(tb),
		WithMockHashMappingService(tb),
		WithMockMailerService(tb),
		WithMockOTPService(tb),
		WithMockPasswordResetService(tb),
		WithMockPinService(tb),
		WithMockRequestService(tb),
		WithMockRenterService(tb),
		WithMockStorageService(tb),
		WithMockTUSService(tb),
		WithMockUserService(tb),
		WithMockWorkflowService(tb),
	}

	if ShouldSetupMockDB() {
		opts = append(opts, WithInMemorySQLite())
	}

	return opts
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

// RegisterEvents is a testing helper that registers events and stores them in the context
func RegisterEvents(ctx TestContext, events ...core.Eventer) []TestContextBuilderOption {
	// Register each event
	for _, e := range events {
		core.RegisterEvent(e.Name(), e)
	}

	// Wrap the events in context options using testing package's WrapCoreOptions
	return WrapCoreOptions([]core.ContextBuilderOption{
		core.ContextWithEvents(core.GetEvents()...),
	})
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

func ConfigureProtocols(ctx TestContext) error {
	for name, _proto := range core.GetProtocols() {
		err := ctx.Config().ConfigureProtocol(name, _proto.Config())
		if err != nil {
			ctx.Logger().Error("Error configuring protocol", zap.String("protocol", _proto.Name()), zap.Error(err))
			return err
		}

		if initProto, ok := _proto.(core.ProtocolInit); ok {
			if err := initProto.Init(ctx); err != nil {
				ctx.Logger().Error("Error initializing protocol", zap.String("protocol", _proto.Name()), zap.Error(err))
				return err
			}
		}
	}

	return nil
}

func ConfigureAPIs(ctx TestContext) error {
	for name, api := range core.GetAPIs() {
		err := ctx.Config().ConfigureAPI(name, api.Config())
		if err != nil {
			ctx.Logger().Error("Error configuring api", zap.String("api", api.Name()), zap.Error(err))
			return err
		}

		if initApi, ok := api.(core.APIInit); ok {
			opts, err := initApi.Init()
			if err != nil {
				ctx.Logger().Error("Error initializing api", zap.String("api", api.Name()), zap.Error(err))
				return err
			}

			AddTestContextOptions(WrapCoreOptions(opts)...)
		}
	}

	return nil
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
				zap.String("api", api.Name()),
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

func BootEnvironment(ctx TestContext) error {
	// InitContext includes processing default and provided options
	err := InitContext(ctx)
	if err != nil {
		return err
	}

	// Run migrations first if enabled
	if ShouldRunDBMigrations() {
		err = RunMigrations(ctx)
		if err != nil {
			return err
		}
	}

	// Then configure protocols and APIs
	err = ConfigureProtocols(ctx)
	if err != nil {
		return err
	}

	err = ConfigureAPIs(ctx)
	if err != nil {
		return err
	}

	// Re-process context options after ConfigureAPIs
	ctx, err = ProcessCtxOptions(ctx, GetTestContextOptions()...)
	if err != nil {
		return err
	}

	err = ConfigureAPIRoutes(ctx)
	if err != nil {
		return err
	}

	return nil
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
	mockDB, mock := db.NewSQLMock(t.(*testing.T))

	// Create the test context with the mock DB
	ctx := NewTestContext(t, WithMockDB(mockDB))

	return ctx, mock
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

func WithInMemorySQLite() TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		provider := db.NewTestSQLiteProvider()
		dbInst, err := provider.Connect(ctx.Logger())
		if err != nil {
			return nil, err
		}

		ctx.RegisterCleanup(func() {
			_ = provider.Close()
		})

		return WithMockDB(dbInst)(ctx)
	}
}
