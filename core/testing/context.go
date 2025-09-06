// Package testing provides utilities for testing core components
package testing

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.lumeweb.com/event/v2"
	router "go.lumeweb.com/portal-router"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	pevent "go.lumeweb.com/portal/event"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
	"gorm.io/gorm"
)

// TestContextBuilderOption configures a test context
type TestContextBuilderOption func(context TestContext) (TestContext, error)

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
	SetFireBootComplete(fire bool)                                // Set whether boot complete event should fire
	FireBootComplete() bool                                       // Get whether boot complete event should fire
}

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
	apiID            string        // Stores the API ID for this context
	fireBootComplete bool          // Controls whether to fire boot complete event
	httpDone         chan struct{} // Signals when HTTP server is done
}

// Ensure testContext implements TestContext
var _ TestContext = (*testContext)(nil)

// NewTestContext creates a new Context suitable for testing with either *testing.T or *testing.B
// It does NOT process the options immediately. Options are processed during BootEnvironment.
func NewTestContext(tb TB, opts ...TestContextBuilderOption) (TestContext, error) {
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

	return testCtx, nil // Return the context without processing options yet
}

// RegisterService adds a service to the context after creation
func (c *testContext) RegisterService(id string, service interface{}) {
	c.services[id] = service
}

// RegisterCleanup adds a function to be called during Teardown
func (c *testContext) RegisterCleanup(fn func()) {
	c.cleanupFuncs = append(c.cleanupFuncs, fn)
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

	// Wait for HTTP server to stop if it was started, with timeout
	if c.httpDone != nil {
		select {
		case <-c.httpDone:
			// Normal shutdown
		case <-time.After(5 * time.Second):
			c.Logger().Warn("Timeout waiting for HTTP service to stop")
		}
	}

	// Process exit functions first (e.g., provider.Close) before cleanup functions
	// This prevents file locking issues on Windows where exit handlers might
	// still have files open when cleanup functions try to delete them
	if err := ProcessExitFuncs(c); err != nil {
		c.Logger().Error("Error during exit function processing", zap.Error(err))
	}

	// Run any registered cleanup functions (e.g., file removals)
	// Note: testing.TB.Cleanup also handles this, but keeping this for clarity
	// and potential future custom cleanup logic not tied to TB.Cleanup.
	for _, fn := range c.cleanupFuncs {
		fn()
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

	svc := c.Service(core.HTTP_SERVICE)
	httpSvc, ok := svc.(core.HTTPService)

	var apiSubdomain string
	if ok && httpSvc != nil {
		apiSubdomain = httpSvc.APISubdomain(c.apiID, false)
	}

	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	if apiSubdomain != "" {
		req.Host = apiSubdomain
	}

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

// ServeHTTP starts the HTTP service in a goroutine and returns immediately.
// Returns nil if service starts successfully, or an error if the service
// cannot be found or started. Prevents multiple starts by returning nil
// if already running.
func (c *testContext) ServeHTTP() error {
	httpSvc := c.Service(core.HTTP_SERVICE)
	if httpSvc == nil {
		return fmt.Errorf("HTTP service not found in context")
	}

	httpService, ok := httpSvc.(core.HTTPService)
	if !ok {
		return fmt.Errorf("service found but is not core.HTTPService")
	}

	// Prevent multiple starts
	if c.httpDone != nil {
		return fmt.Errorf("HTTP service already running")
	}

	c.httpDone = make(chan struct{})
	go func() {
		defer close(c.httpDone)
		err := httpService.Serve()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			c.Logger().Error("HTTP service failed", zap.Error(err))
		} else if err != nil {
			c.Logger().Debug("HTTP service stopped", zap.Error(err))
		}
	}()

	time.Sleep(time.Second)

	// Update config with actual port after server starts
	port := httpService.Port()
	if port > 0 {
		if err := c.Config().Set(c, "core.port", port); err != nil {
			c.Logger().Error("Failed to update config with live port", zap.Error(err))
		}
	}

	return nil
}

// SetStartupFuncs sets the startup functions for the context
func (c *testContext) SetStartupFuncs(funcs []func(core.Context) error) {
	c.startupFuncs = funcs
}

// SetFireBootComplete sets whether boot complete event should fire
func (c *testContext) SetFireBootComplete(fire bool) {
	c.fireBootComplete = fire
}

// APISubdomain returns the formatted API subdomain URL
// id: The API identifier (e.g. "admin")
// proto: Whether to include https:// prefix
func (c *testContext) APISubdomain(id string, proto bool) string {
	api := core.GetAPI(id)
	if api == nil {
		c.tb.Fatalf("API with id '%s' not found - did you register it with WithAPI()?", id)
		return "" // unreachable but makes compiler happy
	}

	subdomain := api.Subdomain()
	domain := c.Config().Config().Core.Domain

	if subdomain == "" {
		if proto {
			return fmt.Sprintf("https://%s", domain)
		}
		return domain
	}

	formatter := ""
	if proto {
		formatter += "https://"
	}
	formatter += "%s.%s"

	return fmt.Sprintf(formatter, subdomain, domain)
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

// CombineOptions combines multiple TestContextBuilderOptions into a single option.
// It accepts:
// - Individual options (CombineOptions(opt1, opt2))
// - A slice of options (CombineOptions([]TestContextBuilderOption{opt1, opt2}))
// - Mixed arguments (CombineOptions(opt1, []TestContextBuilderOption{opt2, opt3}))
// - Nested slices (CombineOptions([]any{opt1, []TestContextBuilderOption{opt2, opt3}}))
func CombineOptions(opts ...any) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		var err error
		for i, opt := range opts {
			if opt == nil {
				return ctx, fmt.Errorf("option at index %d is nil", i)
			}

			switch v := opt.(type) {
			case TestContextBuilderOption:
				ctx, err = v(ctx)
				if err != nil {
					return ctx, fmt.Errorf("option at index %d failed: %w", i, err)
				}

			case []TestContextBuilderOption:
				// Flatten slice of TestContextBuilderOption
				flattened := make([]any, len(v))
				for j, o := range v {
					if o == nil {
						return ctx, fmt.Errorf("option at index %d contains nil element at sub-index %d", i, j)
					}
					flattened[j] = o
				}
				combined := CombineOptions(flattened...)
				ctx, err = combined(ctx)
				if err != nil {
					return ctx, fmt.Errorf("processing slice at index %d: %w", i, err)
				}

			case []any:
				// Recursively flatten nested slices
				combined := CombineOptions(v...)
				ctx, err = combined(ctx)
				if err != nil {
					return ctx, fmt.Errorf("processing slice at index %d: %w", i, err)
				}

			default:
				return ctx, fmt.Errorf("invalid option type %T at index %d, expected TestContextBuilderOption, []TestContextBuilderOption, or []any", opt, i)
			}
		}
		return ctx, nil
	}
}

