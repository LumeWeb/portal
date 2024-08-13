package portal

import (
	"errors"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/event"
	"go.uber.org/zap"
	"gorm.io/gorm"
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

	ctx.Logger().Info("Initializing portal")

	dbInst, ctxOpts := db.NewDatabase(ctx)

	opts, err := p.initServices(ctx)
	if err != nil {
		return err
	}
	ctxOpts = append(ctxOpts, opts...)

	opts, err = p.registerProtocols(ctx)
	if err != nil {
		return err
	}
	ctxOpts = append(ctxOpts, opts...)

	opts, err = p.registerAPIs(ctx)
	if err != nil {
		return err
	}
	ctxOpts = append(ctxOpts, opts...)

	opts, err = p.initModels(ctx, dbInst)
	if err != nil {
		return err
	}
	ctxOpts = append(ctxOpts, opts...)

	opts, err = p.initProtocols(ctx)
	if err != nil {
		return err
	}
	ctxOpts = append(ctxOpts, opts...)

	opts, err = p.initAPIs(ctx)
	if err != nil {
		return err
	}
	ctxOpts = append(ctxOpts, opts...)

	opts = p.initCron()
	ctxOpts = append(ctxOpts, opts...)

	if err := p.initEvents(); err != nil {
		return err
	}

	ctxOpts = append(ctxOpts, core.ContextWithEvents(core.GetEvents()...))
	ctx, err = core.NewContext(ctx.Config(), ctx.Logger(), ctxOpts...)

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

	if err := p.startStartupFuncs(ctx); err != nil {
		return err
	}

	if err := p.startProtocols(ctx); err != nil {
		return err
	}

	if err := p.startCron(ctx); err != nil {
		return err
	}

	if err := p.startHTTP(ctx); err != nil {
		return err
	}

	if err := p.startMailer(ctx); err != nil {
		return err
	}

	if err := p.fireBootCompleteEvent(ctx); err != nil {
		ctx.Logger().Error("Error firing boot complete event", zap.Error(err))
		return err
	}

	return nil
}

func (p *PortalImpl) Stop() error {
	ctx := p.Context()
	ctx.Logger().Info("Stopping portal")

	if err := p.stopProtocols(ctx); err != nil {
		return err
	}

	if err := p.runExitFuncs(ctx); err != nil {
		return err
	}

	return nil
}

func (p *PortalImpl) Serve() error {
	ctx := p.Context()
	ctx.Logger().Info("Serving portal")

	httpSvc := ctx.Service(core.HTTP_SERVICE)

	if httpSvc == nil {
		ctx.Logger().Error("HTTP service not found")
		return errors.New("http service not found")
	}

	return httpSvc.(core.HTTPService).Serve()
}

func (p *PortalImpl) initServices(ctx core.Context) (ctxOpts []core.ContextBuilderOption, err error) {
	svcs := core.GetServices()

	for _, svcInfo := range svcs {
		svc, opts, err := svcInfo.Factory()
		if err != nil {
			ctx.Logger().Error("Error creating service", zap.String("service", svcInfo.ID), zap.Error(err))
			return nil, err
		}

		if opts != nil {
			ctxOpts = append(ctxOpts, opts...)
		}

		ctxOpts = append(ctxOpts, core.ContextWithService(svcInfo.ID, svc))

		if !core.IsCoreService(svcInfo.ID) {
			if configurableSvc, ok := svc.(core.Configurable); ok {
				cfg, err := configurableSvc.Config()
				if err != nil {
					ctx.Logger().Error("Error getting service config", zap.String("service", svcInfo.ID), zap.Error(err))
					return nil, err
				}

				svcConfig, ok := cfg.(config.ServiceConfig)
				if !ok {
					ctx.Logger().Error(config.ErrInvalidServiceConfig.Error(), zap.String("service", svcInfo.ID))
					return nil, config.ErrInvalidServiceConfig
				}
				plugin := core.GetPluginForService(svcInfo.ID)
				if plugin == "" {
					ctx.Logger().Error("Error getting plugin for service", zap.String("service", svcInfo.ID))
					continue
				}
				if err := ctx.Config().ConfigureService(plugin, svcInfo.ID, svcConfig); err != nil {
					ctx.Logger().Error("Error configuring service", zap.String("service", svcInfo.ID), zap.Error(err))
					return nil, err
				}
			}
		}
	}

	return ctxOpts, nil
}

