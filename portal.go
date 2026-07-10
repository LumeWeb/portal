package portal

import (
	"errors"
	"fmt"
	"os"
	"reflect"
	"sync"

	"go.lumeweb.com/portal/build"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/event"
	pkgDNS "go.lumeweb.com/portal/internal/dns"
	pkgReflect "go.lumeweb.com/portal/internal/reflect"
	"go.uber.org/zap"
	"gorm.io/gorm"
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
	ctx    core.Context
	logger *core.Logger
	ctxMu  sync.RWMutex
}

func (p *PortalImpl) Init() error {
	ctx := p.Context()
	logger := ctx.Logger()
	logger.Info("Initializing portal")

	// Fire init start event
	if err := core.Fire(ctx, event.EVENT_INIT_START, event.NewInitStartEvent(ctx, ctx.GetContext())); err != nil {
		logger.Error("Error firing init start event", zap.Error(err))
		return err
	}

	// Stage 1: Component Registration
	// Register all services, protocols, APIs and extensions to make them available
	// for configuration and initialization later. Gather context options that
	// these components may provide. Service options are kept separate initially
	// to ensure proper ordering of context initialization.
	var ctxOpts []core.ContextBuilderOption

	// Initialize services first - their options are split into direct and service-specific
	// to allow staged initialization later
	opts, svcOpts, err := p.initServices()
	if err != nil {
		return err
	}
	ctxOpts = append(ctxOpts, opts...)

	// Register protocols, APIs and extensions - these may depend on services being available
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

	// Fire components registered event
	if err := core.Fire(ctx, event.EVENT_INIT_COMPONENTS_REGISTERED, event.NewInitComponentsRegisteredEvent(ctx, ctx.GetContext())); err != nil {
		logger.Error("Error firing init components registered event", zap.Error(err))
		return err
	}

	// Create new context with gathered options - this establishes the base context
	// with all registered components but before any configuration is applied
	ctx, err = core.NewContext(ctx.Config(), logger, ctxOpts...)
	if err != nil {
		logger.Error("Error applying context options", zap.Error(err))
		return err
	}
	p.SetContext(ctx)

	// Stage 2: Component Configuration
	// Configure all registered components with their respective configs
	if err = p.configureServices(ctx); err != nil {
		return fmt.Errorf("failed to configure services: %w", err)
	}

	// Register metrics for all services and plugins
	if err = p.registerMetrics(ctx); err != nil {
		return fmt.Errorf("failed to register metrics: %w", err)
	}

	if err = p.configureProtocols(ctx); err != nil {
		return fmt.Errorf("failed to configure protocols: %w", err)
	}

	if err = p.configureAPIs(ctx); err != nil {
		return fmt.Errorf("failed to configure APIs: %w", err)
	}

	// Fire components configured event
	if err := core.Fire(ctx, event.EVENT_INIT_COMPONENTS_CONFIGURED, event.NewInitComponentsConfiguredEvent(ctx, ctx.GetContext())); err != nil {
		logger.Error("Error firing init components configured event", zap.Error(err))
		return err
	}

	// Initialize config system and apply log level settings
	err = ctx.Config().Init()
	if err != nil {
		logger.Fatal("Failed to initialize config", zap.Error(err))
		return err
	}

	logger.SetLevelFromConfig()

	// Setup DNS override if configured
	if dnsResolver := ctx.Config().Config().Core.DNSResolverString(); dnsResolver != "" {
		pkgDNS.SetupDNSResolver(dnsResolver, logger)
	}

	// Stage 3: Database & Models Setup
	// Initialize database connection and register all data models
	// Initialize database and collect its context options
	dbInst, dbOpts := db.NewDatabase(ctx)

	// Initialize models and append their options to database options
	opts, err = p.initModels(ctx, dbInst)
	if err != nil {
		return err
	}
	dbOpts = append(dbOpts, opts...)

	// Apply database options first and update portal context
	ctx, err = core.ProcessCtxOptions(ctx, dbOpts...)
	if err != nil {
		logger.Error("Error applying database options", zap.Error(err))
		return err
	}
	p.SetContext(ctx)

	// Setup DB observability with OpenTelemetry tracing
	if err := db.SetupDBObservability(ctx); err != nil {
		logger.Error("Failed to configure DB observability", zap.Error(err))
		return err
	}

	// Fire database ready event (now with working DB)
	if err := core.Fire(ctx, event.EVENT_INIT_DATABASE_READY, event.NewInitDatabaseReadyEvent(ctx, ctx.GetContext())); err != nil {
		logger.Error("Error firing init database ready event", zap.Error(err))
		return err
	}

	// Start fresh with empty options for next stage
	ctxOpts = []core.ContextBuilderOption{}
	// Stage 4: Component Initialization
	// Perform final initialization of protocols, APIs and other components.
	// Service-specific context options are applied last to ensure all other
	// components are properly initialized first.
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

	ctxOpts = append(ctxOpts, svcOpts...)

	// Finalize context with remaining options (excluding already-applied dbOpts)
	if len(ctxOpts) > 0 {
		ctx, err = core.ProcessCtxOptions(ctx, ctxOpts...)
		if err != nil {
			logger.Error("Error creating context", zap.Error(err))
			return err
		}
		p.SetContext(ctx)
	}

	// Fire init complete event
	if err := core.Fire(ctx, event.EVENT_INIT_COMPLETE, event.NewInitCompleteEvent(ctx, ctx.GetContext())); err != nil {
		logger.Error("Error firing init complete event", zap.Error(err))
		return err
	}

	return nil
}