// ProcessStartupFuncs executes all registered startup functions in the TestContext.
// Returns the first error encountered, if any. Functions are executed in the order they were registered.
// This is typically called during test initialization to simulate the portal's startup sequence.
func ProcessStartupFuncs(ctx TestContext) error {
	// Get the current slice of startup functions
	startupFuncs := ctx.StartupFuncs()

	// Clear the startup functions slice to prevent re-execution
	if tc, ok := ctx.(*testContext); ok {
		tc.SetStartupFuncs(nil)
	}

	// Execute each startup function exactly once
	var err error
	for _, fn := range startupFuncs {
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
func InitContext(tb TB, ctx TestContext) error {
	// Get all globally registered test options
	allOpts, err := GetCombinedTestContextOptions(tb)
	if err != nil {
		return err
	}

	// Process all context options
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

// WithAPIID sets the API ID for the test context, overriding any default value.
func WithAPIID(apiID string) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		ctx.SetAPIID(apiID)
		return ctx, nil
	}
}

// WithFireBootComplete sets whether the boot complete event should fire
func WithFireBootComplete(fire bool) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		ctx.SetFireBootComplete(fire)
		return ctx, nil
	}
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

// WithRouter adds a router to the test context
func WithRouter(r router.Router) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		if tc, ok := ctx.(*testContext); ok {
			tc.router = r
			return ctx, nil
		}
		return nil, fmt.Errorf("WithRouter requires *testContext; got %T", ctx)
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

