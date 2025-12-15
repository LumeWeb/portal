// Package testing provides utilities for testing core components
package testing

import (
	"fmt"
	"log"
	"reflect"
	"sync"
	"testing"

	"github.com/samber/lo"
	"go.lumeweb.com/portal/core"
)

func ensureValue(payload any) any {
	if payload == nil {
		return nil
	}
	val := reflect.ValueOf(payload)
	if val.Kind() == reflect.Ptr {
		// If it's a pointer, check if it's nil before dereferencing
		if val.IsNil() {
			return nil
		}
		// Return the pointed-to value
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
	setupCron         = false
	setupCronMu       sync.RWMutex
	setupHTTP         = false
	setupHTTPMu       sync.RWMutex
	testContexts      sync.Map   // map[*testing.T]TestContext
	testMutex         sync.Mutex // Protects test execution
)

// TestMainOpts configures test main behavior
type TestMainOpts struct {
	WithDB         bool
	DBMigrations   bool
	WithCron       bool
	WithHTTP       bool // Enable HTTP service
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

	if opts.WithCron {
		EnableCron()
	}

	if opts.WithHTTP {
		EnableHTTP()
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

	if opts.WithCron {
		DisableCron()
	}

	if opts.WithHTTP {
		DisableHTTP()
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



// getGlobalTestContextOptions returns the global options (from RunTests)
func getGlobalTestContextOptions() []TestContextBuilderOption {
	globalTestCtxOptsMu.RLock()
	defer globalTestCtxOptsMu.RUnlock()
	
	opts := make([]TestContextBuilderOption, len(globalTestCtxOpts))
	copy(opts, globalTestCtxOpts)
	return opts
}

// getTestCaseTestContextOptions returns the test case options (from RunTestCase)
func getTestCaseTestContextOptions() []TestContextBuilderOption {
	testCaseCtxOptsMu.RLock()
	defer testCaseCtxOptsMu.RUnlock()
	
	opts := make([]TestContextBuilderOption, len(testCaseCtxOpts))
	copy(opts, testCaseCtxOpts)
	return opts
}

// GetCombinedTestContextOptions returns all applicable options in order:
// 1. Default options
// 2. Global options (from RunTests)
// 3. Test case options (from RunTestCase)
func GetCombinedTestContextOptions() ([]TestContextBuilderOption, error) {
	// Get defaults first
	defaultOpts, err := DefaultTestContextOptions()
	if err != nil {
		return nil, err
	}

	// Get global and test case options
	globalOpts := getGlobalTestContextOptions()
	testCaseOpts := getTestCaseTestContextOptions()

	// Combine all options
	opts := make([]TestContextBuilderOption, 0, len(defaultOpts)+len(globalOpts)+len(testCaseOpts))
	opts = append(opts, defaultOpts...)
	opts = append(opts, globalOpts...)
	opts = append(opts, testCaseOpts...)

	return opts, nil
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

type (
	TB            = testing.TB
	TestComponent = string
)

const (
	ComponentDB   TestComponent = "db"
	ComponentHTTP TestComponent = "http"
	ComponentCron TestComponent = "cron"
)

// TestComponents creates a bag of test components for RunTestCaseWithComponents
func TestComponents(components ...TestComponent) []TestComponent {
	return components
}

// processTestComponents converts component strings to configuration flags
func processTestComponents(components []TestComponent) RunTestCaseOpts {
	opts := RunTestCaseOpts{
		AutoCleanup:   true,
		FireBootEvent: true,
	}

	for _, comp := range components {
		switch comp {
		case ComponentDB:
			opts.Components = append(opts.Components, ComponentDB)
			opts.RunMigrations = true // DB implies migrations by default
		case ComponentHTTP:
			opts.Components = append(opts.Components, ComponentHTTP)
		case ComponentCron:
			opts.Components = append(opts.Components, ComponentCron)
		}
	}
	return opts
}

// SetupTest creates and manages the test context for a specific test
// It does NOT boot the environment. BootEnvironment must be called separately.
func SetupTest(t TB) (TestContext, error) {
	t.Helper()

	// Check if we already have a context for this test
	if ctx, ok := testContexts.Load(t); ok {
		return ctx.(TestContext), nil
	}

	// Create new context with current options
	// Options are NOT processed here, they are added to the global list
	ctx, err := NewTestContext(t)
	if err != nil {
		return nil, err
	}

	// Store it
	testContexts.Store(t, ctx)

	// Automatically clean up when test finishes
	t.Cleanup(func() {
		ctx.Teardown()
		testContexts.Delete(t)
	})

	return ctx, nil
}

// SetupTestWithDB creates a test context with database support
// It does NOT boot the environment. BootEnvironment must be called separately.
func SetupTestWithDB(t TB) (TestContext, error) {
	t.Helper()

	// Enable DB migrations
	EnableDBMigrations()

	// Setup test context
	ctx, err := SetupTest(t)
	if err != nil {
		// Disable migrations on early return to avoid leaking state
		DisableDBMigrations()
		return nil, err
	}

	return ctx, nil
}

// GetTestContext retrieves the context for a test if it exists
func GetTestContext(t TB) (TestContext, error) {
	t.Helper()
	if ctx, ok := testContexts.Load(t); ok {
		return ctx.(TestContext), nil
	}
	return nil, fmt.Errorf("No test context found - did you call SetupTest()?")
}

// RunTestCaseOpts configures test case behavior
type RunTestCaseOpts struct {
	Components    []TestComponent // List of components to enable
	RunMigrations bool            // Only relevant if DB component is enabled
	CustomOptions []TestContextBuilderOption
	// Additional flags for test behavior
	SkipBoot      bool // Skip booting the environment
	AutoCleanup   bool // Automatically cleanup resources (default true)
	FireBootEvent bool // Fire boot complete event (default true)
}

// runTestCaseInternal is a helper function that contains the common logic for all RunTestCase variants
func runTestCaseInternal(t TB, testFunc func(tb TB, ctx TestContext), opts RunTestCaseOpts) {
	t.Helper()

	testMutex.Lock()
	defer testMutex.Unlock()

	// Reset test case state before test
	ResetAllState()
	if opts.AutoCleanup {
		defer ResetAllState()
	}

	// Enable requested components and get cleanup functions
	cleanups := enableTestComponents(opts)
	if opts.AutoCleanup {
		defer func() {
			for _, fn := range cleanups {
				fn()
			}
		}()
	}

	// Add any custom options to the test case collection
	if len(opts.CustomOptions) > 0 {
		AddTestCaseContextOptions(opts.CustomOptions...)
	}

	// Get or create the context
	var ctx TestContext
	var err error

	if hasComponent(opts.Components, ComponentDB) {
		ctx, err = SetupTestWithDB(t)
	} else {
		ctx, err = SetupTest(t)
	}

	if err != nil {
		t.Fatalf("Failed to setup test context: %v", err)
	}

	// Boot the environment unless skipped
	if !opts.SkipBoot {
		if err := BootEnvironment(t, ctx); err != nil {
			t.Fatalf("Failed to boot test environment: %v", err)
		}
	}

	// Run the actual test
	testFunc(t, ctx)
}

// enableTestComponents enables the requested test components
func enableTestComponents(opts RunTestCaseOpts) []func() {
	var cleanups []func()

	if hasComponent(opts.Components, ComponentDB) {
		if opts.RunMigrations {
			EnableDBMigrations()
			cleanups = append(cleanups, DisableDBMigrations)
		}
		EnableMockDB()
		cleanups = append(cleanups, DisableMockDB)
	}

	if hasComponent(opts.Components, ComponentHTTP) {
		EnableHTTP()
		cleanups = append(cleanups, DisableHTTP)
	}

	if hasComponent(opts.Components, ComponentCron) {
		EnableCron()
		cleanups = append(cleanups, DisableCron)
	}

	return cleanups
}

// hasComponent checks if a component is in the list using lo.Contains
func hasComponent(components []TestComponent, target TestComponent) bool {
	return lo.Contains(components, target)
}

// RunTestCase provides a cleaner way to run tests with automatic context setup
func RunTestCase(t TB, testFunc func(tb TB, ctx TestContext), opts ...TestContextBuilderOption) {
	t.Helper()
	RunTestCaseWithComponents(t, testFunc, nil, opts...)
}

// RunTestCaseWithDB provides a cleaner way to run tests with automatic context setup and database support
func RunTestCaseWithDB(t TB, testFunc func(tb TB, ctx TestContext), opts ...TestContextBuilderOption) {
	t.Helper()
	RunTestCaseWithComponents(t, testFunc, TestComponents(ComponentDB), opts...)
}

// RunTestCaseWithHTTP provides a cleaner way to run tests with automatic context setup
// and HTTP service enabled (no DB)
func RunTestCaseWithHTTP(t TB, testFunc func(tb TB, ctx TestContext), opts ...TestContextBuilderOption) {
	t.Helper()
	RunTestCaseWithComponents(t, testFunc, TestComponents(ComponentHTTP), opts...)
}

// RunTestCaseWithComponents runs a test case with specified components enabled
func RunTestCaseWithComponents(t TB, testFunc func(tb TB, ctx TestContext), components []TestComponent, opts ...TestContextBuilderOption) {
	t.Helper()
	runOpts := processTestComponents(components)
	runOpts.CustomOptions = opts
	runTestCaseInternal(t, testFunc, runOpts)
}

// ResetAllState resets all global state in the core package and testing package
// while preserving package-level configuration like DB settings and error namespaces
func ResetAllState() {
	// Save current error namespaces before reset
	savedNamespaces := core.ExportAllErrorNamespaces()
	
	// Reset core state (including error registry)
	core.ResetState()
	
	// Restore error namespaces to preserve them across test isolation
	if err := core.ReplaceAllErrorNamespaces(savedNamespaces); err != nil {
		log.Fatalf("failed to restore error namespaces: %v", err)
	}

	// Reset testing state (only clears test case specific options)
	ClearTestCaseContextOptions()

	// Note: We intentionally don't reset runDBMigrations/setupMockDB/setupCron/setupHTTP here
	// as these package-level settings should persist across tests within a suite.
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

// EnableCron enables cron service startup during test context initialization
func EnableCron() {
	setupCronMu.Lock()
	defer setupCronMu.Unlock()
	setupCron = true
}

// DisableCron disables cron service startup during test context initialization
func DisableCron() {
	setupCronMu.Lock()
	defer setupCronMu.Unlock()
	setupCron = false
}

// EnableHTTP enables HTTP service startup during test context initialization
func EnableHTTP() {
	setupHTTPMu.Lock()
	defer setupHTTPMu.Unlock()
	setupHTTP = true
}

// DisableHTTP disables HTTP service startup during test context initialization
func DisableHTTP() {
	setupHTTPMu.Lock()
	defer setupHTTPMu.Unlock()
	setupHTTP = false
}

// ShouldSetupHTTP returns whether HTTP service should be started
func ShouldSetupHTTP() bool {
	setupHTTPMu.RLock()
	defer setupHTTPMu.RUnlock()
	return setupHTTP
}

// ShouldSetupCron returns whether cron service should be started
func ShouldSetupCron() bool {
	setupCronMu.RLock()
	defer setupCronMu.RUnlock()
	return setupCron
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

// TestMainOption is a callback function that receives a TestContext and returns TestContextBuilderOptions.
// This allows creating options that need access to the test context during TestMain setup.
type TestMainOption func(ctx TestContext) ([]TestContextBuilderOption, error)

// WithTestMainContext creates a TestContextBuilderOption that executes a callback with access to the test context.
// This is useful in TestMain when you need to build options that depend on the test context.
//
// Example usage in TestMain:
//
//	func TestMain(m *testing.M) {
//		coreTesting.RunTests(m, coreTesting.TestMainOpts{
//			WithDB: true,
//			CustomSetup: func() {
//				coreTesting.AddGlobalTestContextOptions(
//					coreTesting.WithTestMainContext(func(ctx coreTesting.TestContext) ([]coreTesting.TestContextBuilderOption, error) {
//						// Now you have access to the test context
//						db := ctx.DB()
//						if db != nil {
//							// Create options based on the database connection
//							return []coreTesting.TestContextBuilderOption{
//								coreTesting.WithConfig("custom.db.setting", "value"),
//							}, nil
//						}
//						return nil, nil
//					}),
//				)
//			},
//		})
//	}
func WithTestMainContext(callback TestMainOption) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		// Execute the callback with the current test context
		options, err := callback(ctx)
		if err != nil {
			return ctx, fmt.Errorf("TestMain callback failed: %w", err)
		}

		// Apply all returned options using the existing ProcessCtxOptions function
		return ProcessCtxOptions(ctx, options...)
	}
}

// WithTestMainContextSimple is a convenience function that doesn't return an error.
// Use this when your callback logic cannot fail.
func WithTestMainContextSimple(callback func(ctx TestContext) []TestContextBuilderOption) TestContextBuilderOption {
	return WithTestMainContext(func(ctx TestContext) ([]TestContextBuilderOption, error) {
		return callback(ctx), nil
	})
}