func (p *PortalImpl) registerProtocols(ctx core.Context) (ctxOpts []core.ContextBuilderOption, err error) {
	plugins := core.GetPlugins()

	for _, plugin := range plugins {
		if core.PluginHasProtocol(plugin) {
			_proto, opts, err := plugin.Protocol()
			if err != nil {
				ctx.Logger().Error("Error building protocol", zap.String("plugin", plugin.ID), zap.Error(err))
				return nil, err
			}

			if _proto == nil {
				continue
			}

			ctxOpts = append(ctxOpts, opts...)

			core.RegisterProtocol(plugin.ID, _proto)
		}
	}

	return ctxOpts, nil
}

func (p *PortalImpl) registerAPIs(ctx core.Context) (ctxOpts []core.ContextBuilderOption, err error) {
	plugins := core.GetPlugins()

	for _, plugin := range plugins {
		if core.PluginHasAPI(plugin) {
			api, opts, err := plugin.API()
			if err != nil {
				ctx.Logger().Error("Error building API", zap.String("plugin", plugin.ID), zap.Error(err))
				return nil, err
			}

			if api == nil {
				continue
			}

			ctxOpts = append(ctxOpts, opts...)
			core.RegisterAPI(plugin.ID, api)
		}
	}

	return ctxOpts, nil
}

func (p *PortalImpl) initModels(ctx core.Context, dbInst *gorm.DB) (ctxOpts []core.ContextBuilderOption, err error) {
	plugins := core.GetPlugins()

	models := make([]interface{}, 0)
	for _, plugin := range plugins {
		if plugin.Models != nil && len(plugin.Models) > 0 {
			typ := reflect.TypeOf(plugin.Models)
			if typ.Kind() != reflect.Ptr {
				ctx.Logger().Error("Model must be a pointer", zap.String("model", typ.Name()))
				return nil, core.ErrInvalidModel
			}
			models = append(models, plugin.Models...)
		}
	}

	migrations := make([]core.DBMigration, 0)
	for _, plugin := range plugins {
		if plugin.Migrations != nil && len(plugin.Migrations) > 0 {
			migrations = append(migrations, plugin.Migrations...)
		}
	}

	ctxOpts = append(ctxOpts, core.ContextWithStartupFunc(func(ctx core.Context) error {
		for _, model := range models {
			typ := reflect.TypeOf(model)
			if err = dbInst.AutoMigrate(model); err != nil {
				ctx.Logger().Error("Error migrating model", zap.String("model", typ.Name()), zap.Error(err))
				return err
			}
		}

		for _, migration := range migrations {
			if err = migration(p.ctx.DB()); err != nil {
				ctx.Logger().Error("Error running migration", zap.Error(err))
				return err
			}
		}

		return nil
	}))

	return ctxOpts, nil
}

func (p *PortalImpl) initProtocols(ctx core.Context) (ctxOpts []core.ContextBuilderOption, err error) {
	for name, _proto := range core.GetProtocols() {
		err := ctx.Config().ConfigureProtocol(name, _proto.Config())
		if err != nil {
			ctx.Logger().Error("Error configuring protocol", zap.String("protocol", _proto.Name()), zap.Error(err))
			return nil, err
		}

		if initProto, ok := _proto.(core.ProtocolInit); ok {
			if err := initProto.Init(ctx); err != nil {
				ctx.Logger().Error("Error initializing protocol", zap.String("protocol", _proto.Name()), zap.Error(err))
				return nil, err
			}
		}
	}

	return ctxOpts, nil
}

func (p *PortalImpl) initAPIs(ctx core.Context) (ctxOpts []core.ContextBuilderOption, err error) {
	for name, api := range core.GetAPIs() {
		err := ctx.Config().ConfigureAPI(name, api.Config())
		if err != nil {
			ctx.Logger().Error("Error configuring api", zap.String("api", api.Name()), zap.Error(err))
			return nil, err
		}
		if initApi, ok := api.(core.APIInit); ok {
			opts, err := initApi.Init()
			if err != nil {
				ctx.Logger().Error("Error initializing api", zap.String("api", api.Name()), zap.Error(err))
				return nil, err
			}

			ctxOpts = append(ctxOpts, opts...)
		}
	}

	return ctxOpts, nil
}