// DefaultTestContextOptions returns the default options used for new test contexts.
// These include mock implementations of common core services.
func DefaultTestContextOptions(tb TB) ([]TestContextBuilderOption, error) {
	_router, err := router.NewRouter(router.APIInfo().Title("Test API").Version("1.0.0"))
	if err != nil {
		return nil, fmt.Errorf("failed to create test router: %w", err)
	}

	opts := []TestContextBuilderOption{
		WithEnvConfigOrDefault("core.domain", "", "test.local"),
		WithRandomSeedPhrase(),
		WithCoreEvents(),
	}

	// Add DB setup first if enabled
	if ShouldSetupMockDB() {
		opts = append(opts, WithSQLite())
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
	)

	// Use real HTTP service if enabled, otherwise mock
	if ShouldSetupHTTP() {
		opts = append(opts,
			WithEnvConfigOrDefault("core.port", "", 0),
			WithHTTPService(),
		)
	} else {
		opts = append(opts, WithMockHTTPService())
	}

	opts = append(opts,
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
		WithFireBootComplete(true),
	)

	return opts, nil
}

// BootEnvironment initializes the test environment with the following phases:
// 1. Context Initialization - Processes all context builder options
// 2. Component Registration - Registers services, APIs, protocols and extensions
// 3. Component Configuration - Configures services and protocols
// 4. API Initialization - Configures and initializes APIs
// 5. Service Option Processing - Processes all service options after APIs are ready
// 6. Runtime Setup - Starts cron/HTTP services and fires boot complete event
func BootEnvironment(tb TB, ctx TestContext) error {
	// Phase 1: Context Initialization
	// Processes all context builder options (default, global and test case specific)
	// Establishes the base context with core services and configuration
	if err := InitContext(tb, ctx); err != nil {
		return fmt.Errorf("context initialization failed: %w", err)
	}

	// Phase 2: Component Registration  
	// Registers all components (services, APIs, protocols and extensions)
	// Note: Service options are collected but not processed yet
	componentOpts, svcOpts, err := RegisterComponents(ctx)
	if err != nil {
		return fmt.Errorf("component registration failed: %w", err)
	}

	// Process non-service component options first
	ctx, err = ProcessCtxOptions(ctx, componentOpts...)
	if err != nil {
		return fmt.Errorf("processing component options failed: %w", err)
	}

	// Register any plugin services and process their options
	core.RegisterServicesFromPlugins()
	newCtx, pluginSvcOpts, err := ConfigurePluginServices(ctx)
	if err != nil {
		return fmt.Errorf("plugin service registration failed: %w", err)
	}
	ctx = newCtx

	// Phase 3: Component Configuration
	// Configures services and protocols with their respective configs
	// Note: APIs are configured separately in Phase 4
	if err = ConfigureServices(ctx); err != nil {
		return fmt.Errorf("service configuration failed: %w", err)
	}

	if err := ConfigureProtocols(ctx); err != nil {
		return fmt.Errorf("protocol configuration failed: %w", err)
	}

	apiOpts, err := ConfigureAPIs(ctx)
	if err != nil {
		return fmt.Errorf("API configuration failed: %w", err)
	}

	// Phase 4: API Initialization
	// Configures APIs and processes their initialization options
	// This must complete before service options are processed
	if ctx, err = ProcessCtxOptions(ctx, apiOpts...); err != nil {
		return fmt.Errorf("API initialization failed: %w", err)
	}

	// Phase 5: Service Option Processing
	// Combines all service options (main + plugin services)
	allSvcOpts := append(svcOpts, pluginSvcOpts...)

	// Processes them now that APIs are fully initialized
	ctx, err = ProcessCtxOptions(ctx, allSvcOpts...)
	if err != nil {
		return fmt.Errorf("processing service options failed: %w", err)
	}

	// Run any startup functions added during API initialization
	if err = ProcessStartupFuncs(ctx); err != nil {
		return fmt.Errorf("startup functions failed: %w", err)
	}

	// Configure API routes after all components are initialized
	if err = ConfigureAPIRoutes(ctx); err != nil {
		return fmt.Errorf("API route configuration failed: %w", err)
	}

	// Initialize protocols and register their workflows
	if err := InitializeProtocols(ctx); err != nil {
		return fmt.Errorf("protocol initialization failed: %w", err)
	}

	if err = ConfigureProtocolWorkflows(ctx); err != nil {
		return fmt.Errorf("failed to configure protocol workflows: %w", err)
	}

	// Phase 6: Runtime Setup
	// Starts cron/HTTP services if enabled
	// Fires boot complete event if configured
	if ShouldSetupCron() {
		if err = StartCron(ctx); err != nil {
			return fmt.Errorf("failed to start cron service: %w", err)
		}
	}

	// Start HTTP service if enabled
	if ShouldSetupHTTP() {
		tctx, ok := ctx.(*testContext)
		if !ok {
			return fmt.Errorf("HTTP service setup requires *testContext but got %T", ctx)
		}
		if err = tctx.ServeHTTP(); err != nil {
			return fmt.Errorf("failed to start HTTP service: %w", err)
		}
		// Register HTTP service stop with tb.Cleanup to ensure proper shutdown
		tctx.tb.Cleanup(func() {
			if httpSvc := tctx.Service(core.HTTP_SERVICE); httpSvc != nil {
				if httpService, ok := httpSvc.(core.HTTPService); ok {
					if err := httpService.Stop(); err != nil {
						tctx.Logger().Debug("HTTP service stop error", zap.Error(err))
					}
				}
			}
		})
	}

	// Fire boot complete event if enabled
	if tctx, ok := ctx.(*testContext); ok && tctx.FireBootComplete() {
		if err = core.Fire(ctx, pevent.EVENT_BOOT_COMPLETE, pevent.NewBootCompleteEvent(ctx)); err != nil {
			return fmt.Errorf("failed to fire boot complete event: %w", err)
		}
	}

	return nil
}

