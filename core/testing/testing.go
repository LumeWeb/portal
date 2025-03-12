// Package testing provides utilities for testing core components
package testing

import (
	"context"
	"github.com/gookit/event"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.uber.org/zap/zaptest"
	"gorm.io/gorm"
	"testing"
	"time"
)

// ResetAllState resets all global state in the core package
func ResetAllState() {
	core.ResetState()
}

// TestContextOption configures a test context
type TestContextOption func(*testContext)

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
}

// testContext extends the default context for testing
type testContext struct {
	*defaultContext
	t *testing.T
}

// NewTestContext creates a new Context suitable for testing
func NewTestContext(t *testing.T, opts ...TestContextOption) core.Context {
	t.Helper()

	// Create a mock config manager
	mockConfig := NewMockConfigManager()

	// Create a test logger with the mock config
	zapLogger := zaptest.NewLogger(t)
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
		t: t,
	}

	// Apply options
	for _, opt := range opts {
		opt(testCtx)
	}

	return testCtx
}

// WithMockService adds a mock service to the test context
func WithMockService(id string, service core.Service) TestContextOption {
	return func(ctx *testContext) {
		ctx.services[id] = service
	}
}

// WithMockDB adds a mock database to the test context
func WithMockDB(db *gorm.DB) TestContextOption {
	return func(ctx *testContext) {
		ctx.db = db
	}
}

// WithConfigValue sets a configuration value
func WithConfigValue(key string, value interface{}) TestContextOption {
	return func(ctx *testContext) {
		if ctx.cfg != nil {
			// Use Update method from config.Manager interface
			_ = ctx.cfg.Update(key, value)
		}
	}
}

// T returns the testing.T instance
func (c *testContext) T() *testing.T {
	return c.t
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

func (ctx *testContext) NamedLogger(name string) *core.Logger {
	return ctx.logger
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
