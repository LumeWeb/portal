// Package testing provides utilities and patterns for writing integration and
// unit tests for the Portal application's core components and services.
//
// The core principle of this package is to provide **isolated, predictable
// environments for each individual test case**. This is primarily achieved
// through the `RunTestCase` and `RunTestCaseWithDB` helper functions.
//
// Key Concepts:
//
//   - `TB`: An interface satisfied by both `*testing.T` and `*testing.B`, allowing
//     test helpers to work with both test types.
//
//   - `TestContext`: An extension of `core.Context` specifically for testing.
//     It provides access to the `testing.TB` instance (`T()`), methods
//     for registering mock services (`RegisterService`), registering cleanup functions
//     (`RegisterCleanup`), accessing the test router (`Router()`), and setting the
//     database instance (`SetDB`).
//
//   - `TestContextBuilderOption`: A function type (`func(context TestContext) (TestContext, error)`)
//     used to configure a `TestContext`. Options are applied during the `BootEnvironment` phase.
//
//   - `testMutex`: An internal mutex used by `RunTestCase` and `RunTestCaseWithDB`
//     to serialize the framework's setup process, ensuring safe access to shared
//     internal state during test execution.
//
//   - `SetupTest(tb TB)`: Creates a basic `TestContext` structure for the given
//     `testing.TB`. It registers the context with `t.Cleanup` for automatic
//     teardown but *does not* apply options or run startup functions. It's
//     primarily an internal helper used by `RunTestCase` helpers. **Users should
//     generally not call this function directly.**
//
//   - `BootEnvironment(tb TB, ctx TestContext)`: The crucial step that takes a
//     `TestContext` (created by `SetupTest`) and fully initializes it. This
//     includes applying all `TestContextBuilderOption`s, running database migrations
//     (if enabled), executing all registered startup functions, configuring protocols
//     and APIs, and configuring API routes on the test router. This function makes
//     the context ready for use in test logic. It is called automatically by
//     `RunTestCase` and `RunTestCaseWithDB`. **Users should generally not call
//     this function directly.**
//
// Testing Patterns:
//
// This package supports two primary patterns for structuring tests. The first is
// the recommended approach for most individual test functions.
//
// 1. Recommended - Using `RunTestCase` or `RunTestCaseWithDB` for Individual Tests:
//
// This is the standard and preferred way to write most test functions (`func TestXxx(t *testing.T)`)
// or methods within a `testify/suite`. It ensures a fresh, isolated environment
// is created and torn down for *each* test function.
//
// `RunTestCase(t TB, testFunc func(tb TB, ctx TestContext), opts ...TestContextBuilderOption)`:
// This helper encapsulates the full setup, boot, run, and teardown cycle for a single test:
//   - Calls `SetupTest(t)` to create the basic context and register cleanup via `t.Cleanup`.
//   - Adds `DefaultTestContextOptions` and any `opts` passed directly to `RunTestCase`
//     to the context's options list.
//   - Calls `BootEnvironment(t, ctx)` to apply all options, run startup funcs, etc.
//   - Executes the provided `testFunc(tb, ctx)`.
//   - `t.Cleanup` ensures `ctx.Teardown()` is called afterwards.
//
// `RunTestCaseWithDB(t TB, testFunc func(tb TB, ctx TestContext), opts ...TestContextBuilderOption)`:
// Identical to `RunTestCase`, but automatically includes the `WithSQLite(tb)` option
// and enables DB migrations for the test context. **Crucially, this helper ensures a NEW,
// isolated, in-memory SQLite database instance is created and migrated specifically
// for each individual test case it wraps.** This guarantees test isolation at the database level.
//
// Example Test Function using `RunTestCase`:
//
//	package mypackage_test
//
//	import (
//		"testing"
//
//		"github.com/stretchr/testify/assert"
//		"github.com/stretchr/testify/mock"
//		"go.lumeweb.com/portal/core"
//		coreTesting "go.lumeweb.com/portal/core/testing"
//		"go.lumeweb.com/portal/core/testing/mocks" // Assuming mocks are here
//		"go.lumeweb.com/portal/service" // Assuming your service package is here
//	)
//
//	// No TestMain function needed in this file if you use this pattern exclusively.
//
//	func TestMyServiceIsolated(t *testing.T) {
//		// Use RunTestCase to set up and tear down a NEW, isolated test environment
//		// specifically for this test function.
//		// DefaultTestContextOptions are applied automatically.
//		coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
//			// Inside RunTestCase, the context is already set up and booted.
//			// Default options are applied (including default mocks).
//
//			// 1. Get services/dependencies from the context.
//			//    Use core.GetService or specific GetMock...Service helpers.
//			//    Cast mocks to their concrete type to set expectations.
//			myService := core.GetService[core.MyService](ctx, service.MY_SERVICE) // Get the service under test
//			assert.NotNil(tb, myService)
//
//			// Get a specific mock service to set expectations.
//			// Use the GetMock...Service helpers or type assertion if no helper exists.
//			mockDependency := coreTesting.GetMockDependencyService(ctx) // Assuming this helper exists
//			assert.NotNil(tb, mockDependency)
//
//			// 2. Set expectations on mock dependencies.
//			mockDependency.EXPECT().DoSomething().Return(nil).Once()
//
//			// 3. Perform the action being tested.
//			//    err := myService.PerformAction() // Assuming MyService has this method
//
//			// 4. Assert results and errors.
//			//    assert.NoError(tb, err)
//			//    assert.Equal(tb, expected, actual)
//
//			// Mock expectations are automatically asserted by testify/mock via t.Cleanup.
//
//		},
//			// Pass options specific to THIS test case here.
//			// Register the service under test using WrapCoreOption.
//			coreTesting.WrapCoreOption(core.ContextWithService(service.MY_SERVICE, service.NewMyService())), // Assuming NewMyService exists
//			// Add any other mocks or configurations needed ONLY for this test using WithMockServiceFactory.
//			coreTesting.WithMockServiceFactory( // Generic type parameter often inferred
//				service.MY_DEPENDENCY_SERVICE, // Assuming this service ID exists
//				mocks.NewMockMyDependencyService, // Assuming this factory exists and accepts testing.TB
//			),
//			coreTesting.WithConfig("my.service.setting", "test_value"),
//		)
//	}
//
// Example Test Function using `RunTestCaseWithDB`:
//
//	func TestMyServiceIsolatedWithDB(t *testing.T) {
//		// Use RunTestCaseWithDB to get a NEW, isolated context with a real in-memory
//		// database and migrations run specifically for this test.
//		coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
//			// Inside RunTestCaseWithDB, the context is set up with an in-memory
//			// SQLite DB, migrations are run, and the environment is booted.
//
//			// Get the real UserService instance (which uses the DB).
//			// This service is registered via an option passed to RunTestCaseWithDB.
//			userService := core.GetService[core.UserService](ctx, core.USER_SERVICE)
//			assert.NotNil(tb, userService)
//
//			// Access the database instance for this test.
//			db := ctx.DB()
//			assert.NotNil(tb, db)
//
//			// You can still get and set expectations on other default mocks
//			mockMailer := coreTesting.GetMockMailerService(ctx)
//			mockMailer.EXPECT().SendEmail(mock.Anything, mock.Anything).Return(nil).Once()
//
//			// ... test database interactions using userService and ctx.DB() ...
//			// Example: Create a user and then query it
//			// Assuming a User model exists and is included in migrations
//			// newUser := &core.User{Username: "testuser", Email: "test@example.com"}
//			// err := userService.CreateUser(ctx, newUser) // Assuming CreateUser exists
//			// require.NoError(tb, err)
//			// assert.Greater(tb, newUser.ID, uint(0)) // Check if ID was assigned
//
//			// fetchedUser, err := userService.GetUserByID(ctx, newUser.ID) // Assuming GetUserByID exists
//			// require.NoError(tb, err)
//			// assert.Equal(tb, newUser.Username, fetchedUser.Username)
//			// assert.Equal(tb, newUser.Email, fetchedUser.Email)
//		})
//	}
//
// 2. Optional - Using `TestMain` for Package-Level Setup:
//
// This approach is used when you need setup or teardown logic that runs *once*
// for the entire package test run, rather than for each individual test function.
// This is most commonly used for enabling the database globally or adding mocks/services
// that are dependencies for *many* components tested within the package.
//
// `func TestMain(m *testing.M)` is the entry point for the package's tests. It is
// called once before any tests in the package are run. Inside `TestMain`, you
// should call `RunTests` or one of its wrappers (`WithDB`, `WithDBAndOptions`, etc.).
//
// `RunTests(m *testing.M, opts TestMainOpts)`: Manages the overall test suite lifecycle.
// It handles global state reset, optional database setup/migrations, runs `m.Run()`,
// and performs cleanup. Options passed via `TestMainOpts.CustomSetup` (using
// `AddGlobalTestContextOptions`) are added to a *global* list (`globalTestCtxOpts`)
// and apply to *every* context subsequently created by `RunTestCase` or `RunTestCaseWithDB`
// within this `TestMain` execution.
//
// **Crucially:** Even when `TestMain` is used, **individual test functions must still
// call `RunTestCase` or `RunTestCaseWithDB`** to get their NEW, isolated context.
// The context created by `RunTestCase` will inherit the global options set in `TestMain`.
//
// Example TestMain:
//
//	package mypackage_test
//
//	import (
//		"testing"
//
//		"github.com/stretchr/testify/assert"
//		"github.com/stretchr/testify/mock"
//		"go.lumeweb.com/portal/core"
//		coreTesting "go.lumeweb.com/portal/core/testing"
//		"go.lumeweb.com/portal/core/testing/mocks" // Assuming mocks are here
//		"go.lumeweb.com/portal/service" // Assuming your service package is here
//	)
//
//	// TestMain is the entry point for the package's tests.
//	// It is called once before any tests in the package are run.
//	func TestMain(m *testing.M) {
//		// Use RunTests or helpers like WithDBAndOptions to manage the overall
//		// test suite lifecycle and set up a shared environment (e.g., database).
//		// Options passed here are added to a GLOBAL list (`globalTestCtxOpts`)
//		// and apply to ALL test contexts subsequently created by RunTestCase
//		// or RunTestCaseWithDB within this TestMain execution.
//		coreTesting.WithDBAndOptions(m,
//			// Add package-specific test context options here that apply globally.
//			// This is best used for truly global configurations or services
//			// that are dependencies for many components tested in this package.
//			coreTesting.WithConfig("mypackage.setting", "global_value"),
//			// Example: If a custom mock is a dependency for many services in this package,
//			// you might add it globally here using WithMockServiceFactory.
//			// coreTesting.WithMockServiceFactory(
//			// 	service.MY_GLOBAL_DEPENDENCY_SERVICE,
//			// 	mocks.NewMockMyGlobalDependencyService,
//			// ),
//		)
//
//		// Alternatively, use RunTests for more control:
//		// coreTesting.RunTests(m, coreTesting.TestMainOpts{
//		// 	WithDB: true, // Enable database
//		// 	DBMigrations: true, // Run migrations (default with WithDB)
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
//	// when using the TestMain pattern. This ensures a NEW, isolated context
//	// is created, booted, and torn down for EACH test, inheriting global options.
//	func TestMyService(t *testing.T) {
//		// RunTestCase creates a new TestContext for this specific test.
//		// It applies DefaultTestContextOptions and any global options from TestMain.
//		// It then boots the environment and runs the provided function.
//		// Teardown is handled automatically via t.Cleanup.
//		coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
//			// Inside RunTestCase, the context is already set up and booted.
//			// It inherits options from TestMain and applies default options.
//
//			// Get services from the context.
//			// Default mocks (Auth, User, etc.) are available unless overridden globally or per-test.
//			// Use core.GetService to retrieve the service interface.
//			authService := core.GetService[core.AuthService](ctx, core.AUTH_SERVICE)
//			assert.NotNil(tb, authService)
//
//			// To set expectations on a default mock, cast it to its concrete mock type.
//			mockAuthService := authService.(*mocks.MockAuthService)
//			mockAuthService.EXPECT().CheckAccess(mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(true, nil).Once()
//
//			// Get the service under test. This service should be registered via
//			// an option passed to RunTestCase, NOT globally in TestMain, unless
//			// it's a dependency for many other tests.
//			myService := core.GetService[core.MyService](ctx, service.MY_SERVICE) // Assuming MY_SERVICE ID exists
//			assert.NotNil(tb, myService)
//
//			// ... write test logic that uses myService and its dependencies ...
//			// For example, a method on myService that calls authService.CheckAccess
//			// err := myService.PerformProtectedAction(ctx, "user1", "resource", "read")
//			// assert.NoError(tb, err)
//
//			// Assert mock expectations (handled automatically by t.Cleanup via testify/mock)
//		},
//			// Pass options specific to THIS test case here. These override global options if they conflict.
//			// Register the service under test using WrapCoreOption(core.ContextWithService(...))
//			coreTesting.WrapCoreOption(core.ContextWithService(service.MY_SERVICE, service.NewMyService())), // Assuming NewMyService exists
//			// Add any other mocks or configurations needed ONLY for this test.
//			coreTesting.WithConfig("my.service.setting", "test_value"),
//		)
//	}
//
// Core Components and Concepts (Detailed):
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
//       - `WithSQLite(tb)` (if `EnableMockDB()` is called, e.g., by `WithDB` helpers or `RunTestCaseWithDB`)
//       - `WithMockAccessService(tb)`
//       - `WithDefaultRouter(...)` (a default test router)
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
//   - `TestContextBuilderOption`: As defined above, functions that modify the context.
//     They are processed sequentially during `BootEnvironment`.
//
//   - `ProcessCtxOptions(ctx TestContext, options ...TestContextBuilderOption)`:
//     Applies a slice of `TestContextBuilderOption`s to a context. Called internally
//     by `BootEnvironment`. You might need to call this manually if you register
//     components *inside* your test function using helpers like `RegisterAPI`
//     *after* the initial `BootEnvironment` call by `RunTestCase`.
//
//   - `ProcessStartupFuncs(ctx TestContext)`: Executes all registered startup functions.
//     Called internally by `BootEnvironment`.
//
//   - `ProcessExitFuncs(ctx TestContext)`: Executes all registered exit functions.
//     Called internally by `ShutdownTestContext`.
//
//   - `BootEnvironment(tb TB, ctx TestContext)`: As defined above, the main initialization
//     function called by `RunTestCase` helpers. **Users should generally not call
//     this function directly.**
//
//   - `ShutdownTestContext(ctx TestContext)`: Explicitly shuts down a `TestContext`.
//     It cancels the context, waits for completion, runs exit functions, and
//     performs cleanup. `RunTestCase` and `TestMain` helpers handle this automatically
//     via `t.Cleanup`. You typically don't call this directly.
//
//   - `WrapCoreOption(opt core.ContextBuilderOption)`: Converts a `core.ContextBuilderOption`
//     to a `TestContextBuilderOption`. Use this when adding core-level options (like
//     `core.ContextWithService`) to the list of options passed to `RunTestCase` or
//     `AddGlobalTestContextOptions`.
//
//   - `WrapCoreOptions(opts []core.ContextBuilderOption)`: Converts a slice of
//     `core.ContextBuilderOption`s to a slice of `TestContextBuilderOption`s.
//
// Working with Services and Dependencies:
//
//   - `core.GetService[T](ctx core.Context, id string) T`: The standard way to retrieve
//     a service (real or mock) from the context by its ID. You should use the generic
//     type parameter `[T]` to specify the expected interface type.
//
//   - **Integrating the Service Under Test:** The service you are specifically testing
//     in a given test function should be added to the context using an option passed
//     directly to `RunTestCase` or `RunTestCaseWithDB`. This ensures it's available
//     in that specific test's isolated environment. Use `WrapCoreOption` to include
//     core-level service registration options.
//     Example: `coreTesting.WrapCoreOption(core.ContextWithService(service.MY_SERVICE, service.NewMyService()))`
//
//   - `WithService(id string, factory core.ServiceFactory)`:
//     Creates a `TestContextBuilderOption` that registers a service using its
//     `core.ServiceFactory`. This is the recommended way to register the *real*
//     service under test or other real services needed for integration tests.
//     It wraps the core `RegisterService` logic and handles any context options
//     returned by the factory.
//
//   - `RegisterService(ctx TestContext, id string, factory core.ServiceFactory)`:
//     An internal helper function used by `WithService`. It calls
//     the service factory, registers the service instance with the core, and
//     returns any `core.ContextBuilderOption`s provided by the factory, wrapped
//     as `TestContextBuilderOption`s. Users should generally use
//     `WithService` instead of calling this directly.
//
//   - **Working with Mocks generated by Mockery (`testify/mock`):**
//     Mocks generated by `testify/mock` require a `testing.TB` instance to register
//     cleanup functions that verify expectations. The `TestContext` provides access
//     to the correct `TB` via `ctx.T()`.
//
//     - **Recommended: `WithMockServiceFactory[T any](id string, factory MockServiceFactory[T])`**:
//       This is the preferred way to add mocks generated by Mockery (`testify/mock`). It takes
//       a factory function (`MockServiceFactory[T]`) that accepts a `TB` and returns
//       the mock instance (`*T`). The framework calls this factory during the
//       `BootEnvironment` phase with the correct `TB` for the current test, ensuring
//       `mock.Mock.Test(tb)` is called correctly and expectations are verified via `t.Cleanup`.
//       Use this for any mock that needs the `TB` during creation.
//       Example:
//         `coreTesting.WithMockServiceFactory(service.MY_DEPENDENCY_SERVICE, mocks.NewMockMyDependencyService)`
//
//     - **Explicit Warning: Avoid Manual Mock Creation:** **Do NOT** manually create
//       mocks using `new(MockType)` or calling mock constructors like `mocks.NewMockXxxService(t)`
//       directly inside your test function's lambda *unless* you are absolutely certain
//       you understand the `testify/mock` lifecycle and manually handle `mock.Mock.Test(t)`
//       and expectation verification. Using `WithMockServiceFactory` is safer and
//       ensures correct integration with the testing framework's cleanup.
//
//     - `GetMockXxxService(ctx core.Context)` helpers: Convenience functions like
//       `GetMockAccessService`, `GetMockAuthService`, etc., are provided to retrieve
//       and type-assert common default mocks from the context. Use these to easily
//       access default mocks for setting expectations in your test logic.
//       Example: `mockAuthService := coreTesting.GetMockAuthService(ctx)`
//
// Database Testing:
//
//   - `RunTestCaseWithDB` is the easiest way to get a test context with database support.
//     It automatically includes the `WithSQLite(tb)` option, which sets up a real,
//     isolated, in-memory SQLite database for that specific test run. **A new database
//     instance is created and migrated for each test case wrapped by `RunTestCaseWithDB`.**
//     While this per-test isolation is highly beneficial for reliability, be aware that
//     creating and migrating a new database for every test case can introduce overhead,
//     potentially impacting the total execution time for very large test suites.
//   - Access the database instance via `ctx.DB()`. This returns a `*gorm.DB` instance
//     scoped to the test context.
//   - `RunMigrations` is handled automatically by `BootEnvironment` when the DB is enabled
//     via `WithSQLite` or `EnableDBMigrations()`.
//
// Example Database Interaction within `RunTestCaseWithDB`:
//
//	func TestDatabaseInteraction(t *testing.T) {
//		coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
//			db := ctx.DB()
//			assert.NotNil(tb, db)
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
//		})
//	}
//
// Configuration Mocking:
//
// The test context includes a mock `ConfigManager` by default.
//   - Access the mock config manager using `GetMockConfig(ctx)`.
//   - `WithMockAPIConfig(apiID string, apiConfig interface{}) TestContextBuilderOption`:
//     Recommended way to mock the configuration returned by `ctx.Config().GetAPI(apiID)`.
//     Sets a `Maybe()` expectation on the mock `ConfigManager`. Include this option
//     when testing APIs whose factory or `Init` method reads its configuration.
//   - `WithMockProtocolConfig(protocolID string, protocolConfig interface{}) TestContextBuilderOption`:
//     Recommended way to mock the configuration returned by `ctx.Config().GetProtocol(protocolID)`.
//     Sets a `Maybe()` expectation on the mock `ConfigManager`. Include this option
//     when testing Protocols whose factory or `Init` method reads its configuration.
//   - `WithMockServiceConfig(serviceID string, serviceConfig interface{}) TestContextBuilderOption`:
//     Recommended way to mock the configuration returned by `ctx.Config().GetService(serviceID)`.
//     Sets a `Maybe()` expectation on the mock `ConfigManager`. Include this option
//     when testing Services whose factory or `Init` method reads its configuration.
//
// Example Configuration Mocking with Service Registration:
//
//	import (
//		"testing"
//		"github.com/stretchr/testify/assert"
//		"go.lumeweb.com/portal/config" // Assuming your config structs are here
//		coreTesting "go.lumeweb.com/portal/core/testing"
//		"go.lumeweb.com/portal/core"
//		"go.lumeweb.com/portal/service" // Assuming your service factory is here
//	)
//
//	// Assume NewMyServiceFactory is a core.ServiceFactory that looks up its config:
//	// func NewMyServiceFactory() (core.Service, []core.ContextBuilderOption, error) { ... }
//
//	func TestMyServiceInitializationWithConfig(t *testing.T) {
//		mockServiceConfig := &config.ServiceConfig{Enabled: true, Setting: "some_value"}
//
//		coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
//			// When BootEnvironment runs, if MyService calls ctx.Config().GetService("myservice"),
//			// it will receive mockServiceConfig.
//
//			// Retrieve the service instance
//			myService := core.GetService[core.MyService](ctx, "myservice") // Assuming MyService interface exists
//			assert.NotNil(tb, myService)
//
//			// Assert that your service initialized correctly based on this config.
//			// assert.Equal(tb, mockServiceConfig.Setting, myService.GetInternalSetting()) // Assuming GetInternalSetting exists
//
//		},
//			// Provide the mock config for "myservice"
//			coreTesting.WithMockServiceConfig("myservice", mockServiceConfig),
//			// Register the service itself using the new helper
//			coreTesting.WithService("myservice", service.NewMyServiceFactory), // Assuming NewMyServiceFactory exists
//		)
//	}
//
// Testing API Routes and HTTP Interactions:
//
// The test context provides a test router (`ctx.Router()`) allowing simulation of
// HTTP requests against configured API routes.
//
// Steps:
// 1. Ensure the API is registered (e.g., using `WithAPI` passed to `RunTestCase`).
// 2. Get the test router: `testRouter := ctx.Router()`.
// 3. Create a test request: `req, err := http.NewRequest(...)`.
// 4. **Crucially:** Set the `req.Host` header correctly, especially for subdomain routers.
//    Use `ctx.Config().Config().Core.Domain` for the main domain or construct
//    `fmt.Sprintf("%s.%s", apiSubdomain, ctx.Config().Config().Core.Domain)` for subdomains.
// 5. Create a response recorder: `resp := httptest.NewRecorder()`.
// 6. Serve the request: `testRouter.ServeHTTP(resp, req)`.
// 7. Assert the response (`resp.Code`, `resp.Header()`, `resp.Body.String()`).
//
// Example API Route Testing:
//
//	import (
//		"fmt"
//		"net/http"
//		"net/http/httptest"
//		"strings"
//		"testing"
//
//		"github.com/stretchr/testify/assert"
//		"github.com/stretchr/testify/require"
//		"go.lumeweb.com/portal/core"
//		coreTesting "go.lumeweb.com/portal/core/testing"
//		"go.lumeweb.com/portal/service" // Assuming your API factory is here
//	)
//
//	// Assume NewTestAPIFactory registers an API with ID "test" and subdomain "testapi"
//	// and has a GET /status endpoint.
//
//	func TestRegisterAPIAndRoute(t *testing.T) {
//		coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
//			testRouter := ctx.Router()
//			assert.NotNil(tb, testRouter)
//
//			// Simulate an HTTP request to the API endpoint
//			req, err := http.NewRequest(http.MethodGet, "/api/v1/status", nil)
//			require.NoError(tb, err)
//
//			// IMPORTANT: Set the Host header for the subdomain
//			apiSubdomain := "testapi" // Get this from your API definition
//			fullDomain := fmt.Sprintf("%s.%s", apiSubdomain, ctx.Config().Config().Core.Domain)
//			req.Host = fullDomain
//
//			resp := httptest.NewRecorder()
//			testRouter.ServeHTTP(resp, req)
//
//			assert.Equal(tb, http.StatusOK, resp.Code)
//			// assert.Contains(tb, resp.Body.String(), "status: ok")
//
//		},
//			// Pass the API registration option directly to RunTestCase.
//			coreTesting.WithAPI("test", service.NewTestAPIFactory), // Assuming NewTestAPIFactory exists
//			// If the API factory or Init method reads config, provide a mock config option too.
//			// coreTesting.WithMockAPIConfig("test", &config.APIConfig{Enabled: true, Subdomain: "testapi"}),
//		)
//	}

