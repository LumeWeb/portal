package portal

import (
	"errors"
	"fmt"
	"go.lumeweb.com/portal/build"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/event"
	pkgReflect "go.lumeweb.com/portal/internal/reflect"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"os"
	"reflect"
	"sync"
)

var (
	activePortal      Portal
	serviceConfigType = pkgReflect.GetInterfaceType((*config.ServiceConfig)(nil))
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

	// Stage 1: Component Registration
	// Register all services, protocols, APIs and extensions to make them available
	// for configuration and initialization later. Gather context options that
	// these components may provide.
	var ctxOpts []core.ContextBuilderOption

	opts, err := p.initServices()
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

	opts, err = p.registerAPIExtensions(ctx)
	if err != nil {
		return err
	}
	ctxOpts = append(ctxOpts, opts...)

	// Create new context with all gathered options
	newCtx, err := core.NewContext(ctx.Config(), ctx.Logger(), ctxOpts...)
	if err != nil {
		ctx.Logger().Error("Error creating context", zap.Error(err))
		return err
	}
	p.SetContext(newCtx)

	// Stage 2: Component Configuration
	// Configure all registered components with their respective configs
	if err = p.configureServices(ctx); err != nil {
		return fmt.Errorf("failed to configure services: %w", err)
	}

	if err = p.configureProtocols(ctx); err != nil {
		return fmt.Errorf("failed to configure protocols: %w", err)
	}

	if err = p.configureAPIs(ctx); err != nil {
		return fmt.Errorf("failed to configure APIs: %w", err)
	}

	// Initialize config system and apply log level settings
	err = ctx.Config().Init()
	if err != nil {
		ctx.Logger().Fatal("Failed to initialize config", zap.Error(err))
		return err
	}

	ctx.Logger().SetLevelFromConfig()

	// Stage 3: Database & Models Setup
	// Initialize database connection and register all data models
	ctxOpts = make([]core.ContextBuilderOption, 0)
	dbInst, dbOpts := db.NewDatabase(ctx)
	ctxOpts = append(dbOpts, ctxOpts...)

	opts, err = p.initModels(ctx, dbInst)
	if err != nil {
		return err
	}
	ctxOpts = append(opts, ctxOpts...)

	// Stage 4: Component Initialization
	// Perform final initialization of protocols, APIs and other components
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

	// Finalize context with all gathered options
	newCtx, err = core.ProcessCtxOptions(newCtx, ctxOpts...)
	if err != nil {
		ctx.Logger().Error("Error creating context", zap.Error(err))
		return err
	}
	p.SetContext(newCtx)

	return nil
}

