// Package testing provides utilities and patterns for writing integration and
// unit tests for the Portal application's core components and services.
//
// It offers a structured way to set up, configure, run, and tear down a
// test environment that closely mimics the production Portal application,
// including dependency injection, configuration loading, database access,
// and service lifecycle management.
//
// Two primary patterns for using this package are supported for individual test functions:
//
// 1. Using TestMain for package-level setup and teardown:
//
// This is the recommended approach for most test suites, especially when
// database migrations or other expensive setup needs to happen only once
// for all tests in a package.
//
// Example TestMain:
//
//	package mypackage_test
//
//	import (
//		"testing"
//
//		"github.com/stretchr/testify/assert"
//		"go.lumeweb.com/portal/core"
//		coreTesting "go.lumeweb.com/portal/core/testing"
//	)
//
//	func TestMain(m *testing.M) {
//		// Use RunTests or helpers like WithDBAndOptions to manage the overall
//		// test suite lifecycle and set up a shared environment (e.g., database).
//		// Options passed here apply to ALL test contexts created within this TestMain.
//		coreTesting.WithDBAndOptions(m,
//			// Add package-specific test context options here.
//			// These options will be applied to every TestContext created by
//			// RunTestCase within this TestMain execution.
//			coreTesting.WithConfigValue("mypackage.setting", "value"),
//			// Register a real service for all tests in this package if needed.
//			// coreTesting.WithRealMyService(),
//		)
//
//		// Alternatively, use RunTests for more control:
//		// coreTesting.RunTests(m, coreTesting.TestMainOpts{
//		// 	WithDB: true, // Enable database
//		// 	DBMigrations: true, // Run migrations (default with WithDB)
//		// 	TestContextOptions: []coreTesting.TestContextBuilderOption{
//		// 		// Add package-specific test context options here
//		// 		coreTesting.WithConfigValue("mypackage.setting", "value"),
//		// 		// Register a real service for all tests in this package
//		// 		// coreTesting.WithRealMyService(),
//		// 	},
//		// 	CustomSetup: func() {
//		// 		// Perform any package-level setup before tests run
//		// 	},
//		// 	CustomTeardown: func() {
//		// 		// Perform any package-level cleanup after tests run
//		// 	},
//		// })
//	}
//
//	// Individual test function using the environment set up by TestMain.
//	// ALWAYS wrap your test logic in RunTestCase or RunTestCaseWithDB
//	// when using the TestMain pattern. This ensures the context is
//	// correctly booted and torn down for each test.
//	func TestMyService(t *testing.T) {
//		coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
//			// Inside RunTestCase, the context is already set up and booted.
//			// It inherits options from TestMain and applies default options.
//
//			// Get services from the context.
//			// If WithRealMyService was used in TestMain, this will be the real service.
//			// Otherwise, it will be the default mock.
//			myService := core.GetService[core.MyService](ctx, core.MY_SERVICE)
//			assert.NotNil(tb, myService)
//
//			// If you need to interact with a mock service that wasn't explicitly
//			// replaced by a real one in TestMain options, you can retrieve it:
//			// mockDependency := coreTesting.GetMockDependencyService(ctx)
//			// mockDependency.EXPECT().DoSomething().Return(nil).Once()
//
//			// ... write test logic ...
//
//			// Assert mock expectations (handled automatically by t.Cleanup via testify/mock)
//		})
//	}
//
// In this pattern:
//   - `TestMain` is the entry point, called once before any tests in the package.
//   - `testing.RunTests` (or helpers like `testing.WithDB`, `testing.WithDBAndOptions`)
//     manages the overall setup (`SetupTestEnvironment`) and teardown (`ShutdownTestContext`) lifecycle for the *package*.
//   - `TestContextBuilderOption`s passed to `RunTests` or its helpers in `TestMain`
//     apply to *all* test contexts created within that `TestMain` execution.
//   - Individual test functions *must* call `testing.RunTestCase(t, func(tb, ctx) { ... })`
//     or `testing.RunTestCaseWithDB(...)`. These helpers internally call `testing.SetupTest(t)`
//     (which gets or creates the context inheriting global options) and then
//     `testing.BootEnvironment(t, ctx)` to fully initialize the context for that specific test.
//     They also register cleanup with `t.Cleanup` to ensure resources are released after each test.
//
// 2. Using RunTestCase for per-test setup and teardown (without TestMain):
//
// This approach is simpler when you don't have a `TestMain` or when you need
// each test to have a completely isolated environment, even if it means
// repeating setup work. This is the recommended pattern if you do not need
// package-level shared setup like database migrations.
//
// Example Test Function:
//
//	package mypackage_test
//
//	import (
//		"net/http"
//		"net/http/httptest"
//		"strings"
//		"testing"
//
//		"github.com/stretchr/testify/assert"
//		"github.com/stretchr/testify/require"
//		"go.lumeweb.com/portal/core"
//		coreTesting "go.lumeweb.com/portal/core/testing"
//		coreMocks "go.lumeweb.com/portal/core/testing/mocks" // Assuming mocks are here
//	)
//
//	// No TestMain function needed in this file.
//
//	func TestMyServiceIsolated(t *testing.T) {
//		// Use RunTestCase to set up and tear down the test environment
//		// specifically for this test function.
//		// DefaultTestContextOptions are applied automatically.
//		coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
//			// Inside RunTestCase, the context is already set up and booted.
//			// Default options are applied.
//
//			// Get services from the context (these will be mocks by default)
//			myService := core.GetService[core.MyService](ctx, core.MY_SERVICE) // Assuming MyService exists
//			assert.NotNil(tb, myService)
//
//			// Get a specific mock service to set expectations.
//			// Use the GetMock...Service helpers or type assertion if no helper exists.
//			mockDependency := coreTesting.GetMockDependencyService(ctx) // Assuming this helper exists
//			assert.NotNil(tb, mockDependency)
//
//			// Set expectation on the mock
//			mockDependency.EXPECT().DoSomething().Return(nil).Once()
//
//			// Call the service method that uses the dependency
//			// err := myService.PerformAction() // Assuming MyService has this method
//			// assert.NoError(tb, err)
//
//			// Assert that the mock expectation was met (handled automatically by t.Cleanup)
//		})
//	}
//
//	func TestMyServiceIsolatedWithDB(t *testing.T) {
//		// Use RunTestCaseWithDB to get a real in-memory database with migrations.
//		// You can pass additional TestContextBuilderOptions here that apply
//		// only to this specific test context.
//		coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
//			// Inside RunTestCaseWithDB, the context is set up with an in-memory
//			// SQLite DB, migrations are run, and the environment is booted.
//
//			// Get the real UserService instance (which uses the DB).
//			// If WithRealUserService was added as an option to RunTestCaseWithDB
//			// userService := core.GetService[core.UserService](ctx, core.USER_SERVICE)
//			// assert.NotNil(tb, userService)
//
//			// ... test database interactions ...
//
//		}, coreTesting.WithRealUserService()) // Add DB option and real service option
//	}
//
//	func TestRegisterAPI(t *testing.T) {
//		coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
//			// NOTE: Calling registration helpers like RegisterAPI *inside* the testFunc
//			// after the initial BootEnvironment call is less common and potentially
//			// less robust than passing registration options directly to RunTestCase.
//			// If you register components here, they are added *after* the initial
//			// environment boot, which might affect tests that expect components
//			// (like API routes) to be fully configured from the start.
//			// This pattern might be used for testing dynamic registration scenarios.
//
//			// Use the registration helper to get options for registering an API.
//			// These helpers return options that need to be applied to the context.
//			opts, err := coreTesting.RegisterAPI(ctx, "test", NewTestAPI) // Assuming NewTestAPI exists
//			require.NoError(tb, err)
//
//			// Process the options returned by RegisterAPI to add them to the current test context.
//			// This step is necessary because RegisterAPI prepares the options, but doesn't
//			// apply them to the context's service map or configuration immediately.
//			ctx, err = coreTesting.ProcessCtxOptions(ctx, opts...)
//			require.NoError(tb, err)
//
//			// After processing options, the environment needs to be re-booted
//			// to pick up the newly registered components (like API routes).
//			// NOTE: RunTestCase already calls BootEnvironment once. If you register
//			// components *after* the initial boot (e.g., inside the testFunc),
//			// you might need to manually call BootEnvironment again or ensure
//			// your test logic accounts for components being available after options are processed.
//			// A cleaner pattern is to pass registration options directly to RunTestCase.
//
//			// Example of passing registration options directly to RunTestCase:
//			// coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
//			//     // API registration option is processed during the initial BootEnvironment call
//			//     testAPI := core.GetAPI("test")
//			//     assert.NotNil(tb, testAPI)
//			// }, coreTesting.NewAPIRegistrationOption("test", NewTestAPI))
//
//			// If registering inside the testFunc and needing the API immediately:
//			// You might need to re-boot or structure your test differently.
//			// For now, let's assume GetAPI works after ProcessCtxOptions adds the registration info.
//			testAPI := core.GetAPI("test") // Get the globally registered API
//			assert.NotNil(tb, testAPI)
//
//			// You can now test interactions with the registered API within this context
//			// For example, testing its configuration or handlers via the test router.
//		})
//	}
//
//	func TestServiceWithMockDependency(t *testing.T) {
//		// Use RunTestCase and pass options to configure the context before booting.
//		coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
//			// Inside RunTestCase, the context is set up and booted with the provided options.
//
//			// Retrieve the mock from the context to set expectations for the test logic.
//			// Use the GetMock...Service helper if available.
//			mockDependency := coreTesting.GetMockMyDependencyService(ctx) // Assuming this helper exists
//			assert.NotNil(tb, mockDependency)
//
//			// Set expectation on the mock
//			mockDependency.EXPECT().DoSomething().Return(nil).Once()
//
//			// Get the real service that uses the dependency
//			// myService := core.GetService[core.MyService](ctx, core.MY_SERVICE) // Assuming this service exists
//			// assert.NotNil(tb, myService)
//
//			// Call the service method that uses the dependency
//			// err := myService.PerformAction()
//			// assert.NoError(tb, err)
//
//			// Assert that the mock expectation was met (handled automatically by t.Cleanup)
//		},
//			// Pass options to RunTestCase to configure the context before it's booted.
//			// Use WithMockServiceFactory for mocks that need the testing.TB instance.
//			coreTesting.WithMockServiceFactory( // Removed generic type parameter
//				core.MY_DEPENDENCY_SERVICE, // Assuming this service ID exists
//				service.NewMockMyDependencyService, // Assuming this factory exists and accepts testing.TB
//			),
//			// Add the real service that depends on the mock
//			// coreTesting.WithRealMyService(), // Assuming this option exists
//		)
//	}
//
// In this pattern:
//   - No `TestMain` is required.
//   - Each test function calls `testing.RunTestCase` or `testing.RunTestCaseWithDB`.
//   - `RunTestCase` internally calls `testing.SetupTest` (which creates a new
//     `TestContext` for this test), applies `DefaultTestContextOptions` and any
//     options passed to `RunTestCase`, then calls `testing.BootEnvironment` to
//     fully initialize the context, runs the provided `testFunc`, and automatically
//     handles teardown using `t.Cleanup()`.
//   - `TestContextBuilderOption`s passed to `RunTestCase` or `RunTestCaseWithDB`
//     apply only to that specific test context and are processed during its boot.
//   - Helpers like `RegisterAPI`, `RegisterProtocol`, `RegisterAPIExtension`,
//     and `RegisterEvents` are available to register core components. They return
//     `TestContextBuilderOption`s that *must* be applied using `ProcessCtxOptions`
//     if called *inside* the test function *after* the initial `BootEnvironment` call.
//     Prefer passing these options directly to `RunTestCase` so they are processed
//     during the initial `BootEnvironment` call.
//
// Core Components and Concepts:
//
//   - `TestContext`: An extension of `core.Context` specifically for testing.
//     It provides access to the `testing.TB` instance (`t` or `b`) and methods
//     for registering mock services (`RegisterService`) and cleanup functions
//     (`RegisterCleanup`). It also includes a test `Router`.
//
//   - `TestContextBuilderOption`: Functions used to configure a `TestContext`.
//     These are applied during the `BootEnvironment` phase. Examples include
//     `WithMockServiceFactory`, `WithConfigValue`, `WithInMemorySQLite`, etc.
//     Options are processed sequentially in the order they are provided to
//     `ProcessCtxOptions` or `RunTestCase`. The order can matter if one option's
//     configuration depends on another (e.g., setting a config value before a
//     service that reads that value is initialized).
//
//   - `TestEnvironmentOptions`: (Less commonly used directly) Functions used to
//     configure the overall test environment setup process managed by
//     `SetupTestEnvironment`. These often wrap `TestContextBuilderOption`s.
//     For most cases, passing `TestContextBuilderOption`s directly to
//     `RunTestCase` or `TestMain` helpers is sufficient.
//
//   - `SetupTest(tb TB)`: Creates a basic `TestContext` structure for the given
//     `testing.TB`. It registers the context with `t.Cleanup` for automatic
//     teardown but *does not* apply options or run startup functions. It's
//     primarily an internal helper used by `RunTestCase`. You typically don't
//     call this directly in your test functions.
//
//   - `BootEnvironment(tb TB, ctx TestContext)`: The crucial step that takes a
//     `TestContext` (created by `SetupTest`) and fully initializes it. This
//     includes:
//       - Applying all `TestContextBuilderOption`s (default, global from `TestMain`,
//         and those passed to `RunTestCase`). These options are processed during the
//         subsequent `BootEnvironment` call.
//       - Running database migrations (if enabled).
//       - Executing all registered startup functions (which may create DB connections,
//         register services from factories, etc.).
//       - Configuring protocols and APIs (loading config, calling Init).
//       - Configuring API routes on the test router.
//     This function makes the context ready for use in test logic. It is called
//     automatically by `RunTestCase` and `RunTestCaseWithDB`.
//
//   - `ShutdownTestContext(ctx TestContext)`: Explicitly shuts down a `TestContext`.
//     It cancels the context, waits for completion, runs exit functions, and
//     performs cleanup. `RunTestCase` and `TestMain` helpers handle this automatically
//     via `t.Cleanup`. You typically don't call this directly.
//
//   - `RunTests(m *testing.M, opts TestMainOpts)`: Used in `TestMain` to manage
//     the lifecycle of the entire test suite. It handles global state reset,
//     optional database setup/migrations, runs `m.Run()`, and performs cleanup.
//     Accepts `TestContextBuilderOption`s via `TestMainOpts.TestContextOptions`
//     which are added to the global options list and applied to every context
//     created by `RunTestCase` within this `TestMain` run.
//
//   - `RunTestCase(t TB, testFunc func(tb TB, ctx TestContext), opts ...TestContextBuilderOption)`:
//     **Recommended helper for individual test functions.** It encapsulates the
//     full setup, boot, run, and teardown cycle for a single test:
//       1. Calls `SetupTest(t)` to create the basic context and register cleanup.
//       2. Adds `DefaultTestContextOptions` and any `opts` passed to `RunTestCase`
//          to the context's options list. These options are processed during the
//          subsequent `BootEnvironment` call.
//       3. Calls `BootEnvironment(t, ctx)` to apply all options, run startup funcs, etc.
//       4. Executes the provided `testFunc(tb, ctx)`.
//       5. `t.Cleanup` ensures `ctx.Teardown()` is called afterwards.
//     Use this when you don't need a shared `TestMain` environment or for maximum isolation.
//
//   - `RunTestCaseWithDB(t TB, testFunc func(tb TB, ctx TestContext), opts ...TestContextBuilderOption)`:
//     Similar to `RunTestCase` but automatically includes `WithInMemorySQLite()`
//     and enables DB migrations for the test context, ensuring a real database
//     is available and migrated before your test logic runs.
//
//   - `DefaultTestContextOptions(tb TB)`: Provides a standard set of options
//     for `SetupTest`, typically including mock implementations of
//     common core services and a default router. These options are applied
//     automatically by `SetupTest` and `RunTestCase` helpers and do not
//     need to be explicitly passed unless you want to override them.
//     The default options currently include:
//       - `WithDomain("test.local")`
//       - `WithRandomSeedPhrase()`
//       - `WithCoreEvents()`
//       - `WithSQLite(tb)` (if `EnableMockDB()` is called, e.g., by `WithDB` helpers)
//       - `WithMockAccessService(tb)`
//       - `WithRouter(...)` (a default test router)
//       - `WithMockAuthService(tb)`
//       - `WithMockConfigService(tb)`
//       - `WithMockContentScannerService(tb)`
//       - `WithMockCronService(tb)`
//       - `WithMockHTTPService(tb)`
//       - `WithMockHashMappingService(tb)`
//       - `WithMockMailerService(tb)`
//       - `WithMockOTPService(tb)`
//       - `WithMockPasswordResetService(tb)`
//       - `WithMockPinService(tb)`
//       - `WithMockRequestService(tb)`
//       - `WithMockRenterService(tb)`
//       - `WithMockStorageService(tb)`
//       - `WithMockTUSService(tb)`
//       - `WithMockUserService(tb)`
//       - `WithMockWorkflowService(tb)`
//
//   - `WithInMemorySQLite()`: A `TestContextBuilderOption` that configures the
//     context to use a real, in-memory SQLite database. Useful for testing
//     database interactions without external dependencies. This is included
//     in the default options if `EnableMockDB()` is called (e.g., by `RunTestCaseWithDB`).
//
//   - `WithMockService(id string, service core.Service)`: A `TestContextBuilderOption`
//     to register a specific mock service instance directly. The service instance
//     is provided when creating the option. This can be useful for simple mock
//     structs that don't require the testing.TB instance during creation, or
//     if you have manually created a mock instance beforehand. However,
//     `WithMockServiceFactory` is generally preferred, especially for mocks
//     generated by `testify/mock`.
//
//   - `WithMockServiceFactory[T any](id string, factory MockServiceFactory[T])`: A generic `TestContextBuilderOption`
//     to register a service using a factory function that returns a specific mock type. The factory function is called
//     *during* the `BootEnvironment` phase, providing access to the `testing.TB` instance.
//     **This is the preferred way to add mocks generated by `testify/mock`** because it ensures the mock is created
//     with the correct `t.Cleanup` registration for expectation verification.
//     Use this when your mock service requires the `testing.TB` instance during its creation.
//     Note: The generic type parameter `[T any]` is often inferred by the compiler when using `testify/mock` generated
//     mocks, so it may not be explicitly needed in your test code.
//     Example:
//       coreTesting.WithMockServiceFactory( // Generic type parameter often inferred
//           service.API_KEY_SERVICE, // Assuming this service ID exists
//           service.NewMockAPIKeyService, // Assuming this factory exists and accepts testing.TB
//       )
//
//   - `WithConfigValue(key string, value interface{})`: A `TestContextBuilderOption`
//     to set a specific configuration value in the mock config manager.
//
//   - `GetMockConfig(ctx core.Context)`: Helper to retrieve the mock config manager
//     from the context for testing.
//
//   - `GetMockAccessService(ctx core.Context)` (and similar `GetMock...Service` helpers):
//     Helpers to retrieve specific mock service implementations from the context.
//     These are useful for setting expectations on mocks. Helpers exist for all
//     default mock services included in `DefaultTestContextOptions`:
//       - `GetMockAccessService`
//       - `GetMockAuthService`
//       - `GetMockConfigService`
//       - `GetMockContentScannerService`
//       - `GetMockCronService`
//       - `GetMockHTTPService`
//       - `GetMockHashMappingService`
//       - `GetMockMailerService`
//       - `GetMockOTPService`
//       - `GetMockPasswordResetService`
//       - `GetMockPinService`
//       - `GetMockRequestService`
//       - `GetMockRenterService`
//       - `GetMockStorageService`
//       - `GetMockTUSService`
//       - `GetMockUserService`
//       - `GetMockWorkflowService`
//
//   - `NewAPIRegistrationOption(id string, factory core.APIFactory)`: Creates a
//     `TestContextBuilderOption` to register and configure an API. Pass this option
//     to `RunTestCase` or `TestMain` helpers.
//
//   - `NewProtocolRegistrationOption(id string, factory core.ProtocolFactory)`: Creates a
//     `TestContextBuilderOption` to register and configure a Protocol for testing purposes.
//     Pass this option to `RunTestCase` or `TestMain` helpers.
//
//   - `RegisterAPI(ctx TestContext, id string, factory core.APIFactory)`: Registers
//     an API globally and returns `TestContextBuilderOption`s needed to integrate
//     it into the *current* test context. If called *inside* a test function
//     *after* the initial `BootEnvironment` call, you *must* apply the returned options
//     using `ProcessCtxOptions` to make the API available in that context.
//     Prefer passing `NewAPIRegistrationOption` to `RunTestCase` instead.
//
//   - `RegisterEvents(ctx TestContext, events ...core.Eventer)`: Registers events
//     globally and returns `TestContextBuilderOption`s to add them to the *current*
//     test context. If called *inside* a test function *after* the initial
//     `BootEnvironment` call, apply options with `ProcessCtxOptions`. Prefer
//     passing `WithCoreEvents` or similar options to `RunTestCase`.
//
//   - `RegisterAPIExtension(ctx TestContext, factory core.APIExtensionsFactory)`: Registers
//     API extensions globally and returns `TestContextBuilderOption`s to add them
//     to the *current* test context. If called *inside* a test function *after*
//     the initial `BootEnvironment` call, apply options with `ProcessCtxOptions`.
//     Prefer passing options created by a factory that returns
//     `TestContextBuilderOption`s directly to `RunTestCase`.
//
//   - `RegisterProtocol(ctx TestContext, id string, factory core.ProtocolFactory)`: Registers
//     a Protocol globally and returns `TestContextBuilderOption`s needed to integrate
//     it into the *current* test context. If called *inside* a test function
//     *after* the initial `BootEnvironment` call, apply options with `ProcessCtxOptions`.
//     Prefer passing `NewProtocolRegistrationOption` to `RunTestCase`.
//
//   - `ProcessCtxOptions(ctx TestContext, options ...TestContextBuilderOption)`:
//     Applies a slice of `TestContextBuilderOption`s to a context. This is called
//     internally by `BootEnvironment`. You might need to call this manually if
//     you register components *inside* your test function using helpers like
//     `RegisterAPI` *after* the initial `BootEnvironment` call by `RunTestCase`.
//
// Choosing the Right Pattern:
//
// - Use the `TestMain` pattern (`RunTests`, `WithDBAndOptions`, etc.) when:
//   - Database setup (especially migrations) is required and time-consuming,
//     and you want it to run only once for the package.
//   - You need package-level setup or teardown logic.
//   - You want to share a common environment configuration across multiple tests
//     in a package for efficiency.
//   - Individual tests *must* still use `RunTestCase` or `RunTestCaseWithDB`.
//
// - Use the `RunTestCase` (without TestMain) pattern when:
//   - You need maximum isolation between individual tests.
//   - The setup cost per test is negligible.
//   - You are writing a small number of tests in a package and don't want
//     to add a `TestMain` function.
//   - Use `RunTestCase` or `RunTestCaseWithDB` and pass `TestContextBuilderOption`s
//     directly to configure the environment for that specific test.
//
// Typical Individual Test Function Structure:
//
// Regardless of whether you use `TestMain`, the structure of an individual test function
// using this package's helpers is consistent:
//
//	func TestMyFeature(t *testing.T) {
//		// Use RunTestCase or RunTestCaseWithDB to handle setup, boot, and teardown.
//		// Pass any specific options needed for this test.
//		coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
//			// Inside this function, the context (ctx) is fully initialized and ready.
//			// tb is the testing.TB instance (*testing.T or *testing.B).
//
//			// 1. Get services/dependencies from the context.
//			//    Use core.GetService or specific GetMock...Service helpers.
//			myService := core.GetService[core.MyService](ctx, core.MY_SERVICE) // Get the service under test
//			mockDependency := coreTesting.GetMockDependencyService(ctx) // Get a mock dependency
//
//			// 2. Set expectations on mock dependencies.
//			mockDependency.EXPECT().DoSomething(mock.Anything).Return(nil).Once()
//
//			// 3. Perform the action being tested.
//			//    err := myService.PerformAction(ctx, "some_arg")
//
//			// 4. Assert results and errors.
//			//    assert.NoError(tb, err)
//			//    assert.Equal(tb, expected, actual)
//
//			// Mock expectations are automatically asserted by testify/mock via t.Cleanup.
//
//			// 5. (Optional) Interact with other core components for testing.
//
//			// Database Interaction:
//			// If using RunTestCaseWithDB or WithInMemorySQLite option, access the DB:
//			db := ctx.DB()
//
//			// Example: Create a record and then query it
//			// Assuming a User model exists and is included in migrations
//			// newUser := &User{Name: "Test User", Email: "test@example.com"}
//			// result := db.Create(newUser)
//			// assert.NoError(tb, result.Error)
//			// assert.Greater(tb, newUser.ID, uint(0)) // Check if ID was assigned
//
//			// var fetchedUser User
//			// result = db.First(&fetchedUser, newUser.ID)
//			// assert.NoError(tb, result.Error)
//			// assert.Equal(tb, newUser.Name, fetchedUser.Name)
//			// assert.Equal(tb, newUser.Email, fetchedUser.Email)
//
//			// Configuration Testing:
//			// Get the mock config manager to check or set config values during the test:
//			// mockConfig := coreTesting.GetMockConfig(ctx)
//			// value := mockConfig.GetString("some.config.key")
//			// assert.Equal(tb, "expected_value", value)
//			// mockConfig.EXPECT().GetString("another.key").Return("mocked_value").Once()
//
//			// Event Testing:
//			// Get the event manager to dispatch or listen for events:
//			// eventManager := ctx.Event()
//			// eventManager.Publish("user.created", event.M{"user_id": 123})
//			// // You might use an EventRecorder or similar pattern to assert events were published
//
//			// Testing Real Services with Mock Dependencies:
//			// If you used an option like coreTesting.WithRealMyService() and MyService
//			// depends on MockDependencyService (which is included by default),
//			// you can get the real MyService and the mock dependency, set expectations
//			// on the mock, and then call methods on the real MyService. The real service
//			// will use the mock dependency provided in the context.
//
//			// Testing Service Lifecycle (Init/Shutdown):
//			// If a service implements core.ServiceInit or core.ServiceShutdown,
//			// their Init and Shutdown methods are called automatically during
//			// BootEnvironment and ShutdownTestContext respectively. You can test
//			// the behavior of these methods by setting up the context with necessary
//			// dependencies and then asserting the state or mock interactions that
//			// should occur during Init/Shutdown.
//
//		},
//			// Optional: Pass TestContextBuilderOptions specific to this test.
//			// coreTesting.WithConfigValue("my.setting", "test_value"),
//			// coreTesting.WithMockServiceFactory(...) // Generic type parameter often inferred
//			// coreTesting.NewAPIRegistrationOption(...)
//		)
//	}
//
// Testing API Routes and HTTP Interactions:
//
// The testing package provides a test router (`ctx.Router()`) that allows you
// to simulate HTTP requests against the configured API routes within your test
// environment. This is essential for integration testing of your API handlers
// and middleware.
//
// Steps for testing an API route:
//
// 1.  **Ensure the API is Registered:** Make sure the API containing the route
//     you want to test is registered in the test context. This is typically done
//     by passing a `NewAPIRegistrationOption` to `RunTestCase` or `TestMain` helpers.
//
// 2.  **Get the Test Router:** Access the test router from the context:
//     `testRouter := ctx.Router()`
//
// 3.  **Create a Test Request:** Use `net/http.NewRequest` to create an `*http.Request`
//     object. You can set the HTTP method, path, headers, and body.
//
//     **IMPORTANT:** If the API route is registered using `router.Host()` (which is
//     common for APIs with subdomains), you **must** set the `Host` header on your
//     test request to match the expected domain. You can get the base domain from
//     the context's config and construct the full domain if needed.
//
//     Example:
//
//     ```go
//     // For a route on the main domain (e.g., portal.local)
//     req, err := http.NewRequest(http.MethodGet, "/api/v1/status", nil)
//     require.NoError(tb, err)
//     req.Host = ctx.Config().Config().Core.Domain // Set Host header for the main domain
//
//     // For a route on a subdomain (e.g., myapi.portal.local)
//     // Assuming "myapi" is the subdomain configured for the API
//     apiSubdomain := "myapi" // Get this from your API definition or config
//     fullDomain := fmt.Sprintf("%s.%s", apiSubdomain, ctx.Config().Config().Core.Domain)
//     req, err := http.NewRequest(http.MethodPost, "/api/v1/items", strings.NewReader(`{"name": "test"}`))
//     require.NoError(tb, err)
//     req.Header.Set("Content-Type", "application/json")
//     req.Host = fullDomain // Set Host header for the subdomain
//     ```
//
// 4.  **Create a Response Recorder:** Use `net/http/httptest.NewRecorder` to create
//     an `*httptest.ResponseRecorder`. This object will capture the response written
//     by the handler.
//
//     ```go
//     resp := httptest.NewRecorder()
//     ```
//
// 5.  **Serve the Request:** Pass the response recorder and the request to the router's
//     `ServeHTTP` method.
//
//     ```go
//     testRouter.ServeHTTP(resp, req)
//     ```
//
// 6.  **Assert the Response:** Check the captured response details in the `resp` object.
//
//     ```go
//     // Assert status code
//     assert.Equal(tb, http.StatusOK, resp.Code)
//
//     // Assert headers
//     assert.Equal(tb, "application/json", resp.Header().Get("Content-Type"))
//
//     // Assert body content (as string or unmarshal JSON)
//     assert.Contains(tb, resp.Body.String(), "expected content")
//
//     // Example: Unmarshal JSON body
//     // var responseBody map[string]interface{}
//     // err = json.Unmarshal(resp.Body.Bytes(), &responseBody)
//     // require.NoError(tb, err)
//     // assert.Equal(tb, "success", responseBody["status"])
//     ```
//
// Testing Middleware (e.g., Authentication):
//
// If your route is protected by middleware (like the default authentication middleware
// added by `router.AuthSwagger`), you need to include the necessary headers in your
// test request to satisfy the middleware.
//
// For authentication middleware, this typically involves adding an `Authorization`
// header with a valid token. You would need to mock or simulate the authentication
// service to generate or validate these tokens in your test setup.
//
// Example with Authentication:
//
// ```go
// // Assuming you have a mock AuthService and a way to generate a test token
// mockAuthService := coreTesting.GetMockAuthService(ctx)
// testToken := "fake-jwt-token" // Or generate a real test token
//
// // Set expectation on the mock AuthService if the middleware calls it
// // This depends on how your auth middleware interacts with the service
// // mockAuthService.EXPECT().ValidateToken(testToken, mock.Anything).Return(&core.User{ID: 1}, nil).Once()
//
// req, err := http.NewRequest(http.MethodGet, "/api/v1/protected", nil)
// require.NoError(tb, err)
// req.Host = ctx.Config().Config().Core.Domain // Or subdomain host
// req.Header.Set("Authorization", "Bearer " + testToken) // Add the Authorization header
//
// resp := httptest.NewRecorder()
// testRouter := ctx.Router()
// testRouter.ServeHTTP(resp, req)
//
// assert.Equal(tb, http.StatusOK, resp.Code) // Or http.StatusUnauthorized if token is invalid/missing
// // ... assert body etc.
// ```
//
// By using `ctx.Router()`, `net/http.NewRequest`, `httptest.NewRecorder`, and
// asserting the response details, you can effectively test the behavior of your
// API routes and the middleware applied to them within the isolated test environment.
//
// Advanced Test Setup Scenarios:
//
// The flexibility of `TestContextBuilderOption`s allows for advanced test setup
// scenarios, such as overriding default mock services or adding entirely new custom services.
//
// Overriding Default Mocks:
//
// By default, `RunTestCase` and `TestMain` helpers include a standard set of mock
// services (`DefaultTestContextOptions`). You can replace any of these default mocks
// with a different mock implementation or a real service implementation by providing
// a `TestContextBuilderOption` that registers a service with the *same ID* as the
// default mock. Options are processed in order, so options provided later will
// override earlier ones.
//
// Example: Replacing the default MockUserService with a real UserService (assuming it exists and is testable):
//
// ```go
// func TestRealUserServiceWithMockDependencies(t *testing.T) {
// 	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
// 		// Inside this test, the default MockUserService is replaced by the real one.
// 		// However, its dependencies (like the DB, Config, etc.) are still the default mocks
// 		// unless explicitly overridden.
//
// 		userService := core.GetService[core.UserService](ctx, core.USER_SERVICE)
// 		require.NotNil(tb, userService)
//
// 		// You can still get and set expectations on the *other* default mocks
// 		mockMailer := coreTesting.GetMockMailerService(ctx)
// 		mockMailer.EXPECT().SendEmail(mock.Anything, mock.Anything).Return(nil).Once()
//
// 		// ... test the real userService ...
//
// 	},
// 		// This option replaces the default MockUserService
// 		coreTesting.WithRealUserService(), // Assuming this option exists and registers a real UserService
// 		// You might need to add options for the real service's dependencies if they aren't covered by defaults
// 		// coreTesting.WithRealDependencyService(),
// 	)
// }
// ```
//
// Example: Replacing the default MockUserService with a *different* mock implementation:
//
// ```go
// // Assuming you have a custom mock implementation called MyCustomMockUserService
// type MyCustomMockUserService struct {
// 	mock.Mock
// }
//
// func NewMyCustomMockUserService(tb testing.TB) *MyCustomMockUserService {
// 	m := &MyCustomMockUserService{}
// 	m.Mock.Test(tb) // Link mock to the test for automatic expectation verification
// 	return m
// }
//
// // Implement core.UserService methods on MyCustomMockUserService
// // func (m *MyCustomMockUserService) CreateUser(...) error { ... }
//
// func TestWithCustomMockUserService(t *testing.T) {
// 	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
// 		// Get the custom mock service
// 		customMockUserSvc := core.GetService[core.UserService](ctx, core.USER_SERVICE)
// 		require.NotNil(tb, customMockUserSvc)
//
// 		// Assert that it's your custom mock type
// 		_, ok := customMockUserSvc.(*MyCustomMockUserService)
// 		assert.True(tb, ok, "UserService should be MyCustomMockUserService")
//
// 		// Set expectations on your custom mock
// 		// customMockUserSvc.(*MyCustomMockUserService).EXPECT().CreateUser(...).Return(...)
//
// 		// ... test logic ...
//
// 	},
// 		// This option replaces the default MockUserService with your custom mock
// 		coreTesting.WithMockServiceFactory(
// 			core.USER_SERVICE,
// 			NewMyCustomMockUserService, // Use your custom mock factory
// 		),
// 	)
// }
// ```
//
// Adding Custom Services:
//
// If you have a service that is not part of the core package's default mocks,
// you can add it to the test context.
//
// Preferred Method (using TestContextBuilderOption):
// Create a `TestContextBuilderOption` that registers your service. This ensures
// the service is initialized correctly during the `BootEnvironment` phase.
// Use `WithMockServiceFactory` for mocks generated by `testify/mock` or
// `WithMockService` for simple mock structs or real service instances that
// don't require the `testing.TB` instance during creation.
//
// Example: Adding a custom mock service using `WithMockServiceFactory`:
//
// ```go
// // Assuming a custom service interface and its mock exist
// type MyCustomService interface {
// 	DoSomethingCustom() error
// }
//
// // Assuming a mock generated by testify/mock: MockMyCustomService
// // and a factory function: NewMockMyCustomService(tb testing.TB) *MockMyCustomService
// const MY_CUSTOM_SERVICE = "my_custom_service" // Define a unique ID for your service
//
// func TestWithCustomService(t *testing.T) {
// 	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
// 		// Get your custom mock service
// 		customSvc := core.GetService[MyCustomService](ctx, MY_CUSTOM_SERVICE)
// 		require.NotNil(tb, customSvc)
//
// 		// Set expectations on the custom mock
// 		customSvc.(*MockMyCustomService).EXPECT().DoSomethingCustom().Return(nil).Once()
//
// 		// ... test logic that uses the custom service ...
//
// 	},
// 		// Add the custom mock service to the context
// 		coreTesting.WithMockServiceFactory(
// 			MY_CUSTOM_SERVICE,
// 			NewMockMyCustomService,
// 		),
// 	)
// }
// ```
//
// Alternative Method (using ctx.RegisterService inside testFunc):
// You can call `ctx.RegisterService(id, serviceInstance)` directly inside your
// test function *after* the `BootEnvironment` call (which is handled by `RunTestCase`).
// However, this is generally discouraged because the service's `Init` method (if it
// implements `core.ServiceInit`) will *not* be called automatically. This method
// is only suitable for very simple services or mocks that have no initialization
// logic or dependencies that need to be resolved during boot.
//
// ```go
// func TestRegisterServiceInsideTestFunc(t *testing.T) {
// 	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
// 		// Create a simple mock instance
// 		simpleMock := &struct{ mock.Mock }{}
// 		simpleMock.Test(tb)
//
// 		// Register it directly (Init will NOT be called)
// 		ctx.RegisterService("my_simple_mock", simpleMock)
//
// 		// Get the service
// 		retrievedMock := ctx.Service("my_simple_mock")
// 		assert.NotNil(tb, retrievedMock)
//
// 		// Set expectations and test...
// 	})
// }
// ```
//
// Best Practices and Tips:
//
// - **Naming Conventions:**
//   - Test files should end with `_test.go`.
//   - Test functions should start with `Test` followed by the name of the function
//     or feature being tested (e.g., `TestCreateUser`, `TestGetUserByID`).
//   - Use descriptive names that indicate what the test is verifying.
//   - For tests using the `TestMain` pattern, the file containing `TestMain` might
//     be named `main_test.go` or similar, although this is not strictly required.
//
// - **Test Granularity (Unit vs. Integration):**
//   - **Unit Tests:** Focus on testing individual components (e.g., a single service method)
//     in isolation. Mock out all dependencies using `WithMockServiceFactory` or
//     `WithMockService`. These tests are fast and help pinpoint issues in specific units of code.
//   - **Integration Tests:** Test the interaction between multiple components (e.g., a service
//     interacting with the database, or an API handler using a service). Use real implementations
//     for the components under integration (e.g., `WithInMemorySQLite`, `WithRealUserService`)
//     while still mocking out external dependencies or services not directly involved in the
//     integration being tested. These tests are slower but provide confidence that components
//     work together correctly.
//   - The testing framework supports both granular unit tests (by default providing mocks)
//     and integration tests (by allowing you to swap mocks for real implementations and
//     configure components like the database). Choose the granularity appropriate for what
//     you are trying to verify.
//
// - **Using `require` vs `assert`:**
//   - Use `require` (from `github.com/stretchr/testify/require`) for conditions that, if
//     false, mean the test cannot continue meaningfully. `require` functions call `tb.FailNow()`
//     on failure, stopping the test immediately. Examples: checking for errors during setup,
//     ensuring a dependency was retrieved successfully, unmarshalling JSON.
//   - Use `assert` (from `github.com/stretchr/testify/assert`) for verifying the final
//     outcome of the test logic. `assert` functions call `tb.Fail()` on failure but allow
//     the test to continue, which can be useful for reporting multiple failures in a single test.
//
// - **Debugging Tests:**
//   - **Logger Output:** The test context includes a logger (`ctx.Logger()`) which is
//     configured to output to `testing.TB`'s log. Use `ctx.Logger().Debug(...)` or
//     `ctx.Logger().Info(...)` to add debugging output to your tests. This output
//     is only shown when the test fails or when running with the `-v` flag (`go test -v`).
//   - **Print Statements:** Standard `fmt.Println` or `log.Println` can also be used,
//     but `ctx.Logger()` is generally preferred as it integrates with the testing framework's
//     output handling.
//   - **IDE Debugger:** Use your IDE's debugger to set breakpoints and step through
//     your test code and the framework's setup/boot process.
//   - **`tb.Logf` / `tb.Log`:** Direct calls to `tb.Logf` or `tb.Log` also output
//     to the test log and can be useful for simple debugging messages.
//   - **Mock Debugging:** `testify/mock` provides debugging features. You can enable
//     verbose logging for mocks if needed, although the default setup with `tb.Cleanup`
//     and `mock.Mock.Test(tb)` usually provides sufficient information on expectation failures.
//
// By following these patterns and tips, you can write effective, maintainable, and
// debuggable tests for your Portal components and services using this testing framework.
package testing