// API Extension Testing:
//
// Testing API extensions requires registering both the extension and its target API.
// The testing framework provides helpers to simplify this process:
//
// Example:
//
//	func TestAPIExtension(t *testing.T) {
//		coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
//			// Create request to extension endpoint
//			req := ctx.NewAPIRequest(http.MethodGet, "/api/v1/extension-endpoint", nil)
//			resp := httptest.NewRecorder()
//			ctx.Router().ServeHTTP(resp, req)
//			assert.Equal(tb, http.StatusOK, resp.Code)
//		},
//			coreTesting.WithAPIExtension(service.NewTestAPIExtensionFactory),
//			coreTesting.WithMockAPIConfig("admin", &config.APIConfig{Enabled: true})
//		)
//	}
//
// Key Testing Components:
//
// - WithAPIExtension(factory):
//   - Registers the extension and creates mock target API
//   - Sets up proper routing for extension endpoints
//   - Configures API ID for NewAPIRequest
//
// - WithAPIID(id):
//   - Override default API ID when needed
//   - Useful for testing extensions against different APIs
//
// HTTP Request Testing Helpers:
//
// For testing API and API extension endpoints, use these helpers:
//
// - ctx.NewAPIRequest(method, path string, body []byte) *http.Request:
//   - Creates properly formatted HTTP requests for API/extension testing
//   - Automatically:
//   - Sets Content-Type: application/json
//   - Configures proper Host header using API subdomain
//   - Uses the API ID configured in the test context
//   - Usage pattern:
//     req := ctx.NewAPIRequest(http.MethodGet, "/api/v1/endpoint", nil)
//     resp := httptest.NewRecorder()
//     ctx.Router().ServeHTTP(resp, req)
//     // Assert response
//
// - ctx.Router() router.Router:
//   - Gets the test router instance with all registered routes
//   - Use with NewAPIRequest for end-to-end HTTP testing
//
// Key Requirements:
// 1. Must call ctx.SetAPIID() or use WithAPIID() option first
// 2. The target API must be registered (via WithAPI)
// 3. For extensions, the parent API must be registered
//
// Best Practices and Tips:
//
// - **Always wrap test logic in `RunTestCase` or `RunTestCaseWithDB`**. This is the most important rule for ensuring isolated and predictable test environments.
// - Use `TestMain` with `RunTests` only for package-level global setup (like enabling DB globally or adding widespread dependencies).
// - Pass test-specific options (like the service under test registration, specific mocks, or config values) directly to `RunTestCase`/`RunTestCaseWithDB`.
// - Use `WithMockServiceFactory` for adding mocks generated by Mockery (`testify/mock`).
// - Use `core.GetService[T](ctx, id)` to retrieve services (real or mock). Cast default mocks to their concrete `*mocks.MockXxxService` type (or use `GetMockXxxService` helpers) to set expectations.
// - Use `require` (from `testify/require`) for conditions that, if false, mean the test cannot continue meaningfully (e.g., setup failures).
// - Use `assert` (from `testify/assert`) for verifying the final outcome of the test logic.
// - Use `ctx.Logger()` for debugging output; it integrates with `go test -v`.
// - Choose test granularity (unit vs. integration) based on what you are trying to verify. Mock dependencies for unit tests; use real components for integration tests.
// - Remember to set the `req.Host` header when testing API routes, especially for subdomains.
//
// ---
//
// LLM Guidance Section:
//
// This section provides specific rules and preferred patterns for Large Language Models generating or modifying tests using this package.
//
// 1.  **Test File Naming and Package:** Test files MUST end with `_test.go` and reside in the same directory as the code they are testing. They MUST use the *same* package name as the code they are testing (e.g., `package mypackage` for tests of code in `package mypackage`). The filename itself should ideally reflect the specific component being tested (e.g., `api_key_service_test.go` for tests related to `api_key.go`), rather than a generic name like `service_test.go` if the file only tests one component.
// 2.  **Prioritize `RunTestCase` / `RunTestCaseWithDB`:** The primary structure for individual test functions (`func TestXxx(t *testing.T)`) MUST be a call to `coreTesting.RunTestCase` or `coreTesting.RunTestCaseWithDB` wrapping the core test logic within the provided lambda function (`func(tb coreTesting.TB, ctx coreTesting.TestContext) { ... }`).
// 3.  **`TestMain` for Global Setup:** If a `TestMain` function exists in the package, it MUST use `coreTesting.RunTests` or one of its wrappers (`WithDB`, `WithDBAndOptions`, etc.) to manage the package lifecycle and add global options via `AddGlobalTestContextOptions`. Individual test functions MUST still use `RunTestCase`/`RunTestCaseWithDB`.
// 4.  **Service Under Test Registration:** The service instance being primarily tested in a given `TestXxx` function MUST be registered using an option passed directly to the `RunTestCase` or `RunTestCaseWithDB` call. The preferred way is `coreTesting.WithService(service.MY_SERVICE_ID, service.NewMyServiceFactory)`. Do NOT add the service under test globally via `TestMain` unless it is a dependency required by *many* other components tested in the package.
// 5.  **Accessing Dependencies:** To retrieve services (real or mock) within the test lambda, use `core.GetService[T](ctx, serviceID)`. To set expectations on default mocks (like `MockAuthService`), retrieve the service using `core.GetService` and then type-assert it to its concrete mock type (e.g., `authService := core.GetService[core.AuthService](ctx, core.AUTH_SERVICE); mockAuthService := authService.(*mocks.MockAuthService)`), or use the provided `GetMockXxxService` helpers.
// 6.  **Correct Mock Creation:** When adding *new* mocks (not the default ones) that are generated by Mockery (`testify/mock`), they MUST be added using the `coreTesting.WithMockServiceFactory` option passed to `RunTestCase`/`RunTestCaseWithDB`. The factory function provided to `WithMockServiceFactory` MUST accept a `TB` argument and call `mock.Mock.Test(tb)` on the mock instance. **Strictly avoid** manually creating mocks with `new(MockType)` or calling mock constructors directly inside the test lambda without properly linking them to the test's `TB` for cleanup.
// 7.  **Database Interaction:** For tests requiring database access, use `coreTesting.RunTestCaseWithDB`. Access the database instance within the test lambda using `ctx.DB()`. **A new database instance is created and migrated for each test case wrapped by `RunTestCaseWithDB`.** While this per-test isolation is highly beneficial for reliability, be aware that creating and migrating a new database for every test case can introduce overhead, potentially impacting the total execution time for very large test suites.
// 8.  **Configuration Mocking:** Use `coreTesting.WithMockAPIConfig`, `coreTesting.WithMockProtocolConfig`, and `coreTesting.WithMockServiceConfig` options to provide mock configurations for APIs, Protocols, and Services that read their settings from the `ConfigManager`.
// 9.  **API Route Testing:** When testing HTTP routes, use `ctx.Router()`, `net/http.NewRequest`, and `httptest.NewRecorder`. Always set the `req.Host` header correctly, especially for subdomain-based APIs, using `ctx.Config().Config().Core.Domain`.
// 10. **Service Interface Location:** When referencing service interfaces (e.g., `core.AuthService`), assume the interface definition is located either in the same package as the service implementation or in a sibling package named `types` (e.g., `core/types`).
//
// ---
//
// By adhering to these guidelines, tests will be isolated, reliable, and easier to understand and maintain.
package testing
