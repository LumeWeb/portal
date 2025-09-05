// Package testing provides utilities for testing core components
package testing

import (
	"fmt"
	"reflect"
	"sync"
	"testing"

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
	testContexts      sync.Map   // map[*testing.T]TestContext
	testMutex         sync.Mutex // Protects test execution
)

// TestMainOpts configures test main behavior
type TestMainOpts struct {
	WithDB         bool
	DBMigrations   bool
	WithCron       bool
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
func GetCombinedTestContextOptions(tb TB) ([]TestContextBuilderOption, error) {
	globalTestCtxOptsMu.RLock()
	defer globalTestCtxOptsMu.RUnlock()
	testCaseCtxOptsMu.RLock()
	defer testCaseCtxOptsMu.RUnlock()

	// Get defaults first
	defaultOpts, err := DefaultTestContextOptions(tb)
	if err != nil {
		return nil, err
	}

	// Combine with global and test case options
	opts := make([]TestContextBuilderOption, 0, len(defaultOpts)+len(globalTestCtxOpts)+len(testCaseCtxOpts))
	opts = append(opts, defaultOpts...)
	opts = append(opts, globalTestCtxOpts...)
	opts = append(opts, testCaseCtxOpts...)

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

type TB = testing.TB

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
	
	// Ensure migrations are disabled when test finishes
	t.Cleanup(DisableDBMigrations)

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
	ctx, err := SetupTest(t)
	if err != nil {
		t.Fatalf("Failed to setup test context: %v", err)
	}

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
	ctx, err := SetupTestWithDB(t)
	if err != nil {
		t.Fatalf("Failed to setup test context with DB: %v", err)
	}

	// Boot the environment *after* the context is stored and before running the test function
	if err := BootEnvironment(t, ctx); err != nil {
		t.Fatalf("Failed to boot test environment: %v", err)
	}

	testFunc(t, ctx)

	// Test case options are cleared by deferred ResetAllState
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
