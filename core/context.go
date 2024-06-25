package core

import (
	"context"
	"fmt"
	"github.com/gookit/event"
	"go.lumeweb.com/portal/config"
	"gorm.io/gorm"
)

type ContextBuilderFunc func() error
type ContextBuilderOption func(Context) (Context, error)

type StartupFunc func(Context) error
type ExitFunc func(Context) error

type Context struct {
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

func NewContext(config config.Manager, logger *Logger, options ...ContextBuilderOption) (Context, error) {
	newCtx := Context{
		Context:  context.Background(),
		services: make(map[string]any),
		cfg:      config,
		logger:   logger,
		event:    event.NewManager(""),
	}
	c, cancel := context.WithCancel(newCtx)

	newCtx.Context = c
	newCtx.cancel = cancel

	options = append(options, ContextWithExitFunc(func(ctx Context) error {
		return ctx.event.CloseWait()
	}))

	var err error

	for _, opt := range options {
		newCtx, err = opt(newCtx)
		if err != nil {
			return newCtx, err
		}
	}

	return newCtx, nil
}

func (ctx *Context) Service(id string) any {
	if svc, ok := ctx.services[id]; ok {
		return svc
	}

	return nil
}

func (ctx *Context) OnExit(f func(Context) error) {
	ctx.exitFuncs = append(ctx.exitFuncs, f)
}

func (ctx *Context) OnStartup(f func(Context) error) {
	ctx.startupFuncs = append(ctx.startupFuncs, f)
}

func (ctx *Context) StartupFuncs() []func(Context) error {
	return ctx.startupFuncs
}

func (ctx *Context) ExitFuncs() []func(Context) error {
	return ctx.exitFuncs
}

func (ctx *Context) DB() *gorm.DB {
	return ctx.db
}

func (ctx *Context) Logger() *Logger {
	return ctx.logger
}

func (ctx *Context) Config() config.Manager {
	return ctx.cfg
}

func (ctx *Context) Cancel() {
	ctx.cancel()
}

func (ctx *Context) ExitCode() int {
	return ctx.exitCode
}

func (ctx *Context) Event() *event.Manager {
	return ctx.event
}

func (ctx *Context) SetExitCode(code int) {
	ctx.exitCode = code
}

func ContextWithService(id string, svc Service) ContextBuilderOption {
	return func(ctx Context) (Context, error) {
		ctx.services[id] = svc
		return ctx, nil
	}
}

func ContextWithStartupFunc(f StartupFunc) ContextBuilderOption {
	return func(ctx Context) (Context, error) {
		ctx.OnStartup(f)
		return ctx, nil
	}
}

func ContextWithExitFunc(f ExitFunc) ContextBuilderOption {
	return func(ctx Context) (Context, error) {
		ctx.OnExit(f)
		return ctx, nil
	}
}

func ContextWithEvents(events ...Eventer) ContextBuilderOption {
	return func(ctx Context) (Context, error) {
		for _, e := range events {
			ctx.event.AddEvent(e)
		}
		return ctx, nil
	}
}

func ContextWithDB(db *gorm.DB) ContextBuilderOption {
	return func(ctx Context) (Context, error) {
		ctx.db = db
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
