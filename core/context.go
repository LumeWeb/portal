package core

import (
	"context"
	"fmt"
	"github.com/gookit/event"
	"go.lumeweb.com/portal/config"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var _ Context = (*defaultContext)(nil)

type LifecycleFunc func(Context) error

// Context interface
type Context interface {
	context.Context
	Service(id string) any
	OnExit(f LifecycleFunc)
	OnStartup(f LifecycleFunc)
	StartupFuncs() []func(Context) error
	ExitFuncs() []func(Context) error
	DB() *gorm.DB
	Logger() *Logger
	ProtocolLogger(protocol Protocol) *Logger
	APILogger(api API) *Logger
	ServiceLogger(service Service) *Logger
	NamedLogger(name string) *Logger
	Config() config.Manager
	Cancel()
	ExitCode() int
	Event() *event.Manager
	SetExitCode(code int)
	GetContext() context.Context
}

// defaultContext struct implementing the Context interface
type defaultContext struct {
	context.Context
	services     map[string]any
	cfg          config.Manager
	logger       *Logger
	exitFuncs    []func(Context) error
	exitCode     int
	startupFuncs []func(Context) error
	db           *gorm.DB
	cancel       context.CancelFunc
	event        *event.Manager
}

// NewContext creates a new Context
func NewContext(config config.Manager, logger *Logger, options ...ContextBuilderOption) (Context, error) {
	// Create a new context with cancel
	baseCtx, cancel := context.WithCancel(context.Background())

	newCtx := &defaultContext{
		Context:  baseCtx,
		services: make(map[string]any),
		cfg:      config,
		logger:   logger,
		event:    event.NewManager(""),
		cancel:   cancel,
	}

	options = append(options, ContextWithExitFunc(func(ctx Context) error {
		return ctx.Event().CloseWait()
	}))

	var err error
	currentCtx := Context(newCtx)

	for _, opt := range options {
		currentCtx, err = opt(currentCtx)
		if err != nil {
			return currentCtx, err
		}
		// Type assert back to *defaultContext if needed
		if dc, ok := currentCtx.(*defaultContext); ok {
			newCtx = dc
		} else {
			return currentCtx, fmt.Errorf("context type changed unexpectedly")
		}
	}

	return newCtx, nil
}

// Implement the Context interface methods for defaultContext

func (ctx *defaultContext) Service(id string) any {
	if svc, ok := ctx.services[id]; ok {
		return svc
	}
	return nil
}

func (ctx *defaultContext) OnExit(f LifecycleFunc) {
	ctx.exitFuncs = append(ctx.exitFuncs, f)
}

func (ctx *defaultContext) OnStartup(f LifecycleFunc) {
	ctx.startupFuncs = append(ctx.startupFuncs, f)
}

func (ctx *defaultContext) StartupFuncs() []func(Context) error {
	return ctx.startupFuncs
}

func (ctx *defaultContext) ExitFuncs() []func(Context) error {
	return ctx.exitFuncs
}

func (ctx *defaultContext) DB() *gorm.DB {
	return ctx.db.WithContext(ctx)
}

func (ctx *defaultContext) Logger() *Logger {
	return ctx.logger
}

func (ctx *defaultContext) Config() config.Manager {
	return ctx.cfg
}

func (ctx *defaultContext) Cancel() {
	ctx.cancel()
}

func (ctx *defaultContext) ExitCode() int {
	return ctx.exitCode
}

func (ctx *defaultContext) Event() *event.Manager {
	return ctx.event
}

func (ctx *defaultContext) SetExitCode(code int) {
	ctx.exitCode = code
}

func (ctx *defaultContext) Value(key any) any {
	return ctx.Context.Value(key)
}

func (ctx *defaultContext) GetContext() context.Context {
	return ctx.Context
}

func (ctx *defaultContext) ProtocolLogger(protocol Protocol) *Logger {
	return ctx.NamedLogger(fmt.Sprintf("protocol-%s", protocol.Name()))
}

func (ctx *defaultContext) APILogger(api API) *Logger {
	return ctx.NamedLogger(fmt.Sprintf("api-%s", api.Name()))
}

func (ctx *defaultContext) ServiceLogger(service Service) *Logger {
	return ctx.NamedLogger(fmt.Sprintf("service-%s", service.ID()))
}

func (ctx *defaultContext) NamedLogger(name string) *Logger {
	return &Logger{
		Logger: ctx.logger.Logger.Named(name),
		level:  ctx.logger.level,
		cm:     ctx.logger.cm,
	}
}

// ContextBuilderOption and related functions

type ContextBuilderOption func(Context) (Context, error)

func ContextWithService(id string, svc Service) ContextBuilderOption {
	return func(ctx Context) (Context, error) {
		if defaultCtx, ok := ctx.(*defaultContext); ok {
			defaultCtx.services[id] = svc
		}
		return ctx, nil
	}
}

func ContextWithStartupFunc(f LifecycleFunc) ContextBuilderOption {
	return func(ctx Context) (Context, error) {
		ctx.OnStartup(f)
		return ctx, nil
	}
}

func ContextWithExitFunc(f LifecycleFunc) ContextBuilderOption {
	return func(ctx Context) (Context, error) {
		ctx.OnExit(f)
		return ctx, nil
	}
}

func ContextWithEvents(events ...Eventer) ContextBuilderOption {
	return func(ctx Context) (Context, error) {
		for _, e := range events {
			ctx.Event().AddEvent(e)
		}
		return ctx, nil
	}
}

func ContextWithDB(db *gorm.DB) ContextBuilderOption {
	return func(ctx Context) (Context, error) {
		if defaultCtx, ok := ctx.(*defaultContext); ok {
			defaultCtx.db = db
		}
		return ctx, nil
	}
}

func ContextWithCron(factory CronFactory) ContextBuilderOption {
	return func(ctx Context) (Context, error) {
		cron, err := factory(ctx)
		if err != nil {
			return ctx, err
		}
		ctx.OnStartup(func(ctx Context) error {
			cronService := ctx.Service(CRON_SERVICE)
			if cronService == nil {
				return fmt.Errorf("cron service not found")
			}

			cronService.(CronService).RegisterEntity(cron)
			return nil
		})
		return ctx, nil
	}
}

func ContextOptions(options ...ContextBuilderOption) []ContextBuilderOption {
	return options
}

func GetService[T Service](ctx Context, id string) T {
	svc := ctx.Service(id)
	if svc == nil {
		ctx.Logger().Fatal("service not found", zap.String("service", id))
	}

	typedSvc, ok := svc.(T)

	if !ok {
		ctx.Logger().Fatal("service type mismatch", zap.String("service", id))
	}

	return typedSvc
}

func ServiceExists(ctx Context, id string) bool {
	if ctx.Service(id) == nil {
		return false
	}
	return true
}
