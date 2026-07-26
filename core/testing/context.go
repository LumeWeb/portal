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
	"go.opentelemetry.io/otel/trace"
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
	SetStartupFuncs(funcs []func(core.Context) error)             // Set the startup functions for the context
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
var _ core.Context = (*testContext)(nil)

// NewTestContext creates a new Context suitable for testing with either *testing.T or *testing.B
// It does NOT process the options immediately. Options are processed during BootEnvironment.
func NewTestContext(tb TB, opts ...TestContextBuilderOption) (TestContext, error) {
	return NewTestContextWithConfig(tb, ConfigModeReal, opts...)
}

// NewTestContextWithConfig creates a new Context with specified config mode
func NewTestContextWithConfig(tb TB, configMode ConfigMode, opts ...TestContextBuilderOption) (TestContext, error) {
	tb.Helper()

	// Create config manager based on mode
	cfg := NewTestConfigManager(tb, configMode)

	// Default to firing boot complete event
	fireBootComplete := false

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

	logger := core.NewLogger(cfg)
	// Replace the underlying zap logger
	logger.Logger = zapLogger

	// Set the logger on the config
	cfg.SetLogger(zapLogger)

	// Create context with cancel
	baseCtx, cancel := context.WithCancel(context.Background())

	// Create the test context
	testCtx := &testContext{
		defaultContext: &defaultContext{
			Context:      baseCtx,
			services:     make(map[string]any),
			cfg:          cfg,
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

func (ctx *testContext) ReplaceLogger(logger *core.Logger) {
	ctx.logger = logger
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

func (ctx *testContext) TraceMethod(name string, opts ...core.SpanOption) (context.Context, trace.Span) {
	return core.TraceMethod(ctx.Context, name, opts...)
}

func (ctx *testContext) NamedLogger(name string) *core.Logger {
	return core.NewLogger(ctx.Config(), ctx.logger.Logger.Named(name))
}

func (ctx *testContext) WithTracer(service, subsystem string) core.Context {
	return &testContext{
		defaultContext: &defaultContext{
			Context:      core.WithTracerInfo(ctx.Context, service, subsystem),
			services:     ctx.services,
			cfg:          ctx.cfg,
			db:           ctx.db,
			exitCode:     ctx.exitCode,
			exitFuncs:    ctx.exitFuncs,
			startupFuncs: ctx.startupFuncs,
			event:        ctx.event,
			logger:       ctx.logger,
			cancel:       ctx.cancel,
			router:       ctx.router,
		},
		tb:               ctx.tb,
		cleanupFuncs:     ctx.cleanupFuncs,
		apiID:            ctx.apiID,
		fireBootComplete: ctx.fireBootComplete,
	}
}

func (ctx *testContext) WithTracerService(service string) core.Context {
	return ctx.WithTracer(service, core.GetTracerSubsystem(ctx.Context))
}

func (ctx *testContext) WithTracerSubsystem(subsystem string) core.Context {
	return ctx.WithTracer(core.GetTracerService(ctx.Context), subsystem)
}

func (ctx *testContext) WithProtocolTracer(protocolName string) core.Context {
	return ctx.WithTracer(core.DefaultTracerService, "protocol-"+protocolName)
}

func (ctx *testContext) WithAPITracer(apiName string) core.Context {
	return ctx.WithTracer(core.DefaultTracerService, "api-"+apiName)
}

func (ctx *testContext) WithAPIExtensionTracer(extensionName string) core.Context {
	return ctx.WithTracer(core.DefaultTracerService, "apiext-"+extensionName)
}

func (ctx *testContext) WithServiceTracer(serviceName string) core.Context {
	return ctx.WithTracer(core.DefaultTracerService, "service-"+serviceName)
}

func (ctx *testContext) WithProtocolSubcomponent(protocolName, subcomponentName string) core.Context {
	return ctx.WithTracer(core.DefaultTracerService, "protocol-"+protocolName+"."+subcomponentName)
}

func (ctx *testContext) WithAPISubcomponent(apiName, subcomponentName string) core.Context {
	return ctx.WithTracer(core.DefaultTracerService, "api-"+apiName+"."+subcomponentName)
}

func (ctx *testContext) WithAPIExtensionSubcomponent(extensionName, subcomponentName string) core.Context {
	return ctx.WithTracer(core.DefaultTracerService, "apiext-"+extensionName+"."+subcomponentName)
}

func (ctx *testContext) WithServiceSubcomponent(serviceName, subcomponentName string) core.Context {
	return ctx.WithTracer(core.DefaultTracerService, "service-"+serviceName+"."+subcomponentName)
}

func (ctx *testContext) Config() config.Manager {
	return ctx.cfg
}

func (ctx *testContext) SetConfig(cfg config.Manager) {
	ctx.cfg = cfg
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

			case func(TestContext) (TestContext, error):
				// Handle functions that match the TestContextBuilderOption signature
				// This includes functions returned by CombineOptions itself
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

// InitContext initializes a TestContext by:
// 1. Processing default test context options
// 2. Executing startup functions registered by those defaults
// Returns the first error encountered at any step.
// NOTE: This function is now primarily used internally by BootEnvironment.
// For most test cases, use SetupTest or RunTestCase helpers.
// Global and test-case specific options are processed separately by
// ProcessGlobalOptions and ProcessTestCaseOptions in BootEnvironment.
func InitContext(ctx TestContext) error {
	var err error

	// Process only default options
	defaultOpts, err := DefaultTestContextOptions()
	if err != nil {
		return err
	}

	ctx, err = ProcessCtxOptions(ctx, defaultOpts...)
	if err != nil {
		return err
	}

	// Process core startup functions
	err = ProcessStartupFuncs(ctx)
	if err != nil {
		return err
	}

	return nil
}

// ProcessGlobalOptions processes only global options (from RunTests)
// This should be called after InitContext to apply global configurations
//
// Note: All TestContextBuilderOption implementations mutate the context in place.
// The returned TestContext is the same instance and is returned only for API symmetry
// with ProcessCtxOptions and future-proofing. The function could return just error,
// but keeping the (TestContext, error) signature maintains consistency.
func ProcessGlobalOptions(ctx TestContext) (TestContext, error) {
	globalOpts := getGlobalTestContextOptions()

	if len(globalOpts) > 0 {
		var err error
		ctx, err = ProcessCtxOptions(ctx, globalOpts...)
		if err != nil {
			return ctx, fmt.Errorf("failed to process global options: %w", err)
		}
	}

	return ctx, nil
}

// ProcessTestCaseOptions processes only test case specific options
// This should be called after InitContext to apply test case specific configurations
//
// Note: All TestContextBuilderOption implementations mutate the context in place.
// The returned TestContext is the same instance and is returned only for API symmetry
// with ProcessCtxOptions and future-proofing. The function could return just error,
// but keeping the (TestContext, error) signature maintains consistency.
func ProcessTestCaseOptions(ctx TestContext) (TestContext, error) {
	testCaseOpts := getTestCaseTestContextOptions()

	if len(testCaseOpts) > 0 {
		var err error
		ctx, err = ProcessCtxOptions(ctx, testCaseOpts...)
		if err != nil {
			return ctx, fmt.Errorf("failed to process test case options: %w", err)
		}
	}

	return ctx, nil
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

func ContextOptions(options ...TestContextBuilderOption) []TestContextBuilderOption {
	return options
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

// SetupDatabaseOptions creates and returns database setup test context options
// (without migrations). This helper centralizes the database setup logic.
func SetupDatabaseOptions() []TestContextBuilderOption {
	var opts []TestContextBuilderOption

	// Add DB setup if enabled
	if ShouldSetupMockDB() {
		opts = append(opts, WithSQLite())
	}

	return opts
}

// SetupMigrationOptions creates and returns migration test context options
// based on the current migration settings. This helper centralizes
// the migration logic separately from database setup.
func SetupMigrationOptions() []TestContextBuilderOption {
	var opts []TestContextBuilderOption

	// Add migrations if enabled
	if ShouldSetupMockDB() && ShouldRunDBMigrations() {
		opts = append(opts, WithDBMigrations())
	}

	return opts
}

// DefaultTestContextOptions returns the default options used for new test contexts.
// These include mock implementations of common core services.
func DefaultTestContextOptions() ([]TestContextBuilderOption, error) {
	_router, err := router.NewRouter(router.APIInfo().Title("Test API").Version("1.0.0"))
	if err != nil {
		return nil, fmt.Errorf("failed to create test router: %w", err)
	}

	opts := []TestContextBuilderOption{
		WithEnvConfigOrDefault("core.domain", "", "test.local"),
		WithRandomSeedPhrase(),
		WithCoreEvents(),
		WithErrorNamespaces(core.ExportAllErrorNamespaces()),
	}

	// Add remaining mock services
	opts = append(opts,
		WithMockAccessService(),
		WithRouter(_router),
		WithMockAuthService(),
		WithMockContentScannerService(),
	)

	// Use real cron service if enabled, otherwise mock
	if ShouldSetupCron() {
		opts = append(opts, WithCronService())
	} else {
		opts = append(opts, WithMockCronService())
	}

	// Use real HTTP service if enabled, otherwise mock
	if ShouldSetupHTTP() {
		opts = append(opts,
			WithEnvConfigOrDefault("core.port", "", 0),
			WithHTTPService(),
		)
	} else {
		opts = append(opts,
			WithMockHTTPService(),
			WithConfig("core.port", 80),
		)
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

// BootEnvironment initializes the test environment by executing a series of carefully ordered phases
// that mirror the portal's production boot sequence. Each phase builds upon the previous one to ensure
// proper initialization order and dependency resolution.
//
// The phases are:
//  1. Context Initialization - Processes default context options only
//     Establishes the base context with core services and configuration
//     1.5. Global Options Processing - Processes global options from RunTests
//     1.6. Test Case Options Processing - Processes test case specific options
//  2. Component Registration - Registers all components (services, APIs, protocols and extensions)
//     Note: Service options are collected but not processed yet to allow proper ordering
//  3. Plugin Service Registration - Registers and configures services from plugins
//     Collects any context options they return
//  4. Component Configuration - Configures services and protocols with their respective configs
//     Note: APIs are configured separately in Phase 5 to ensure services are ready first
//  5. API Configuration - Configures APIs and processes their initialization options
//     This must complete before service options are processed (Phase 6)
//  6. Service Option Processing - Combines all service options (main + plugin services)
//     Processes them now that APIs are fully initialized
//  7. Startup Functions - Runs any startup functions added during initialization
//  8. Service Initialization - Initializes services after configuration but before API route setup
//  9. API Route Configuration - Configures API routes after all components are initialized
//     Applies any registered extensions using the same router
//  10. Protocol Initialization - Initializes protocols and registers their workflows
//  11. Protocol Workflow Configuration - Registers workflows from all protocols
//  12. Runtime Setup - Starts cron/HTTP services if enabled and fires boot complete event
//     Note: HTTP service is started in a goroutine and registered for cleanup
//
// The function returns nil on success, or an error if any phase fails. Errors include detailed
// information about which phase failed.
func BootEnvironment(tb TB, ctx TestContext) error {
	var err error

	// Phase 1: Context Initialization
	if err = InitContext(ctx); err != nil {
		return fmt.Errorf("context initialization failed: %w", err)
	}

	// Phase 1.5: Global Options Processing
	ctx, err = ProcessGlobalOptions(ctx)
	if err != nil {
		return fmt.Errorf("global options processing failed: %w", err)
	}

	// Phase 1.6: Database Setup (before test options)
	// Backup existing startup functions before DB setup
	originalStartupFuncs := ctx.StartupFuncs()
	// Clear startup functions temporarily during DB setup
	ctx.SetStartupFuncs(nil)

	dbOpts := SetupDatabaseOptions()
	if len(dbOpts) > 0 {
		ctx, err = ProcessCtxOptions(ctx, dbOpts...)
		if err != nil {
			return fmt.Errorf("database setup failed: %w", err)
		}
		if err = ProcessStartupFuncs(ctx); err != nil {
			return fmt.Errorf("startup functions failed: %w", err)
		}
	}

	// Restore original startup functions after DB setup
	ctx.SetStartupFuncs(originalStartupFuncs)

	// Phase 1.7: Test Case Options Processing
	ctx, err = ProcessTestCaseOptions(ctx)
	if err != nil {
		return fmt.Errorf("test case options processing failed: %w", err)
	}

	// Phase 1.8: Migration Setup (after test options, before component registration)
	// Backup existing startup functions before migration setup
	originalStartupFuncs = ctx.StartupFuncs()
	// Clear startup functions temporarily during migration setup
	ctx.SetStartupFuncs(nil)

	migrationOpts := SetupMigrationOptions()
	if len(migrationOpts) > 0 {
		ctx, err = ProcessCtxOptions(ctx, migrationOpts...)
		if err != nil {
			return fmt.Errorf("migration setup failed: %w", err)
		}
		if err = ProcessStartupFuncs(ctx); err != nil {
			return fmt.Errorf("startup functions failed: %w", err)
		}
	}

	// Restore original startup functions after migration setup
	ctx.SetStartupFuncs(originalStartupFuncs)

	// Phase 2: Component Registration
	componentOpts, svcOpts, err := RegisterComponents(ctx)
	if err != nil {
		return fmt.Errorf("component registration failed: %w", err)
	}

	ctx, err = ProcessCtxOptions(ctx, componentOpts...)
	if err != nil {
		return fmt.Errorf("processing component options failed: %w", err)
	}

	// Phase 3: Plugin Service Registration
	core.RegisterServicesFromPlugins()
	core.RegisterKeyIdentityHandlersFromPlugins()
	newCtx, pluginSvcOpts, err := ConfigurePluginServices(ctx)
	if err != nil {
		return fmt.Errorf("plugin service registration failed: %w", err)
	}
	ctx = newCtx

	// Phase 4: Component Configuration
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

	// Phase 5: API Configuration
	if ctx, err = ProcessCtxOptions(ctx, apiOpts...); err != nil {
		return fmt.Errorf("API option processing failed: %w", err)
	}

	// Phase 6: Service Option Processing
	allSvcOpts := append(svcOpts, pluginSvcOpts...)
	ctx, err = ProcessCtxOptions(ctx, allSvcOpts...)
	if err != nil {
		return fmt.Errorf("processing service options failed: %w", err)
	}

	// Phase 7: Startup Functions
	// Note: Core/default startup functions are executed during InitContext (Phase 1).
	// This phase runs startup functions registered during later phases (components, plugins, options, etc.).
	// Fire EVENT_BOOT_STARTUP_FUNCS before running startup functions
	if tctx, ok := ctx.(*testContext); ok && tctx.FireBootComplete() {
		if err = core.Fire(ctx, pevent.EVENT_BOOT_STARTUP_FUNCS, pevent.NewBootStartupFuncsEvent(ctx, ctx.GetContext())); err != nil {
			return fmt.Errorf("failed to fire boot startup funcs event: %w", err)
		}
	}

	// Load configuration before running startup functions
	if err = ctx.Config().Load(); err != nil {
		return fmt.Errorf("configuration load failed: %w", err)
	}

	if err = ProcessStartupFuncs(ctx); err != nil {
		return fmt.Errorf("startup functions failed: %w", err)
	}

	// Fire EVENT_BOOT_STARTUP_FUNCS_COMPLETED after running startup functions
	if tctx, ok := ctx.(*testContext); ok && tctx.FireBootComplete() {
		if err = core.Fire(ctx, pevent.EVENT_BOOT_STARTUP_FUNCS_COMPLETED, pevent.NewBootStartupFuncsCompletedEvent(ctx, ctx.GetContext())); err != nil {
			return fmt.Errorf("failed to fire boot startup funcs completed event: %w", err)
		}
	}

	// Phase 8: Service Initialization
	if err := InitializeServices(ctx); err != nil {
		return fmt.Errorf("service initialization failed: %w", err)
	}

	// Phase 9: API Route Configuration
	if err = ConfigureAPIRoutes(ctx); err != nil {
		return fmt.Errorf("API route configuration failed: %w", err)
	}

	// Phase 10: Protocol Initialization
	// Fire EVENT_BOOT_PROTOCOL_WORKFLOWS and EVENT_BOOT_PROTOCOLS before initializing protocols
	if tctx, ok := ctx.(*testContext); ok && tctx.FireBootComplete() {
		if err = core.Fire(ctx, pevent.EVENT_BOOT_PROTOCOL_WORKFLOWS, pevent.NewBootProtocolWorkflowsEvent(ctx, ctx.GetContext())); err != nil {
			return fmt.Errorf("failed to fire boot protocol workflows event: %w", err)
		}

		if err = core.Fire(ctx, pevent.EVENT_BOOT_PROTOCOLS, pevent.NewBootProtocolsEvent(ctx, ctx.GetContext())); err != nil {
			return fmt.Errorf("failed to fire boot protocols event: %w", err)
		}
	}

	if err := InitializeProtocols(ctx); err != nil {
		return fmt.Errorf("protocol initialization failed: %w", err)
	}

	// Fire EVENT_BOOT_PROTOCOL_WORKFLOWS_COMPLETED and EVENT_BOOT_PROTOCOLS_COMPLETED after initializing protocols
	if tctx, ok := ctx.(*testContext); ok && tctx.FireBootComplete() {
		if err = core.Fire(ctx, pevent.EVENT_BOOT_PROTOCOL_WORKFLOWS_COMPLETED, pevent.NewBootProtocolWorkflowsCompletedEvent(ctx, ctx.GetContext())); err != nil {
			return fmt.Errorf("failed to fire boot protocol workflows completed event: %w", err)
		}

		if err = core.Fire(ctx, pevent.EVENT_BOOT_PROTOCOLS_COMPLETED, pevent.NewBootProtocolsCompletedEvent(ctx, ctx.GetContext())); err != nil {
			return fmt.Errorf("failed to fire boot protocols completed event: %w", err)
		}
	}

	// Phase 11: Protocol Workflow Configuration
	// Fire EVENT_BOOT_PLUGIN_WORKFLOWS before configuring protocol workflows
	if tctx, ok := ctx.(*testContext); ok && tctx.FireBootComplete() {
		if err = core.Fire(ctx, pevent.EVENT_BOOT_PLUGIN_WORKFLOWS, pevent.NewBootPluginWorkflowsEvent(ctx, ctx.GetContext())); err != nil {
			return fmt.Errorf("failed to fire boot plugin workflows event: %w", err)
		}
	}

	if err = ConfigureProtocolWorkflows(ctx); err != nil {
		return fmt.Errorf("failed to configure protocol workflows: %w", err)
	}

	// Fire EVENT_BOOT_PLUGIN_WORKFLOWS_COMPLETED after configuring protocol workflows
	if tctx, ok := ctx.(*testContext); ok && tctx.FireBootComplete() {
		if err = core.Fire(ctx, pevent.EVENT_BOOT_PLUGIN_WORKFLOWS_COMPLETED, pevent.NewBootPluginWorkflowsCompletedEvent(ctx, ctx.GetContext())); err != nil {
			return fmt.Errorf("failed to fire boot plugin workflows completed event: %w", err)
		}
	}

	// Phase 12: Runtime Setup
	if tctx, ok := ctx.(*testContext); ok && tctx.FireBootComplete() {
		// Fire EVENT_BOOT_START at the very beginning of runtime setup
		if err = core.Fire(ctx, pevent.EVENT_BOOT_START, pevent.NewBootStartEvent(ctx, ctx.GetContext())); err != nil {
			return fmt.Errorf("failed to fire boot start event: %w", err)
		}

		// Fire EVENT_BOOT_CRON before starting cron
		if err = core.Fire(ctx, pevent.EVENT_BOOT_CRON, pevent.NewBootCronEvent(ctx, ctx.GetContext())); err != nil {
			return fmt.Errorf("failed to fire boot cron event: %w", err)
		}

		if ShouldSetupCron() {
			if err = StartCron(ctx); err != nil {
				return fmt.Errorf("failed to start cron service: %w", err)
			}
			tctx.tb.Cleanup(func() {
				if cronSvc := tctx.Service(core.CRON_SERVICE); cronSvc != nil {
					if stopper, ok2 := cronSvc.(interface{ Stop() error }); ok2 {
						if err := stopper.Stop(); err != nil {
							tctx.Logger().Debug("Cron service stop error", zap.Error(err))
						}
					}
				}
			})
		}

		// Fire EVENT_BOOT_CRON_COMPLETED after starting cron
		if err = core.Fire(ctx, pevent.EVENT_BOOT_CRON_COMPLETED, pevent.NewBootCronCompletedEvent(ctx, ctx.GetContext())); err != nil {
			return fmt.Errorf("failed to fire boot cron completed event: %w", err)
		}

		// Fire EVENT_BOOT_HTTP before starting HTTP
		if err = core.Fire(ctx, pevent.EVENT_BOOT_HTTP, pevent.NewBootHTTPEvent(ctx, ctx.GetContext())); err != nil {
			return fmt.Errorf("failed to fire boot http event: %w", err)
		}

		if ShouldSetupHTTP() {
			tctx, ok := ctx.(*testContext)
			if !ok {
				return fmt.Errorf("HTTP service setup requires *testContext but got %T", ctx)
			}
			if err = tctx.ServeHTTP(); err != nil {
				return fmt.Errorf("failed to start HTTP service: %w", err)
			}
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

		// Fire EVENT_BOOT_HTTP_COMPLETED after starting HTTP
		if err = core.Fire(ctx, pevent.EVENT_BOOT_HTTP_COMPLETED, pevent.NewBootHTTPCompletedEvent(ctx, ctx.GetContext())); err != nil {
			return fmt.Errorf("failed to fire boot http completed event: %w", err)
		}

		// Fire EVENT_BOOT_MAILER (even though we don't start mailer in tests)
		if err = core.Fire(ctx, pevent.EVENT_BOOT_MAILER, pevent.NewBootMailerEvent(ctx, ctx.GetContext())); err != nil {
			return fmt.Errorf("failed to fire boot mailer event: %w", err)
		}

		// Fire EVENT_BOOT_MAILER_COMPLETED
		if err = core.Fire(ctx, pevent.EVENT_BOOT_MAILER_COMPLETED, pevent.NewBootMailerCompletedEvent(ctx, ctx.GetContext())); err != nil {
			return fmt.Errorf("failed to fire boot mailer completed event: %w", err)
		}

		// Fire EVENT_BOOT_COMPLETED at the very end
		if err = core.Fire(ctx, pevent.EVENT_BOOT_COMPLETED, pevent.NewBootCompletedEvent(ctx, ctx.GetContext())); err != nil {
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

		// Use ContextWithStartupComponent to properly wire up the service with BaseComponent, DB, GetConfig, Logger, and Context
		startupOpt := core.ContextOptions(core.ContextWithStartupComponent(svc))
		opts = append(opts, WrapCoreOptions(startupOpt)...)

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

	return svc.Start(ctx.GetContext())
}