func (p *PortalImpl) initEvents() error {
	for _, plugin := range core.GetPlugins() {
		for _, e := range plugin.Events {
			core.RegisterEvent(e.Name(), e)
		}
	}

	return nil
}

func (p *PortalImpl) configureProtocols(ctx core.Context) error {
	for name, _proto := range core.GetProtocols() {
		err := ctx.Config().ConfigureProtocol(name, _proto.Config())
		if err != nil {
			ctx.Logger().Error("Error configuring protocol", zap.String("protocol", _proto.Name()), zap.Error(err))
			return err
		}
	}

	return nil
}

func (p *PortalImpl) configureAPIs(ctx core.Context) error {
	for name, api := range core.GetAPIs() {
		err := ctx.Config().ConfigureAPI(name, api.Config())
		if err != nil {
			ctx.Logger().Error("Error configuring api", zap.String("api", api.Name()), zap.Error(err))
			return err
		}
	}

	return nil
}

func (p *PortalImpl) initCron() (ctxOpts []core.ContextBuilderOption) {
	for _, plugin := range core.GetPlugins() {
		if core.PluginHasCron(plugin) {
			cronFactory := plugin.Cron()
			if cronFactory == nil {
				continue
			}

			ctxOpts = append(ctxOpts, core.ContextWithCron(cronFactory))
		}
	}

	return ctxOpts
}

func (p *PortalImpl) startStartupFuncs(ctx core.Context) error {
	for _, startupFunc := range ctx.StartupFuncs() {
		if err := startupFunc(ctx); err != nil {
			ctx.Logger().Error("Error starting portal", zap.Error(err))
			return err
		}
	}

	return nil
}

func (p *PortalImpl) startProtocols(ctx core.Context) error {
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

func (p *PortalImpl) startCron(ctx core.Context) error {
	cronSvc := ctx.Service(core.CRON_SERVICE)

	if cronSvc == nil {
		ctx.Logger().Error("Cron service not found")
		return errors.New("cron service not found")
	}

	err := cronSvc.(core.CronService).Start()
	if err != nil {
		return err
	}

	return nil
}

func (p *PortalImpl) startHTTP(ctx core.Context) error {
	httpSvc := ctx.Service(core.HTTP_SERVICE)

	if httpSvc == nil {
		ctx.Logger().Error("HTTP service not found")
		return errors.New("http service not found")
	}

	err := httpSvc.(core.HTTPService).Init()

	if err != nil {
		return err
	}

	return nil
}

func (p *PortalImpl) startMailer(ctx core.Context) error {
	mailerSvc := ctx.Service(core.MAILER_SERVICE)

	if mailerSvc == nil {
		ctx.Logger().Error("Mailer service not found")
		return errors.New("mailer service not found")
	}

	plugins := core.GetPlugins()

	for _, plugin := range plugins {
		if plugin.MailerTemplates != nil {
			for name, tpl := range plugin.MailerTemplates {
				if err := mailerSvc.(core.MailerService).TemplateRegister(name, tpl); err != nil {
					ctx.Logger().Error("Error registering mailer template", zap.String("template", name), zap.Error(err))
					return err
				}
			}
		}
	}

	return nil
}

func (p *PortalImpl) stopProtocols(ctx core.Context) error {
	for _, proto := range core.GetProtocols() {
		if stopPlugin, ok := proto.(core.ProtocolStop); ok {
			if err := stopPlugin.Stop(ctx); err != nil {
				ctx.Logger().Error("Error stopping protocol", zap.String("protocol", proto.Name()), zap.Error(err))
				return err
			}
		}
	}

	return nil
}

func (p *PortalImpl) runExitFuncs(ctx core.Context) error {
	for _, exitFunc := range ctx.ExitFuncs() {
		if err := exitFunc(ctx); err != nil {
			ctx.Logger().Error("Error stopping portal", zap.Error(err))
		}
	}

	return nil
}

func (p *PortalImpl) fireBootCompleteEvent(ctx core.Context) error {
	return event.FireBootCompleteEvent(ctx)
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
