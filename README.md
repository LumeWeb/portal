# Portal

Portal is a modular, plugin-based Go web application server that provides a framework for building decentralized storage and content delivery applications. The architecture is built around a plugin system where core functionality and features are implemented as pluggable components.

[![Go Version](https://img.shields.io/badge/Go-1.26.0-blue)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow)](LICENSE)

## Features

- **Plugin Architecture**: Fully modular design where all functionality is implemented as pluggable components
- **Storage Protocols**: Support for multiple storage backends including Sia, S3, and custom protocols
- **Event-Driven Initialization**: Sophisticated boot sequence with event-driven component startup
- **Comprehensive Testing Framework**: Built-in testing utilities with `TestContext`, mock helpers, and isolated test environments
- **Request-Based Workflows**: Orchestrated operation workflows for upload, download, and pin operations
- **OpenTelemetry Integration**: Full observability with tracing and metrics support
- **Access Control**: Casbin-based RBAC with custom Echo path matching
- **Database Migrations**: Automatic migration handling via Goose
- **Cron Job Scheduling**: Built-in scheduling with go-co-op/gocron
- **TUS Resumable Upload Support**: Resumable file uploads via go-tus integration

## Architecture

Portal uses a **plugin-based architecture** where the core provides minimal infrastructure and all features are implemented as plugins. This design enables:

- **Plugins**: Self-contained modules that register protocols, APIs, services, models, migrations, and cron jobs
- **Components**: Base interface that all plugins, services, and APIs implement with Context, Logger, DB, and Config
- **Services**: Business logic layer (Auth, User, Storage, etc.) registered via service registry
- **Protocols**: Storage backends implementing upload/download/pin functionality

### Core Interfaces

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

## Installation

### Prerequisites

- Go 1.26.0 or later
- Docker (for containerized builds)

### Building the Project

```bash
# Build the project using Docker (recommended)
./build.sh

# Build with debug mode enabled
XPORTAL_DEBUG=1 ./build.sh
```

The build script produces a binary at `./dist/portal`.

## Quick Start

### Starting the Server

```bash
# Start the server (default command)
./dist/portal
```

### Available Commands

```bash
# Display version information
./dist/portal version

# Display detailed version information
./dist/portal version --full

# Print available environment variables and their values
./dist/portal config-env
```

## Configuration

Portal uses a centralized configuration system with environment variable support. Configuration schemas are defined in the `config/` package with validation.

Key configuration areas:
- **Database**: MySQL, SQLite, and Redis cache providers
- **Storage**: S3, Sia, and custom storage protocols
- **Observability**: OpenTelemetry tracing and metrics
- **Cluster**: Distributed node configuration
- **Cron**: Job scheduling settings

## Development

### Building and Testing

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

### Code Generation

Mockery is pre-installed for generating mock implementations:

```bash
# Generate mocks using mockery
$HOME/go/bin/mockery
```

### Testing Framework

The `core/testing` package provides a powerful testing framework:

**Key Components:**
- `TestContext`: Extended `core.Context` for testing with access to `testing.TB`, router, DB registration, cleanup
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

### Common Development Patterns

#### Service Implementation
- Services embed `*core.BaseComponent` for base functionality
- Use `core.GetService[T](ctx, id)` to get dependencies
- Store service ID in a constant at package level
- Implement `ID() string` returning the service constant

#### Event Handling
- Subscribe to events using `core.Listen[EventType](ctx, eventId, handler)`
- Use helper functions defined in `event/*.go` for common events
- Fire events using `core.Fire(ctx, eventId, eventData)`

#### Database Operations
- Use `db.RetryableComponentTransaction()` for retryable operations
- Use `goose` migrations with `RegisterDBMigration(up, down)`
- Check for `gorm.ErrRecordNotFound` when appropriate

#### Logging
- Use structured logging via `ctx.Logger()`
- Use zap fields like `zap.Error(err)`, `zap.String("key", value)`

## Project Structure

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

### Key Directories

- **`build/`**: Build utilities
- **`cmd/`**: CLI entry points - main application and embedded variants
- **`config/`**: Configuration schemas and validation for all subsystems
- **`core/`**: Framework interfaces - Component, Context, Service, Protocol, Event
- **`db/`**: Database layer - Models, Providers (MySQL, SQLite, Redis), Migrations
- **`event/`**: Event handlers for boot sequence and application events
- **`service/`**: Business logic implementations - Auth, User, Storage, Upload, etc.
- **`internal/`**: Shared utilities (DNS, email, reflection)

## Contributing

Contributions are welcome! Please follow these guidelines:

1. **Code Style**: Follow Go best practices and use meaningful variable/function names
2. **Testing**: All new functionality should include appropriate tests
3. **Documentation**: Update documentation for new features or changes
4. **Commits**: Use conventional commit messages

For development standards and internal guidelines, see [`AGENTS.md`](AGENTS.md).

## License

Portal is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.

---

**Module Path**: `go.lumeweb.com/portal`