// RegisterServices initializes all services and collects their context options.
// Returns both direct context options and service-specific options separately
// to allow staged initialization.
func RegisterServices(ctx TestContext) ([]TestContextBuilderOption, error) {
	var opts []TestContextBuilderOption
	svcs := core.GetServices()

	for _, svcInfo := range svcs {
		svc, svcOpts, err := svcInfo.Factory()
		if err != nil {
			return nil, fmt.Errorf("failed to initialize service %s: %w", svcInfo.ID, err)
		}
		if svcOpts != nil {
			opts = append(opts, WrapCoreOptions(svcOpts)...)
		}
		tctx, ok := ctx.(*testContext)
		if !ok {
			return nil, fmt.Errorf("RegisterServices requires *testContext but got %T", ctx)
		}
		tctx.RegisterService(svcInfo.ID, svc)
	}

	return opts, nil
}

// ConfigurePluginServices initializes and registers all services that belong to plugins.
// It iterates through all registered services, filters for those associated with plugins,
// creates service instances using their factories, collects any context options they return,
// and registers them in the test context.
// Returns:
// - The updated context
// - All collected service options (to be processed later)
// - Any error encountered
func ConfigurePluginServices(ctx TestContext) (TestContext, []TestContextBuilderOption, error) {
	var allOpts []TestContextBuilderOption

	for _, svcInfo := range core.Unsafe_GetServiceMap() {
		plugin := core.GetPluginForService(svcInfo.ID)
		if plugin != "" {
			ctxOpts, err := RegisterService(ctx, svcInfo.ID, svcInfo.Factory, plugin)
			if err != nil {
				return ctx, nil, err
			}
			allOpts = append(allOpts, ctxOpts...)
		}
	}

	return ctx, allOpts, nil
}

// StartCron starts the cron service if enabled
func StartCron(ctx TestContext) error {
	cronSvc := ctx.Service(core.CRON_SERVICE)
	if cronSvc == nil {
		return fmt.Errorf("cron service not found in context")
	}

	svc, ok := cronSvc.(core.CronService)
	if !ok {
		return fmt.Errorf("service found but is not core.CronService")
	}

	return svc.Start()
}