func (p *PortalImpl) Start() error {
	ctx := p.Context()
	ctx.Logger().Info("Starting portal")

	if err := core.Fire(ctx, event.EVENT_BOOT_START, event.NewBootStartEvent(ctx, ctx.GetContext())); err != nil {
		ctx.Logger().Error("Error firing boot start event", zap.Error(err))
		return err
	}

	if err := core.Fire(ctx, event.EVENT_BOOT_STARTUP_FUNCS, event.NewBootStartupFuncsEvent(ctx, ctx.GetContext())); err != nil {
		ctx.Logger().Error("Error firing boot startup funcs event", zap.Error(err))
		return err
	}

	if err := p.startStartupFuncs(ctx); err != nil {
		return err
	}

	if err := core.Fire(ctx, event.EVENT_BOOT_STARTUP_FUNCS_COMPLETED, event.NewBootStartupFuncsCompletedEvent(ctx, ctx.GetContext())); err != nil {
		ctx.Logger().Error("Error firing boot startup funcs completed event", zap.Error(err))
		return err
	}

	if err := core.Fire(ctx, event.EVENT_BOOT_PLUGIN_WORKFLOWS, event.NewBootPluginWorkflowsEvent(ctx, ctx.GetContext())); err != nil {
		ctx.Logger().Error("Error firing boot plugin workflows event", zap.Error(err))
		return err
	}

	if err := p.registerPluginWorkflows(ctx); err != nil {
		return err
	}

	if err := core.Fire(ctx, event.EVENT_BOOT_PLUGIN_WORKFLOWS_COMPLETED, event.NewBootPluginWorkflowsCompletedEvent(ctx, ctx.GetContext())); err != nil {
		ctx.Logger().Error("Error firing boot plugin workflows completed event", zap.Error(err))
		return err
	}

	if err := core.Fire(ctx, event.EVENT_BOOT_PROTOCOL_WORKFLOWS, event.NewBootProtocolWorkflowsEvent(ctx, ctx.GetContext())); err != nil {
		ctx.Logger().Error("Error firing boot protocol workflows event", zap.Error(err))
		return err
	}

	if err := p.registerProtocolWorkflows(ctx); err != nil {
		return err
	}

	if err := core.Fire(ctx, event.EVENT_BOOT_PROTOCOL_WORKFLOWS_COMPLETED, event.NewBootProtocolWorkflowsCompletedEvent(ctx, ctx.GetContext())); err != nil {
		ctx.Logger().Error("Error firing boot protocol workflows completed event", zap.Error(err))
		return err
	}

	if err := core.Fire(ctx, event.EVENT_BOOT_PROTOCOLS, event.NewBootProtocolsEvent(ctx, ctx.GetContext())); err != nil {
		ctx.Logger().Error("Error firing boot protocols event", zap.Error(err))
		return err
	}

	if err := p.startProtocols(ctx); err != nil {
		return err
	}

	if err := core.Fire(ctx, event.EVENT_BOOT_PROTOCOLS_COMPLETED, event.NewBootProtocolsCompletedEvent(ctx, ctx.GetContext())); err != nil {
		ctx.Logger().Error("Error firing boot protocols completed event", zap.Error(err))
		return err
	}

	if err := core.Fire(ctx, event.EVENT_BOOT_CRON, event.NewBootCronEvent(ctx, ctx.GetContext())); err != nil {
		ctx.Logger().Error("Error firing boot cron event", zap.Error(err))
		return err
	}

	if err := p.startCron(ctx); err != nil {
		return err
	}

	if err := core.Fire(ctx, event.EVENT_BOOT_CRON_COMPLETED, event.NewBootCronCompletedEvent(ctx, ctx.GetContext())); err != nil {
		ctx.Logger().Error("Error firing boot cron completed event", zap.Error(err))
		return err
	}

	if err := core.Fire(ctx, event.EVENT_BOOT_HTTP, event.NewBootHTTPEvent(ctx, ctx.GetContext())); err != nil {
		ctx.Logger().Error("Error firing boot http event", zap.Error(err))
		return err
	}

	if err := p.startHTTP(ctx); err != nil {
		return err
	}

	if err := core.Fire(ctx, event.EVENT_BOOT_HTTP_COMPLETED, event.NewBootHTTPCompletedEvent(ctx, ctx.GetContext())); err != nil {
		ctx.Logger().Error("Error firing boot http completed event", zap.Error(err))
		return err
	}

	if err := core.Fire(ctx, event.EVENT_BOOT_MAILER, event.NewBootMailerEvent(ctx, ctx.GetContext())); err != nil {
		ctx.Logger().Error("Error firing boot mailer event", zap.Error(err))
		return err
	}

	if err := p.startMailer(ctx); err != nil {
		return err
	}

	if err := core.Fire(ctx, event.EVENT_BOOT_MAILER_COMPLETED, event.NewBootMailerCompletedEvent(ctx, ctx.GetContext())); err != nil {
		ctx.Logger().Error("Error firing boot mailer completed event", zap.Error(err))
		return err
	}

	if err := p.fireBootCompletedEvent(ctx); err != nil {
		ctx.Logger().Error("Error firing boot completed event", zap.Error(err))
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

func (p *PortalImpl) registerProtocolWorkflows(ctx core.Context) error {
	workflowSvc := core.GetService[core.WorkflowService](ctx, core.WORKFLOW_SERVICE)

	for _, proto := range core.GetProtocols() {
		wfs := proto.Workflows()
		if len(wfs) == 0 {
			continue
		}
		for _, workflow := range wfs {
			ctx.Logger().Debug("Registering protocol workflow",
				zap.String("protocol", proto.Name()),
				zap.String("workflow", workflow.Name))
			if err := workflowSvc.RegisterWorkflow(workflow.Name, workflow.Steps, workflow.AutoTriggerFirstStep); err != nil {
				return fmt.Errorf("failed to register workflow %s for protocol %s: %w", workflow.Name, proto.Name(), err)
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
				cfgResult, err := configurableSvc.GetConfig()
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
					ctx.Logger().Warn("GetConfig value was not addressable; using pointer to a copy for configuration.", zap.String("service", svcInfo.ID))
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

func (p *PortalImpl) registerMetrics(ctx core.Context) error {
	// Register metrics for all core and plugin services
	for _, svcInfo := range core.GetServices() {
		if err := core.RegisterServiceMetrics(svcInfo.ID, svcInfo.Metrics); err != nil {
			ctx.Logger().Error("Failed to register service metrics",
				zap.String("service", svcInfo.ID),
				zap.Error(err))
		}
	}

	// Register metrics for all plugins
	for _, plugin := range core.GetPlugins() {
		if err := core.RegisterPluginMetrics(plugin.ID, plugin.Metrics); err != nil {
			ctx.Logger().Error("Failed to register plugin metrics",
				zap.String("plugin", plugin.ID),
				zap.Error(err))
		}
	}

	return nil
}

func (p *PortalImpl) initServices() (ctxOpts []core.ContextBuilderOption, svcCtxOpts []core.ContextBuilderOption, err error) {
	// Initialize all services and collect their context options.
	// Returns both direct context options and service-specific options separately
	// to allow staged initialization.
	svcs := core.GetServices()

	for _, svcInfo := range svcs {
		svc, opts, err := svcInfo.Factory()
		if err != nil {
			return nil, nil, err
		}
		if opts != nil {
			svcCtxOpts = append(svcCtxOpts, opts...)
		}
		ctxOpts = append(ctxOpts, core.ContextWithService(svcInfo.ID, svc), core.ContextWithStartupComponent(svc))
	}

	return ctxOpts, svcCtxOpts, nil
}

func (p *PortalImpl) configureProtocols(ctx core.Context) error {
	for name, proto := range core.GetProtocols() {
		err := ctx.Config().ConfigureProtocol(name, proto.GetConfig())
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
			ctxOpts = append(ctxOpts, core.ContextWithStartupComponent(proto))
		}
	}

	return ctxOpts, nil
}

func (p *PortalImpl) configureAPIs(ctx core.Context) error {
	for name, api := range core.GetAPIs() {
		err := ctx.Config().ConfigureAPI(name, api.GetConfig())
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
			ctxOpts = append(ctxOpts, core.ContextWithStartupComponent(api))
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

					// If the extension declares metrics, register them into the
					// target API's plugin registry so they are served on that
					// API's /metrics endpoint.
					if metricsExt, ok := ext.(core.APIExtensionMetrics); ok {
						targetAPI := ext.TargetAPI()
						if err := core.RegisterPluginMetrics(targetAPI, metricsExt.Metrics()); err != nil {
							ctx.Logger().Error("Failed to register API extension metrics",
								zap.String("plugin", plugin.ID),
								zap.String("target_api", targetAPI),
								zap.Error(err))
						} else {
							ctx.Logger().Info("Registered API extension metrics",
								zap.String("plugin", plugin.ID),
								zap.String("target_api", targetAPI))
						}
					}

					ctxOptions = append(ctxOptions, core.ContextWithStartupComponent(ext))

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
			if err = initProto.Init(ctx); err != nil {
				ctx.Logger().Error("Error initializing protocol", zap.String("protocol", proto.Name()), zap.Error(err))
				return nil, err
			}
		}
	}

	return ctxOpts, nil
}

func (p *PortalImpl) initAPIs(ctx core.Context) (ctxOpts []core.ContextBuilderOption, err error) {
	for _, api := range core.GetAPIs() {
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

	err := cronSvc.(core.CronService).Start(ctx)
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

func (p *PortalImpl) fireBootCompletedEvent(ctx core.Context) error {
	return core.Fire(ctx, event.EVENT_BOOT_COMPLETED, event.NewBootCompletedEvent(ctx, ctx.GetContext()))
}

func NewPortal(ctx core.Context) *PortalImpl {
	return &PortalImpl{
		ctx:    ctx,
		logger: ctx.Logger(),
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
	p.logger = ctx.Logger()
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
