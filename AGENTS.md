# AGENTS.md
This file provides guidance to various AI agents when working with code in this repository.

## Project Overview

Portal is a modular, plugin-based Go web application server that provides a framework for building decentralized storage and content delivery applications. The architecture is built around a plugin system where core functionality and features are implemented as pluggable components.

**Key Technologies:**
- Go 1.26.0
- GORM for database ORM
- gorilla/mux for HTTP routing (via portal-router)
- OpenTelemetry for observability
- Echo web framework
- Casbin for access control
- Goose for database migrations

## Common Commands

### Building
```bash
# Build the project using Docker (recommended)
./build.sh

# Build with debug mode enabled
XPORTAL_DEBUG=1 ./build.sh
```

### Running Tests
```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -coverprofile=coverage.out ./...

# Run specific test file
go test ./path/to/file_test.go

# Run tests with verbose output
go test -v ./...

# Run integration tests
go test ./internal/service_tests/...
```

### Running the Application
```bash
# Start the server (default command)
./dist/portal

# Run specific commands
./dist/portal version
./dist/portal config-env
```

### Code Generation
```bash
# Install mockery (v3 required for interface mocks)
go install github.com/vektra/mockery/v3@v3.7.0

# Generate mocks using mockery
mockery
```

### Database Migrations
```bash
# Migrations are handled automatically by the application
# They are registered using goose and run during initialization
# See db/db.go and core/db.go for migration handling
```

## High-Level Architecture

### Directory Structure

```
portal/
├── build/                 # Build utilities
├── cmd/                   # CLI entry points and commands
│   ├── internal/         # Internal CLI logic
│   ├── portal/           # Main application entry
│   └── portal_embed/     # Embedded CLI entry
├── config/               # Configuration management and schemas
│   └── types/            # Configuration type definitions
├── core/                 # Core framework interfaces and abstractions
│   ├── internal/        # Internal core implementation
│   ├── testing/         # Testing utilities
│   └── web_manifest/    # Web manifest utilities
├── db/                   # Database models and providers
│   ├── migrations/      # Database migration files
│   ├── mocks/           # Database mock implementations
│   ├── models/          # Data models
│   └── types/           # Custom database types
├── event/               # Event system and event handlers
├── internal/            # Internal utilities
│   ├── dns/            # DNS-related utilities
│   ├── email/          # Email utilities
│   └── reflect/        # Reflection utilities
├── pkg/                 # Package utilities
└── service/             # Service implementations
    ├── internal/       # Internal service helpers
    └── testing/        # Service testing utilities
```

### Core Architectural Patterns

#### Plugin-Based Architecture
The entire application is built around a plugin system. Everything that isn't part of the minimal core is a plugin:

- **Plugins**: Self-contained modules that can register protocols, APIs, services, models, migrations, and cron jobs
- **Protocols**: Storage backends (e.g., Sia, S3) that implement upload/download/pin functionality
- **APIs**: HTTP endpoints and REST API implementations
- **Services**: Business logic services (Auth, User, Storage, etc.)
- **Components**: Base interface that all plugins, services, and APIs implement with Context, Logger, DB, and Config

Service ID constants are defined in `core/` and follow pattern like `core.USER_SERVICE`, `core.AUTH_SERVICE`, etc.

#### Component System
All components implement the `core.Component` interface:
```go
type Component interface {
    ID() string
    Context() core.Context
    SetContext(ctx core.Context)
    Logger() *core.Logger
    SetLogger(logger *core.Logger)
    DB() *gorm.DB
    SetDB(db *gorm.DB)
    Config() config.Manager
    SetConfig(cfg config.Manager)
}
```

The `BaseComponent` struct provides a default implementation.

#### Event-Driven Initialization
The portal uses an event-driven boot sequence. Boot events are fired in order:
1. `EVENT_BOOT_START` - Boot starts
2. `EVENT_BOOT_STARTUP_FUNCS` - Run startup functions
3. `EVENT_BOOT_PLUGIN_WORKFLOWS` - Register plugin workflows
4. `EVENT_BOOT_PROTOCOL_WORKFLOWS` - Register protocol workflows  
5. `EVENT_BOOT_PROTOCOLS` - Start protocols
6. `EVENT_BOOT_CRON` - Start cron jobs
7. `EVENT_BOOT_HTTP` - Start HTTP server
8. `EVENT_BOOT_MAILER` - Start mailer
9. `EVENT_BOOT_COMPLETED` - Boot complete

Event handlers can register using `core.Listen[EventType](ctx, eventId, handler)` or helper functions like `event.OnBootProtocols(ctx, handler)`.

#### Service Registry
Services are registered with `core.RegisterService(serviceInfo, pluginId...)` and retrieved using `core.GetService[T](ctx, serviceId)`. The service registry handles dependency resolution and ordering. Core services are loaded from the `service/` package, and plugin services are loaded from plugins.

