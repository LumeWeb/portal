// Package testing provides utilities and patterns for writing integration and
// unit tests for the Portal application's core components and services.
//
// It offers a structured way to set up, configure, run, and tear down a
// test environment that closely mimics the production Portal application,
// including dependency injection, configuration loading, database access,
// and service lifecycle management.
//
// Two primary patterns for using this package are supported:
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
//		"go.lumeweb.com/portal/core/testing"
//	)
//
//	func TestMain(m *testing.M) {
//		// Run tests with an in-memory SQLite database enabled by default.
//		// This allows tests that need a database to use a real one without
//		// external dependencies. Migrations are also enabled by default.
//		// You can pass additional TestContextBuilderOptions here that will
//		// apply to all test contexts created within this TestMain.
//		testing.WithDBAndOptions(m,
//			// Add package-specific test context options here
//			testing.WithConfigValue("mypackage.setting", "value"),
//			// Register a real service for all tests in this package
//			// testing.WithRealMyService(),
//		)
//
//		// Alternatively, use RunTests for more control:
//		// testing.RunTests(m, testing.TestMainOpts{
//		// 	WithDB: true, // Enable database
//		// 	DBMigrations: true, // Run migrations (default with WithDB)
//		// 	TestContextOptions: []testing.TestContextBuilderOption{
//		// 		// Add package-specific test context options here
//		// 		testing.WithConfigValue("mypackage.setting", "value"),
//		// 		// Register a real service for all tests in this package
//		// 		// testing.WithRealMyService(),
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
//	// Individual test function using the environment set up by TestMain
//	func TestMyService(t *testing.T) {
//		// SetupTest retrieves or creates the TestContext for this test.
//		// Since TestMain configured the environment, this context will
//		// inherit those settings (like the in-memory DB and any options
//		// passed to WithDBAndOptions).
//		ctx := testing.SetupTest(t)
//
//		// Get services from the context
//		// If WithRealMyService was used in TestMain, this will be the real service.
//		// Otherwise, it will be the default mock.
//		myService := core.GetService[core.MyService](ctx, core.MY_SERVICE)
//		assert.NotNil(t, myService)
//
//		// If you need to interact with a mock service that wasn't explicitly
//		// replaced by a real one in TestMain options, you can retrieve it:
//		// mockDependency := testing.GetMockDependencyService(ctx)
//		// mockDependency.EXPECT().DoSomething().Return(nil).Once()
//
//		// ... write test logic ...
//	}
//
// In this pattern:
//   - `TestMain` is the entry point, called once before any tests in the package.
//   - `testing.RunTests` (or helpers like `testing.WithDB`, `testing.WithDBAndOptions`)
//     manages the overall setup (`SetupTestEnvironment`) and teardown (`ShutdownTestContext`) lifecycle.
//   - `TestContextBuilderOption`s passed to `RunTests` or its helpers in `TestMain`
//     apply to *all* test contexts created within that `TestMain` execution.
//   - Individual test functions call `testing.SetupTest(t)` to get the context.
//     `SetupTest` is optimized to reuse the environment configured by `TestMain`
//     if one exists for the current `testing.T`. It also registers cleanup
//     with `t.Cleanup` to ensure resources are released after each test.
//
// 2. Using RunTestCase for per-test setup and teardown:
//
// This approach is simpler when you don't have a `TestMain` or when you need
// each test to have a completely isolated environment, even if it means
// repeating setup work.
//
// Example Test Function:
//
//	package mypackage_test
//
//	import (
//		"testing"
//
//		"github.com/stretchr/testify/assert"
//		"go.lumeweb.com/portal/core"
//		coreTesting "go.lumeweb.com/portal/core/testing"
//		coreMocks "go.lumeweb.com/portal/core/testing/mocks"
//	)
//
//	// No TestMain function needed in this file.
//
//	func TestMyServiceIsolated(t *testing.T) {
//		// Use RunTestCase to set up and tear down the test environment
//		// specifically for this test function.
//		// DefaultTestEnvironmentOptions are applied automatically.
//		coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
//			// Get services from the context (these will be mocks by default)
//			myService := core.GetService[core.MyService](ctx, core.MY_SERVICE)
//			assert.NotNil(tb, myService)
//
//			// Get a specific mock service to set expectations
//			mockDependency := coreTesting.GetMockDependencyService(ctx) // Use helper
//			assert.NotNil(tb, mockDependency)
//
//			// Set expectation on the mock
//			mockDependency.EXPECT().DoSomething().Return(nil).Once()
//
//			// Call the service method that uses the dependency
//			// err := myService.PerformAction() // Assuming MyService has this method
//			// assert.NoError(tb, err)
//
//			// Assert that the mock expectation was met
//			mockDependency.AssertExpectations(tb)
//		})
//	}
//
//	func TestMyServiceIsolatedWithDB(t *testing.T) {
//		// Use RunTestCaseWithDB to get a real in-memory database with migrations.
//		// You can pass additional TestContextBuilderOptions here that apply
//		// only to this specific test context.
//		coreTesting.RunTestCaseWithDB(t, func(t coreTesting.TB, ctx coreTesting.TestContext) {
//			// Get the real UserService instance (which uses the DB)
//			// If WithRealUserService was added as an option to RunTestCaseWithDB
//			// userService := core.GetService[core.UserService](ctx, core.USER_SERVICE)
//			// assert.NotNil(t, userService)
//
//			// ... test database interactions ...
//
//		}, coreTesting.WithRealUserService()) // Add DB option and real service option
//	}
//
//	func TestRegisterAPI(t *testing.T) {
//		coreTesting.RunTestCase(t, func(t coreTesting.TB, ctx coreTesting.TestContext) {
//			// Use the new helper to register an API within a test case
//			opts, err := coreTesting.RegisterAPI(ctx, "test", NewTestAPI) // Assuming NewTestAPI exists
//			require.NoError(t, err)
//
//			// Process the options returned by RegisterAPI to add them to the context
//			// This step is necessary because RegisterAPI returns options that need
//			// to be applied to the current test context.
//			_, err = coreTesting.ProcessCtxOptions(ctx, opts...)
//			require.NoError(t, err)
//
//			// Now the API should be registered and configured in the test context
//			testAPI := core.GetAPI("test")
//			assert.NotNil(t, testAPI)
//
//			// You can now test interactions with the registered API within this context
//			// For example, testing its configuration or handlers via the test router.
//		})
//	}
//
//	func TestServiceWithMockDependency(t *testing.T) {
//		coreTesting.RunTestCase(t, func(t coreTesting.TB, ctx coreTesting.TestContext) {
//			// Use WithMockServiceFactory to provide a mock that needs the testing.TB instance.
//			// This factory function will be called during BootEnvironment.
//			ctx, err := coreTesting.ProcessCtxOptions(ctx,
//				coreTesting.WithMockServiceFactory(core.MY_DEPENDENCY_SERVICE, func(tb coreTesting.TB) interface{} {
//					// Create the mock using the provided TB
//					mockDependency := coreMocks.NewMockMyDependencyService(tb) // Assuming this mock exists
//					// Set expectations here if needed for the mock's initialization phase
//					return mockDependency
//				}),
//				// Add the real service that depends on the mock
//				// coreTesting.WithRealMyService(), // Assuming this option exists
//			)
//			require.NoError(t, err)
//
//			// Retrieve the mock from the context to set expectations for the test logic
//			mockDependency := coreTesting.GetMockMyDependencyService(ctx) // Assuming this helper exists
//			assert.NotNil(t, mockDependency)
//
//			// Set expectation on the mock for the test logic
//			mockDependency.EXPECT().DoSomething().Return(nil).Once()
//
//			// Get the real service that uses the dependency
//			// myService := core.GetService[core.MyService](ctx, core.MY_SERVICE)
//			// assert.NotNil(t, myService)
//
//			// Call the service method that uses the dependency
//			// err := myService.PerformAction()
//			// assert.NoError(t, err)
//
//			// Assert that the mock expectation was met (handled by t.Cleanup)
//		})
//	}
//
// In this pattern:
//   - No `TestMain` is required.
//   - Each test function calls `testing.RunTestCase` or `testing.RunTestCaseWithDB`.
//   - `RunTestCase` internally calls `testing.SetupTest` (which creates a new
//     `TestContext` for this test) and registers `ctx.Teardown()` with `t.Cleanup()`.
//   - `DefaultTestContextOptions` are applied to the context by default.
//   - Additional `TestContextBuilderOption`s can be passed to `RunTestCase` or
//     `RunTestCaseWithDB` to customize the environment for that specific test.
//   - Helpers like `RegisterAPI`, `RegisterProtocol`, `RegisterAPIExtension`,
//     and `RegisterEvents` are available to register core components within
//     a test context. They return `TestContextBuilderOption`s that must be
//     applied using `ProcessCtxOptions`.
//
// Core Components and Concepts:
//
//   - `TestContext`: An extension of `core.Context` specifically for testing.
//     It provides access to the `testing.TB` instance (`t` or `b`) and methods
//     for registering mock services (`RegisterService`) and cleanup functions
//     (`RegisterCleanup`). It also includes a test `Router`.
//
//   - `TestContextBuilderOption`: Functions used to configure a `TestContext`.
//     Examples include `WithMockService`, `WithConfigValue`, `WithInMemorySQLite`, etc.
//
//   - `TestEnvironmentOptions`: Functions used to configure the overall test
//     environment setup process managed by `SetupTestEnvironment`. These often
//     wrap `TestContextBuilderOption`s.
//
//   - `SetupTestEnvironment(tb TB, opts ...TestEnvironmentOptions)`: The core
//     function for building a fully initialized `TestContext`. It resets global
//     state, creates a new context, applies default and provided options, and
//     calls `BootEnvironment`.
//
//   - `BootEnvironment(ctx TestContext)`: Simulates the core application startup
//     sequence within the test context. This includes processing context options,
//     running DB migrations (if enabled), configuring protocols and APIs, and
//     configuring API routes.
//
//   - `ShutdownTestContext(ctx TestContext)`: Explicitly shuts down a `TestContext`.
//     It cancels the context, waits for completion, runs exit functions, and
//     performs cleanup. `RunTestCase` and `TestMain` helpers handle this automatically
//     via `t.Cleanup`.
//
//   - `RunTests(m *testing.M, opts TestMainOpts)`: Used in `TestMain` to manage
//     the lifecycle of the entire test suite. It handles global state reset,
//     optional database setup/migrations, runs `m.Run()`, and performs cleanup.
//     Accepts `TestContextBuilderOption`s via `TestMainOpts.TestContextOptions`.
//
//   - `RunTestCase(t TB, testFunc func(t TB, ctx TestContext), opts ...TestContextBuilderOption)`:
//     A convenience helper for individual test functions. It calls `SetupTest(t)`,
//     runs the provided `testFunc` with the context, and automatically handles
//     teardown using `t.Cleanup`. Accepts `TestContextBuilderOption`s that apply
//     only to this test case.
//
//   - `RunTestCaseWithDB(t TB, testFunc func(t TB, ctx TestContext), opts ...TestContextBuilderOption)`:
//     Similar to `RunTestCase` but automatically includes `WithInMemorySQLite()`
//     and enables DB migrations for the test context.
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
//     in the default options if `EnableMockDB()` is called.
//
//   - `WithMockService(id string, service core.Service)`: A `TestContextBuilderOption`
//     to register a specific mock service implementation in the context. Use this
//     when you have a pre-created mock instance that does not require the
//     `testing.TB` instance during its construction or initial setup.
//
//   - `WithMockServiceFactory[T any](id string, factory MockServiceFactory[T])`: A generic `TestContextBuilderOption`
//     to register a service using a factory function that returns a specific mock type. The factory function is called
//     during the `BootEnvironment` phase, providing access to the `testing.TB` instance.
//     Use this when your mock service requires the `testing.TB` instance during its creation (e.g., for mocks generated 
//     by `testify/mock` which use `t.Cleanup`). The generic type parameter ensures type safety when registering mocks.
//     Example:
//       coreTesting.WithMockServiceFactory[service.MockAPIKeyService](
//           service.API_KEY_SERVICE,
//           service.NewMockAPIKeyService,
//       )
//
//   - `WithConfigValue(key string, value interface{})`: A `TestContextBuilderOption`
//     to set a specific configuration value in the mock config manager.
//
//   - `GetMockConfig(ctx core.Context)`: Helper to retrieve the mock config manager
//     from the context for setting expectations or inspecting state.
//
//   - `GetMockAccessService(ctx core.Context)` (and similar `GetMock...Service` helpers):
//     Helpers to retrieve specific mock service implementations from the context.
//
//   - `NewAPIRegistrationOption(id string, factory core.APIFactory)`: Creates a
//     `TestContextBuilderOption` to register and configure an API.
//
//   - `NewProtocolRegistrationOption(id string, factory core.ProtocolFactory)`: Creates a
//     `TestContextBuilderOption` to register and configure a Protocol.
//
//   - `RegisterAPI(ctx TestContext, id string, factory core.APIFactory)`: Registers
//     an API directly within a test function and returns options to apply.
//
//   - `RegisterProtocol(ctx TestContext, id string, factory core.ProtocolFactory)`: Registers
//     a Protocol directly within a test function and returns options to apply.
//
//   - `RegisterAPIExtension(ctx TestContext, factory core.APIExtensionsFactory)`: Registers
//     API extensions directly within a test function and returns options to apply.
//
//   - `RegisterEvents(ctx TestContext, events ...core.Eventer)`: Registers events
//     directly within a test function and returns options to apply.
//
//   - `ProcessCtxOptions(ctx TestContext, options ...TestContextBuilderOption)`:
//     Applies a slice of `TestContextBuilderOption`s to a context. Necessary after
//     calling registration helpers like `RegisterAPI`.
//
// Choosing the Right Pattern:
//
// - Use the `TestMain` pattern when:
//   - Database setup (especially migrations) is required and time-consuming.
//   - You need package-level setup or teardown logic.
//   - You want to share a common environment configuration across multiple tests
//     in a package for efficiency.
//   - Use `WithDBAndOptions` or `RunTests` with `TestMainOpts.TestContextOptions`
//     to configure the shared environment.
//
// - Use the `RunTestCase` (without TestMain) pattern when:
//   - You need maximum isolation between individual tests.
//   - The setup cost per test is negligible.
//   - You are writing a small number of tests in a package and don't want
//     to add a `TestMain` function.
//   - Use `RunTestCase` or `RunTestCaseWithDB` and pass `TestContextBuilderOption`s
//     directly to configure the environment for that specific test.
//   - Use registration helpers (`RegisterAPI`, etc.) followed by `ProcessCtxOptions`
//     to register core components within the test function.
package testing