func (p *PortalImpl) Start() error {
	ctx := p.Context()
	ctx.Logger().Info("Starting portal")

	if err := p.startStartupFuncs(ctx); err != nil {
		return err
	}

	if err := p.registerPluginWorkflows(ctx); err != nil {
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

func (p *PortalImpl) registerPluginWorkflows(ctx core.Context) error {
	workflowSvc := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)

	for _, plugin := range core.GetPlugins() {
		if plugin.Workflows != nil {
			workflows, err := plugin.Workflows(ctx)
			if err != nil {
				ctx.Logger().Error("Error getting workflows from plugin", zap.String("plugin", plugin.ID), zap.Error(err))
				return err
			}
			for _, workflow := range workflows {
				if err = workflowSvc.RegisterWorkflow(workflow.Name, workflow.Steps, workflow.AutoTriggerFirstStep); err != nil {
					return fmt.Errorf("failed to register workflow %s for plugin %s: %w", workflow.Name, plugin.ID, err)
				}
			}
		}
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
	ctx.Logger().Info("Serving portal", zap.Stringer("version", build.GetInfo()))
	for _, plugin := range core.GetPlugins() {
		version := "unknown"
		if plugin.Version != nil {
			version = plugin.Version.Info().String()
		}
		ctx.Logger().Info("Loaded plugin", zap.String("plugin", plugin.ID), zap.String("version", version))
	}

	httpSvc := ctx.Service(core.HTTP_SERVICE)

	if httpSvc == nil {
		ctx.Logger().Error("HTTP service not found")
		return errors.New("http service not found")
	}

	return httpSvc.(core.HTTPService).Serve()
}

func (p *PortalImpl) configureServices(ctx core.Context) error {
	svcs := core.GetServices()

	for _, svcInfo := range svcs {
		if !core.IsCoreService(svcInfo.ID) {
			if configurableSvc, ok := ctx.Service(svcInfo.ID).(core.Configurable); ok {
				cfgResult, err := configurableSvc.Config()
				if err != nil {
					return err
				}
				if cfgResult == nil {
					ctx.Logger().Error("Configurable service returned nil config", zap.String("service", svcInfo.ID))
					return config.ErrInvalidServiceConfig
				}

				compliantCfg, isCompliant := pkgReflect.EnsureCompliantType(cfgResult, serviceConfigType)
				if !isCompliant {
					ctx.Logger().Error(config.ErrInvalidServiceConfig.Error()+" (type does not implement interface)", zap.String("service", svcInfo.ID), zap.Any("config_type", reflect.TypeOf(cfgResult)))
					return config.ErrInvalidServiceConfig
				}

				if reflect.ValueOf(cfgResult).Kind() != reflect.Pointer && reflect.ValueOf(compliantCfg).Kind() == reflect.Pointer && !reflect.ValueOf(cfgResult).CanAddr() {
					ctx.Logger().Warn("Config value was not addressable; using pointer to a copy for configuration.", zap.String("service", svcInfo.ID))
				}

				svcConfig, ok := compliantCfg.(config.ServiceConfig)
				if !ok {
					ctx.Logger().Error("Internal error: compliant config object could not be cast to ServiceConfig", zap.String("service", svcInfo.ID), zap.Any("config_type", reflect.TypeOf(compliantCfg)))
					return config.ErrInvalidServiceConfig
				}

				plugin := core.GetPluginForService(svcInfo.ID)
				if plugin == "" {
					return config.ErrInvalidServiceConfig
				}

				if err = ctx.Config().ConfigureService(plugin, svcInfo.ID, svcConfig); err != nil {
					return fmt.Errorf("failed to configure service %q for plugin %q: %w", svcInfo.ID, plugin, err)
				}
			}
		}
	}

	return nil
}

func (p *PortalImpl) initServices() (ctxOpts []core.ContextBuilderOption, err error) {
	svcs := core.GetServices()

	for _, svcInfo := range svcs {
		svc, opts, err := svcInfo.Factory()
		if err != nil {
			return nil, err
		}
		if opts != nil {
			ctxOpts = append(ctxOpts, opts...)
		}
		ctxOpts = append(ctxOpts, core.ContextWithService(svcInfo.ID, svc))
	}

	return ctxOpts, nil
}

func (p *PortalImpl) configureProtocols(ctx core.Context) error {
	for name, proto := range core.GetProtocols() {
		err := ctx.Config().ConfigureProtocol(name, proto.Config())
		if err != nil {
			ctx.Logger().Error("Error configuring protocol", zap.String("protocol", proto.Name()), zap.Error(err))
			return fmt.Errorf("failed to configure protocol %q: %w", proto.Name(), err)
		}
	}
	return nil
}

func (p *PortalImpl) registerProtocols(ctx core.Context) (ctxOpts []core.ContextBuilderOption, err error) {
	plugins := core.GetPlugins()

	for _, plugin := range plugins {
		if core.PluginHasProtocol(plugin) {
			proto, opts, err := plugin.Protocol()
			if err != nil {
				ctx.Logger().Error("Error building protocol", zap.String("plugin", plugin.ID), zap.Error(err))
				return nil, err
			}

			if proto == nil {
				continue
			}

			ctxOpts = append(ctxOpts, opts...)
			core.RegisterProtocol(plugin.ID, proto)
		}
	}

	return ctxOpts, nil
}

func (p *PortalImpl) configureAPIs(ctx core.Context) error {
	for name, api := range core.GetAPIs() {
		err := ctx.Config().ConfigureAPI(name, api.Config())
		if err != nil {
			ctx.Logger().Error("Error configuring api", zap.String("api", api.Name()), zap.Error(err))
			return fmt.Errorf("failed to configure API %q: %w", api.Name(), err)
		}
	}
	return nil
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

func (p *PortalImpl) registerAPIExtensions(ctx core.Context) (ctxOpts []core.ContextBuilderOption, err error) {
	plugins := core.GetPlugins()

	for _, plugin := range plugins {
		if core.PluginHasAPIExtensions(plugin) {
			extensions, err := plugin.APIExtensions(ctx)
			if err != nil {
				ctx.Logger().Error("Error building API extensions", zap.String("plugin", plugin.ID), zap.Error(err))
				return nil, err
			}

			for _, extFactory := range extensions {
				factory := extFactory
				apiExtStartup := core.ContextBuilderOption(func(ctx core.Context) (core.Context, error) {
					ext, ctxOptions, err := factory()
					if err != nil {
						ctx.Logger().Error("Error building API extensions", zap.String("plugin", plugin.ID), zap.Error(err))
						return nil, err
					}
					ctx.Logger().Info("Registering API extension",
						zap.String("plugin", plugin.ID),
						zap.String("target", ext.TargetAPI()))
					core.RegisterAPIExtension(ext)

					return core.ProcessCtxOptions(ctx, ctxOptions...)
				})
				ctxOpts = append(ctxOpts, apiExtStartup)
			}
		}
	}

	return ctxOpts, nil
}

func (p *PortalImpl) initModels(_ core.Context, dbInst *gorm.DB) (ctxOpts []core.ContextBuilderOption, err error) {
	ctxOpts = append(ctxOpts, core.ContextWithStartupFunc(func(ctx core.Context) error {
		migrationManager, err := NewMigrationManager(ctx)
		if err != nil {
			return fmt.Errorf("failed to create migration manager: %w", err)
		}

		if err = migrationManager.RunMigrations(dbInst); err != nil {
			return fmt.Errorf("failed to run migrations: %w", err)
		}

		return nil
	}))

	return ctxOpts, nil
}

func (p *PortalImpl) initProtocols(ctx core.Context) (ctxOpts []core.ContextBuilderOption, err error) {
	for _, proto := range core.GetProtocols() {
		if initProto, ok := proto.(core.ProtocolInit); ok {
			if err := initProto.Init(ctx); err != nil {
				ctx.Logger().Error("Error initializing protocol", zap.String("protocol", proto.Name()), zap.Error(err))
				return nil, err
			}
		}

		workflowSvc := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)

		// Register workflows for each protocol
		for _, workflow := range proto.Workflows() {
			if err := workflowSvc.RegisterWorkflow(workflow.Name, workflow.Steps, workflow.AutoTriggerFirstStep); err != nil {
				return nil, fmt.Errorf("failed to register workflow %s for protocol %s: %w", workflow.Name, proto.Name(), err)
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

func (p *PortalImpl) initCron() (ctxOpts []core.ContextBuilderOption) {
	/*	for _, plugin := range core.GetPlugins() {
		if core.PluginHasCron(plugin) {
			cronFactory := plugin.Cron()
			if cronFactory == nil {
				continue
			}

			ctxOpts = append(ctxOpts, core.ContextWithCron(cronFactory))
		}
	}*/

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
	return ctx.Fire(event.EVENT_BOOT_COMPLETE, event.NewBootCompleteEvent(ctx))
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