#### Database Layer
- **Models**: Registered using `db/models/models.go` and must be `db.RegisterModel(Model{})`.
- **Providers**: Abstracted database backends (MySQL, SQLite, Redis cache) via `db/provider.go`.
- **Migrations**: Auto-run Goose migrations. Core migrations in `db/migrations/` are registered via `core.RegisterDBMigration(up, down)`.
- **Retryable Transactions**: Use `db.RetryableComponentTransaction()` for operations that need retry on lock errors.
- **Model Interfaces**: Protocol-specific pin data models (e.g., `db/mocks/PinDataModel.go`) implement `core.PinDataModel`.

#### Testing Framework
The `core/testing` package provides a powerful testing framework:

**Key Concepts:**
- `TestContext`: Extended `core.Context` for testing with access to `testing.TB`, router, DB registration, cleanup
- `TestContextBuilderOption`: Functions that configure test contexts
- `RunTestCase`: Creates isolated test environments per test case
- `RunTestCaseWithDB`: Same as above but adds in-memory SQLite DB with migrations

**Recommended Pattern:**
```go
func TestMyFunction(t *testing.T) {
    coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
        // Get services/mocks using core.GetService or helpers
        // Set mock expectations
        // Run test logic
        // Assertions
    })
}
```

**Testing Helpers:**
- `EnableMockDB()`, `DisableMockDB()`, `ShouldSetupMockDB()` for DB control
- `SetupDatabaseOptions()`, `SetupMigrationOptions()` for database setup
- Mock service helpers (e.g., `coreTesting.GetMockUserService(ctx)`)
- `WithMockServiceFactory[ServiceType](serviceId, factory)` for generic mocks

**Important Rules:**
1. Always use `RunTestCase` or `RunTestCaseWithDB` for test isolation
2. Get mock services via helpers or type assertion on mocked interfaces
3. Set expectations on mocks before calling tested code
4. Use `WrapCoreOption(core.ContextWithService(id, instance))` to register real services

#### Request-Based Architecture
The request-based architecture centers on `core/workflow.go` and `core/operation.go`:

- `core.WorkflowService`: Orchestrates operation workflows
- `core.OperationHandler`: Interface for handling operations
- `models.Request`: Request model with protocol, type, status, data
- `core.StartWorkflow(name, request, opts...)`: Start a workflow

#### OpenTelemetry Integration
- `core.TraceMethod(ctx, "Method.Name")` traces methods
- `core.MetricTrack(observer, errors, f)` for error tracking
- `core.MetricTrackResult(observer, errors, f)` for multi-value returns
- Metrics are exposed per service in `service/internal/*/metrics.go`

#### Access Control
- `core.ACCESS_SERVICE` provides Casbin-based RBAC
- Custom `KeyMatchEcho` matcher for Echo path syntax
- `service/internal/access/key_matcher.go` has the matcher logic

#### Configuration
- `config.Manager` manages configuration with environment variable support
- `core.Configurable` interface and `Config()` method on components
- Config schemas defined in `config/*.go` using validation
- Use `config.NewManager()` and set up validators

#### Storage Protocols
Protocols like S3/Sia implement `core.StorageProtocol` and interfaces for downloads (`DownloadObject`), uploads (`StorageUploadRequest`), and pins (`PinHandler`).

#### HTTP Routing
- `core.HTTPService` provides the router (gorilla/mux via portal-router)
- APIs register routes in APIs
- `TestContext` has a `Router()` method for test access

### Best Practices

#### Service Implementation
- Services embed `*core.BaseComponent` for base functionality
- Use `core.GetService[T](ctx, id)` to get dependencies
- Store service ID in a constant at package level
- Implement `ID() string` returning the service constant

#### Protocol Implementation
- Protocols implement `core.StorageProtocol` or related interfaces
- Register via `core.RegisterProtocol(pluginId, protocol)`
- Implement methods for upload/download/pin operations
- Use `core.TraceMethod` for tracing

#### Error Handling
- Use domain-specific error types defined in `core/errors.go` and `core/workflow_errors.go`
- Use `core.NewAccountError`, `core.NewRequestError` for typed errors
- Always wrap errors with context

#### Testing
- Never share state between test cases
- Always prefer `RunTestCase`/`RunTestCaseWithDB` over manual setup
- Use mock helpers instead of direct instantiation where available
- Ensure mocks include core.Service interface methods

#### Database Operations
- Use GORM's auto-increment for ID fields on all models
- Always wrap DB operations in `db.RetryableComponentTransaction()` when appropriate
- Use `goose` migrations with `RegisterDBMigration(up, down)`
- Check for `gorm.ErrRecordNotFound` when appropriate
- For foreign key violations, check errors properly

