package portal

import (
	"github.com/samber/lo"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.uber.org/zap"
	"os"
	"reflect"
	"sync"
)

var (
	activePortal Portal
)

type Portal interface {
	Init() error
	Start() error
	Stop() error
	Context() core.Context
	Serve() error
}

type PortalImpl struct {
	ctx   core.Context
	ctxMu sync.RWMutex
}

func (p *PortalImpl) Init() error {
	ctx := p.Context()

	dbInst, ctxOpts := db.NewDatabase(ctx)

	svcs := core.GetServices()

	for _, svcInfo := range svcs {
		svc, opts, err := svcInfo.Factory()
		if err != nil {
			ctx.Logger().Error("Error creating service", zap.String("service", svcInfo.ID), zap.Error(err))
			return err
		}

		if opts != nil {
			opts = append(ctxOpts, opts...)
		}

		ctxOpts = append(ctxOpts, core.ContextWithService(svcInfo.ID, svc))
	}

	plugins := core.GetPlugins()

	for _, plugin := range plugins {
		if plugin.Models != nil && len(plugin.Models) > 0 {
			for _, model := range plugin.Models {
				typ := reflect.TypeOf(model)
				if typ.Kind() != reflect.Ptr {
					ctx.Logger().Error("Model must be a pointer", zap.String("model", typ.Name()))
					return core.ErrInvalidModel
				}

				ctxOpts = append(ctxOpts, core.ContextWithStartupFunc(func(ctx core.Context) error {
					if err := dbInst.AutoMigrate(model); err != nil {
						ctx.Logger().Error("Error migrating model", zap.String("model", typ.Name()), zap.Error(err))
						return err
					}

					return nil
				}))
			}
		}
	}

	for _, plugin := range plugins {
		if core.PluginHasProtocol(plugin) {
			if !lo.Contains(ctx.Config().Config().Core.Protocols, plugin.ID) {
				continue
			}
			_proto, opts, err := plugin.Protocol()
			if err != nil {
				ctx.Logger().Error("Error building protocol", zap.String("plugin", plugin.ID), zap.Error(err))
				return err
			}

			if _proto == nil {
				continue
			}

			ctxOpts = append(ctxOpts, opts...)

			core.RegisterProtocol(plugin.ID, _proto)
		}
	}

	for _, plugin := range plugins {
		if core.PluginHasAPI(plugin) {
			api, opts, err := plugin.API()
			if err != nil {
				ctx.Logger().Error("Error building API", zap.String("plugin", plugin.ID), zap.Error(err))
				return err
			}

			if api == nil {
				continue
			}

			ctxOpts = append(ctxOpts, opts...)

			core.RegisterAPI(plugin.ID, api)
		}
	}

	for _, _proto := range core.GetProtocols() {
		err := ctx.Config().ConfigureProtocol(_proto.Name(), _proto.Config())
		if err != nil {
			ctx.Logger().Error("Error configuring protocol", zap.String("protocol", _proto.Name()), zap.Error(err))
			return err
		}

		if initProto, ok := _proto.(core.ProtocolInit); ok {
			if err := initProto.Init(&ctx); err != nil {
				ctx.Logger().Error("Error initializing protocol", zap.String("protocol", _proto.Name()), zap.Error(err))
				return err
			}
		}
	}

	for _, api := range core.GetAPIs() {
		if initApi, ok := api.(core.APIInit); ok {
			opts, err := initApi.Init()
			if err != nil {
				ctx.Logger().Error("Error initializing api", zap.String("api", api.Subdomain()), zap.Error(err))
				return err
			}

			ctxOpts = append(ctxOpts, opts...)
		}
	}

	ctx, err := core.NewContext(ctx.Config(), ctx.Logger(), ctxOpts...)

	if err != nil {
		ctx.Logger().Error("Error creating context", zap.Error(err))
		return err
	}

	p.SetContext(ctx)

	return nil
}

func (p *PortalImpl) Start() error {
	ctx := p.Context()
	ctx.Logger().Info("Starting portal")

	for _, startupFunc := range ctx.StartupFuncs() {
		if err := startupFunc(ctx); err != nil {
			ctx.Logger().Error("Error starting portal", zap.Error(err))
			return err
		}
	}

	for _, proto := range core.GetProtocols() {
		if startPlugin, ok := proto.(core.ProtocolStart); ok {
			if err := startPlugin.Start(ctx); err != nil {
				ctx.Logger().Error("Error starting protocol", zap.String("protocol", proto.Name()), zap.Error(err))
				return err
			}
		}
	}

	return nil
}

func (p *PortalImpl) Stop() error {
	ctx := p.Context()
	ctx.Logger().Info("Stopping portal")
	for _, proto := range core.GetProtocols() {
		if stopPlugin, ok := proto.(core.ProtocolStop); ok {
			if err := stopPlugin.Stop(ctx); err != nil {
				ctx.Logger().Error("Error stopping protocol", zap.String("protocol", proto.Name()), zap.Error(err))
				return err
			}
		}
	}

	for _, exitFunc := range ctx.ExitFuncs() {
		if err := exitFunc(ctx); err != nil {
			ctx.Logger().Error("Error stopping portal", zap.Error(err))
		}
	}

	return nil
}

func (p *PortalImpl) Serve() error {
	ctx := p.Context()
	ctx.Logger().Info("Serving portal")
	return ctx.Service(core.HTTP_SERVICE).(core.HTTPService).Serve()
}

func NewPortal(ctx core.Context) *PortalImpl {
	return &PortalImpl{
		ctx: ctx,
	}
}

func (p *PortalImpl) Context() core.Context {
	p.ctxMu.RLock()
	defer p.ctxMu.RUnlock()
	return p.ctx
}

func (p *PortalImpl) SetContext(ctx core.Context) {
	p.ctxMu.Lock()
	defer p.ctxMu.Unlock()
	p.ctx = ctx
}

func NewActivePortal(ctx core.Context) {
	activePortal = NewPortal(ctx)
}

func Start() error {
	return activePortal.Start()
}

func Init() error {
	return activePortal.Init()
}

func Stop() error {
	return activePortal.Stop()
}

func Serve() error {
	return activePortal.Serve()
}

func Context() core.Context {
	return activePortal.Context()
}

func ActivePortal() Portal {
	return activePortal
}

func Shutdown(activePortal Portal, logger *zap.Logger) {
	ctx := activePortal.Context()

	if logger == nil {
		logger = ctx.Logger().Logger
	}

	// Cancel the context
	ctx.Cancel()

	// Wait for the context to be canceled
	<-ctx.Done()

	// Stop the portal
	if err := activePortal.Stop(); err != nil {
		logger.Error("Failed to stop portal", zap.Error(err))
		ctx.SetExitCode(core.ExitCodeFailedQuit)
	}

	// Exit the process
	os.Exit(ctx.ExitCode())
}