#### Event Handling
- Subscribe to events using `core.Listen[EventType](ctx, eventId, handler)`
- Use helper functions defined in `event/*.go` for common events
- Fire events using `core.Fire(ctx, eventId, eventData)`
- Events carry `core.Context` and `context.Context` fields

#### Context Usage
- Always pass `core.Context` through, not `context.Context` (except where needed)
- Use `ctx.GetContext()` for the standard context
- Components store context, logger, DB, config via `Set*()` methods
- Use `core.NewContext(cfg, logger, opts...)` to build contexts with options

#### Metrics and Observability
- Add metrics in `service/internal/*/metrics.go` for new services
- Use `core.MetricTrack` and `core.MetricTrackResult` for instrumentation
- Expose metrics in service's `GetCollectors()` method

#### Plugin Development
- Define `PluginInfo` with version, dependencies, services, protocols, APIs, migrations
- Register plugins before registration of their components
- Check `core.PluginHasProtocol`, `core.PluginHasAPI` when iterating

#### Initialization Order
1. Register plugins and their metadata/configs
2. Register services (core.RegisterService)
3. Fire BootStart event
4. Run startup functions
5. Register plugin workflows
6. Register protocol workflows
7. Register protocols and APIs
8. Fire BOOT_XXX events for each subsystem
9. Fire BootCompleted event

#### Service Dependencies
- Services declare dependencies in `ServiceInfo.Depends` slice
- Service registry handles initialization order
- Use `core.GetService[T]()` to get dependencies at runtime

#### Type Assertions and Safety
- When asserting for mock objects, check for `ok` status
- Use type switches for cleaner multiple-branch assertions
- Provide clear error messages on type assertion failures

#### Logging
- Use structured logging via `ctx.Logger()`
- Use zap fields like `zap.Error(err)`, `zap.String("key", value)`
- Follow log level conventions (Info, Warn, Error)

#### Configuration
- Service configs implement `config.ServiceConfig` and `config.Defaults`
- Use `config.Manager` for centralized config management
- Environment variables prefixed by the module
- Validate configs before initialization

#### OpenTelemetry Tracing
- Use `core.TraceMethod(ctx, "function.Name")` for method spans
- Close spans via `defer span.End()`
- Pass `context.Context` through traced boundaries

#### Cleanup and Teardown
- Use `t.Cleanup()` to register cleanup in tests
- Services and protocols implement `Init()` and `Stop()` for lifecycle
- Mocks are automatically cleaned up by `mock` package

#### Model Registration
- Use `db.RegisterModel(MyModel{})` to register DB models
- Place models in `db/models/` package
- Use `models.GetModels()` to retrieve all registered models
- Ensure models have proper GORM tags for relationships and constraints

#### Workflow and Operations
- Workflows are started via `core.StartWorkflow(name, request, opts...)`
- `OperationHandler` implementations handle operations
- Requests track operation progress in database
- Use `models.NewRequest(proto, type).Build()` for request creation

#### Cron Jobs
- Implement `core.Cronable` interface for scheduled tasks
- Register via `core.RegisterCronable(service)` or plugin metadata
- Use `go-co-op/gocron/v2` for scheduling
- Cron job state stored in `db/models/cron_job.go`

#### Hash and Content Addressing
- Storage content identified via `core.StorageHash` with hash algorithms
- Use `core.HashRegistry` for hash algorithm registration
- Storage proof retrieval via protocol pin handlers

#### HTTP and Routing
- HTTP service provides `Router()` via portal-router
- Routes registered in packages' API registration
- Use gorilla/mux for pattern matching

#### Authentication and Authorization
- `core.AuthService` provides authentication (JWT, OTP, etc.)
- Use ED25519 for JWT signing
- `core.AccessControlService` is implemented using Casbin

#### Email and Notifications
- `core.Mail` service provides email sending capability
- Templates supported via `core.MailerTemplates`

#### File Uploads
- Use `go-tus` for resumable uploads
- TUS service implements upload endpoints
- Integrate with storage protocols after upload

#### Request Management
- `core.RequestService` manages request models
- Requests track operation lifecycle and progress

#### User Management
- `core.UserService` provides user CRUD, account operations
- Users have public keys for authentication
- Account deletion and password reset supported

#### Pin Management
- `core.PinService` manages pinned content tracking
- Protocol-specific pin data models for storage backends

#### Content Scanning
- Content scanning for policy enforcement
- Storage service integration for scanned content

#### Host Integration
- Integrated with Sia's renterd daemon
- Host scoring and selection via hostscore.info

#### Metrics and Monitoring
- Prometheus metrics exposed per service
- `core.MetricTrack` and `core.MetricTrackResult` helpers

