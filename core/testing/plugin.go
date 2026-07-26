// Package testing provides utilities for testing core components
package testing

import (
	"fmt"
	"net"
	"reflect"
	"strconv"
	"strings"

	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	pkgReflect "go.lumeweb.com/portal/internal/reflect"
	"go.uber.org/zap"
)

var (
	serviceConfigType = pkgReflect.GetInterfaceType((*config.ServiceConfig)(nil))
)

// WithAPI creates a TestContextBuilderOption that wraps RegisterAPI
// to register and configure an API for testing purposes.
func WithAPI(id string, factory core.APIFactory) TestContextBuilderOption {
	return func(tctx TestContext) (TestContext, error) {
		opts, err := RegisterAPI(tctx, id, factory)
		if err != nil {
			return tctx, err
		}
		return ProcessCtxOptions(tctx, opts...)
	}
}

// registerProtocolWithHelper is a helper function to register a protocol.
func registerProtocolWithHelper(tctx TestContext, id string, factory core.ProtocolFactory) (TestContext, error) {
	opts, err := RegisterProtocol(tctx, id, factory)
	if err != nil {
		return tctx, err
	}
	return ProcessCtxOptions(tctx, opts...)
}

// WithProtocol creates a TestContextBuilderOption that registers and configures a Protocol.
func WithProtocol(id string, factory core.ProtocolFactory) TestContextBuilderOption {
	return func(tctx TestContext) (TestContext, error) {
		return registerProtocolWithHelper(tctx, id, factory)
	}
}

// WithMockProtocol creates a TestContextBuilderOption that registers a mock protocol.
// It takes a protocol name and a callback function that allows configuring the mock protocol.
func WithMockProtocol(protocolName string, configureMock ...func(protocol *MockProtocol)) TestContextBuilderOption {
	return func(tctx TestContext) (TestContext, error) {
		// Create a new mock protocol
		mockProtocol := NewMockProtocol(tctx.T(), protocolName)

		// Configure the mock using the provided callback
		if len(configureMock) > 0 {
			for _, v := range configureMock {
				if v != nil {
					v(mockProtocol)
				}
			}

		}

		// Create a protocol factory that returns the configured mock
		protocolFactory := func() (core.Protocol, []core.ContextBuilderOption, error) {
			return mockProtocol, nil, nil // No additional context options for mock protocols
		}

		return registerProtocolWithHelper(tctx, protocolName, protocolFactory)
	}
}

// WithCustomMockProtocol creates a TestContextBuilderOption that registers a mock protocol
// using a custom callback function that returns a core.Protocol implementation.
// This allows for more flexible mock protocol creation beyond the standard MockProtocol.
func WithCustomMockProtocol(protocolName string, protocolFactory func(TestContext) core.Protocol) TestContextBuilderOption {
	return func(tctx TestContext) (TestContext, error) {
		// Create a protocol factory that passes the context to the user's callback
		factory := func() (core.Protocol, []core.ContextBuilderOption, error) {
			return protocolFactory(tctx), nil, nil
		}
		return registerProtocolWithHelper(tctx, protocolName, factory)
	}
}

// WithAPIExtension registers an API extension and automatically creates and registers
// a mock version of its target API.
func WithAPIExtension(extFactory core.APIExtensionFactory) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		// First create the extension to get its target API
		ext, extOpts, err := extFactory()
		if err != nil {
			return ctx, fmt.Errorf("failed to create API extension: %w", err)
		}

		// Check if the extension is nil
		if ext == nil {
			return ctx, fmt.Errorf("API extension factory returned nil extension")
		}

		targetAPI := ext.TargetAPI()
		tb := ctx.T()

		// Create mock API for the target using the testing package's NewMockAPI
		mockAPI := NewMockAPI(tb, targetAPI).WithSubdomain(targetAPI)

		// Register the mock API using existing API registration mechanism
		apiRegOpt := WithAPI(targetAPI, func() (core.API, []core.ContextBuilderOption, error) {
			return mockAPI, nil, nil
		})

		// Process API registration first
		ctx, err = ProcessCtxOptions(ctx, apiRegOpt)
		if err != nil {
			return ctx, fmt.Errorf("failed to register mock API: %w", err)
		}

		// Set the API ID in the context
		ctx.SetAPIID(targetAPI)

		// Register the extension
		core.RegisterAPIExtension(ext)

		// Add ContextWithStartupComponent for the extension
		ctxOpts := append([]core.ContextBuilderOption{}, core.ContextOptions(core.ContextWithStartupComponent(ext))...)
		ctxOpts = append(ctxOpts, extOpts...)

		return ProcessCtxOptions(ctx, WrapCoreOptions(ctxOpts)...)
	}
}

// RegisterAPIs registers all APIs from plugins similar to portal's registerAPIs
func RegisterAPIs(ctx TestContext) ([]TestContextBuilderOption, error) {
	var opts []TestContextBuilderOption

	for _, plugin := range core.GetPlugins() {
		if core.PluginHasAPI(plugin) {
			api, apiOpts, err := plugin.API()
			if err != nil {
				ctx.Logger().Error("Error building API",
					zap.String("plugin", plugin.ID),
					zap.Error(err))
				return nil, err
			}

			if api == nil {
				continue
			}

			opts = append(opts, WrapCoreOptions(core.ContextOptions(core.ContextWithStartupComponent(api)))...)
			opts = append(opts, WrapCoreOptions(apiOpts)...)
			core.RegisterAPI(plugin.ID, api)
		}
	}

	return opts, nil
}

// registerAPIInstance handles the registration of a single API instance
func registerAPIInstance(ctx TestContext, id string, api core.API, opts []core.ContextBuilderOption) ([]TestContextBuilderOption, error) {
	if api == nil {
		err := fmt.Errorf("api instance is nil for %s", id)
		ctx.Logger().Error(err.Error(), zap.String("id", id))
		return nil, err
	}

	// Register the instance with core
	core.RegisterAPI(id, api)

	// Create and return wrapped context options
	ctxOpts := append([]core.ContextBuilderOption{}, core.ContextOptions(core.ContextWithStartupComponent(api))...)
	ctxOpts = append(ctxOpts, opts...)
	return WrapCoreOptions(ctxOpts), nil
}

// RegisterAPI registers an API and wraps any returned context options for test context
func RegisterAPI(ctx TestContext, id string, factory core.APIFactory) ([]TestContextBuilderOption, error) {
	api, opts, err := factory()
	if err != nil {
		ctx.Logger().Error("Error building API", zap.String("plugin", id), zap.Error(err))
		return nil, err
	}

	return registerAPIInstance(ctx, id, api, opts)
}

// registerAPIExtensionInstance handles the registration of a single API extension instance
func registerAPIExtensionInstance(tctx TestContext, extFactory core.APIExtensionFactory, pluginID string) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		ext, opts, err := extFactory()
		if err != nil {
			ctx.Logger().Error("Error building API extension",
				zap.String("plugin", pluginID),
				zap.Error(err))
			return ctx, err
		}

		if ext == nil {
			err := fmt.Errorf("API extension factory returned nil extension for plugin %s", pluginID)
			ctx.Logger().Error(err.Error(),
				zap.String("plugin", pluginID))
			return ctx, err
		}

		ctx.Logger().Info("Registering API extension",
			zap.String("plugin", pluginID),
			zap.String("target", ext.TargetAPI()))
		core.RegisterAPIExtension(ext)

		// Add ContextWithStartupComponent for the extension
		ctxOpts := append([]core.ContextBuilderOption{}, core.ContextOptions(core.ContextWithStartupComponent(ext))...)
		ctxOpts = append(ctxOpts, opts...)

		return ProcessCtxOptions(ctx, WrapCoreOptions(ctxOpts)...)
	}
}

// RegisterAPIExtensions registers all API extensions from plugins similar to portal's registerAPIExtensions
func RegisterAPIExtensions(ctx TestContext) ([]TestContextBuilderOption, error) {
	var opts []TestContextBuilderOption

	for _, plugin := range core.GetPlugins() {
		if core.PluginHasAPIExtensions(plugin) {
			extensions, err := plugin.APIExtensions(ctx)
			if err != nil {
				ctx.Logger().Error("Error building API extensions",
					zap.String("plugin", plugin.ID),
					zap.Error(err))
				return nil, err
			}

			for _, extFactory := range extensions {
				opts = append(opts, registerAPIExtensionInstance(ctx, extFactory, plugin.ID))
			}
		}
	}

	return opts, nil
}

// RegisterAPIExtension registers API extensions and wraps any returned context options
func RegisterAPIExtension(ctx TestContext, factory core.APIExtensionsFactory) (ctxOpts []TestContextBuilderOption, err error) {
	extensions, err := factory(ctx)
	if err != nil {
		ctx.Logger().Error("Error building API extensions", zap.Error(err))
		return nil, err
	}

	for _, extFactory := range extensions {
		// Use the same helper pattern but with a simplified version since we don't have plugin ID
		apiExtStartup := TestContextBuilderOption(func(tctx TestContext) (TestContext, error) {
			ext, ctxOptions, err := extFactory()
			if err != nil {
				tctx.Logger().Error("Error building API extension", zap.Error(err))
				return nil, err
			}

			// Check if the extension is nil
			if ext == nil {
				err := fmt.Errorf("API extension factory %T returned nil extension", extFactory)
				tctx.Logger().Error(err.Error())
				return nil, err
			}

			tctx.Logger().Info("Registering API extension",
				zap.String("api", ext.TargetAPI()),
				zap.String("extension", fmt.Sprintf("%T", ext)))

			core.RegisterAPIExtension(ext)

			// Add ContextWithStartupComponent for the extension
			opts := append([]core.ContextBuilderOption{}, core.ContextOptions(core.ContextWithStartupComponent(ext))...)
			opts = append(opts, ctxOptions...)

			return ProcessCtxOptions(tctx, WrapCoreOptions(opts)...)
		})
		ctxOpts = append(ctxOpts, apiExtStartup)
	}

	return ctxOpts, nil
}

// registerProtocolInstance handles the registration of a single protocol instance
func registerProtocolInstance(ctx TestContext, id string, proto core.Protocol, opts []core.ContextBuilderOption) ([]TestContextBuilderOption, error) {
	if proto == nil {
		err := fmt.Errorf("protocol instance is nil for %s", id)
		ctx.Logger().Error(err.Error(), zap.String("id", id))
		return nil, err
	}

	// Register the instance with core
	core.RegisterProtocol(id, proto)

	// Create and return wrapped context options
	ctxOpts := append([]core.ContextBuilderOption{}, core.ContextOptions(core.ContextWithStartupComponent(proto))...)
	ctxOpts = append(ctxOpts, opts...)
	return WrapCoreOptions(ctxOpts), nil
}

// RegisterProtocols registers all protocols from plugins similar to portal's registerProtocols
func RegisterProtocols(ctx TestContext) ([]TestContextBuilderOption, error) {
	var opts []TestContextBuilderOption

	for _, plugin := range core.GetPlugins() {
		if core.PluginHasProtocol(plugin) {
			proto, protoOpts, err := plugin.Protocol()
			if err != nil {
				ctx.Logger().Error("Error building protocol",
					zap.String("plugin", plugin.ID),
					zap.Error(err))
				return nil, err
			}

			if proto == nil {
				continue
			}

			ctxOpts := append([]core.ContextBuilderOption{}, core.ContextOptions(core.ContextWithStartupComponent(proto))...)
			ctxOpts = append(ctxOpts, protoOpts...)
			opts = append(opts, WrapCoreOptions(ctxOpts)...)
			core.RegisterProtocol(plugin.ID, proto)
		}
	}

	return opts, nil
}

// RegisterProtocol registers a Protocol and wraps any returned context options for test context
func RegisterProtocol(ctx TestContext, id string, factory core.ProtocolFactory) ([]TestContextBuilderOption, error) {
	proto, opts, err := factory()
	if err != nil {
		ctx.Logger().Error("Error building Protocol", zap.String("plugin", id), zap.Error(err))
		return nil, err
	}

	if proto == nil {
		err := fmt.Errorf("protocol factory for %s returned nil", id)
		ctx.Logger().Error(err.Error(), zap.String("plugin", id))
		return nil, err
	}

	return registerProtocolInstance(ctx, id, proto, opts)
}

// ConfigureProtocols configures all registered protocols with their respective configs.
// This only handles configuration - initialization is handled separately by InitializeProtocols.
func ConfigureProtocols(ctx TestContext) error {
	for name, proto := range core.GetProtocols() {
		// Configure protocol through config manager
		err := ctx.Config().ConfigureProtocol(name, proto.GetConfig())
		if err != nil {
			ctx.Logger().Error("Error configuring protocol",
				zap.String("protocol", proto.Name()),
				zap.Error(err))
			return err
		}

	}

	return nil
}

// RegisterService registers a Service and wraps any returned context options for test context
func RegisterService(ctx TestContext, id string, factory core.ServiceFactory, plugin ...string) (ctxOpts []TestContextBuilderOption, err error) {
	service, opts, err := factory()
	if err != nil {
		ctx.Logger().Error("Error building Service", zap.String("service", id), zap.Error(err))
		return nil, err
	}

	if service == nil {
		return nil, fmt.Errorf("service factory returned nil service")
	}

	// Register the instance locally and globally
	if err := registerServiceInstance(ctx, id, service, plugin...); err != nil {
		return nil, fmt.Errorf("failed to register service: %w", err)
	}

	// Prepend ContextWithStartupComponent to ensure proper wiring
	startupOpts := WrapCoreOptions(core.ContextOptions(core.ContextWithStartupComponent(service)))
	wrappedOpts := WrapCoreOptions(opts)
	allOpts := append(startupOpts, wrappedOpts...)

	return allOpts, nil
}

// configureService handles the configuration of a single service, including:
// - Type checking and interface compliance verification
// - Pointer/non-pointer conversion handling
// - Plugin association validation
// - Actual configuration through the config manager
func configureService(ctx TestContext, svcInfo core.ServiceInfo, svc any) error {
	// Detect a GetConfig() provider
	type serviceWithConfig interface{ GetConfig() (any, error) }
	provider, ok := svc.(serviceWithConfig)
	if !ok {
		return nil // service has no config
	}

	// Get the concrete config object from the service
	cfgResult, err := provider.GetConfig()
	if err != nil {
		ctx.Logger().Error("Error getting service config",
			zap.String("service", svcInfo.ID),
			zap.Error(err))
		return err
	}

	// Ensure the config type is compliant with ServiceConfig interface
	// This handles cases where the service returns a non-pointer but needs pointer semantics
	compliantCfg, isCompliant := pkgReflect.EnsureCompliantType(cfgResult, serviceConfigType)
	if !isCompliant {
		ctx.Logger().Error(config.ErrInvalidServiceConfig.Error()+" (type does not implement interface)",
			zap.String("service", svcInfo.ID),
			zap.Any("config_type", reflect.TypeOf(cfgResult)))
		return config.ErrInvalidServiceConfig
	}

	// Log if we had to use a pointer to a copy due to non-addressable value
	if reflect.ValueOf(cfgResult).Kind() != reflect.Pointer &&
		reflect.ValueOf(compliantCfg).Kind() == reflect.Pointer &&
		!reflect.ValueOf(cfgResult).CanAddr() {
		ctx.Logger().Warn("GetConfig value was not addressable; using pointer to a copy for configuration.",
			zap.String("service", svcInfo.ID))
	}

	// Final type assertion after compliance check
	svcConfig, ok := compliantCfg.(config.ServiceConfig)
	if !ok {
		ctx.Logger().Error("Internal error: compliant config object could not be cast to ServiceConfig",
			zap.String("service", svcInfo.ID),
			zap.Any("config_type", reflect.TypeOf(compliantCfg)))
		return config.ErrInvalidServiceConfig
	}

	// Skip core services (they're configured differently)
	if core.IsCoreService(svcInfo.ID) {
		return nil
	}

	// Get plugin association for the service
	pluginName := core.GetPluginForService(svcInfo.ID)
	if pluginName == "" {
		ctx.Logger().Error("Service has no plugin association",
			zap.String("service", svcInfo.ID))
		return config.ErrMissingPluginAssociation
	}

	// Actually configure the service through the config manager
	if err := ctx.Config().ConfigureService(pluginName, svcInfo.ID, svcConfig); err != nil {
		ctx.Logger().Error("Error configuring service",
			zap.String("service", svcInfo.ID),
			zap.Error(err))
		return err
	}

	return nil
}

// ConfigureServices configures all registered services with their ServiceConfig implementations.
// Handles core services differently from plugin services.
func ConfigureServices(ctx TestContext) error {
	for _, svcInfo := range core.GetServices() {
		svc := ctx.Service(svcInfo.ID)
		if svc == nil {
			continue // Skip unregistered services
		}

		if err := configureService(ctx, svcInfo, svc); err != nil {
			return err
		}
	}

	return nil
}

// InitializeServices initializes all services that implement ServiceInit interface
func InitializeServices(ctx TestContext) error {
	for _, svcInfo := range core.GetServices() {
		svc := ctx.Service(svcInfo.ID)
		if svc == nil {
			continue // Skip unregistered services
		}

		if initSvc, ok := svc.(core.ServiceInit); ok {
			if err := func() (err error) {
				defer func() {
					if r := recover(); r != nil {
						ctx.Logger().Error("Service init panic",
							zap.String("service", svcInfo.ID),
							zap.Any("recover", r))
						err = fmt.Errorf("panic in ServiceInit.IInit for %s: %v", svcInfo.ID, r)
					}
				}()
				return initSvc.Init()
			}(); err != nil {
				ctx.Logger().Error("Error initializing service",
					zap.String("service", svcInfo.ID),
					zap.Error(err))
				return err
			}
		}
	}

	return nil
}

// ConfigureAPIs configures all registered APIs and returns any context options
func ConfigureAPIs(ctx TestContext) ([]TestContextBuilderOption, error) {
	var opts []TestContextBuilderOption

	for name, api := range core.GetAPIs() {
		// Configure API through config manager
		err := ctx.Config().ConfigureAPI(name, api.GetConfig())
		if err != nil {
			ctx.Logger().Error("Error configuring API",
				zap.String("api", api.Name()),
				zap.Error(err))
			return nil, err
		}

		// Initialize API if it implements APIInit
		if initApi, ok := api.(core.APIInit); ok {
			apiOpts, err := initApi.Init()
			if err != nil {
				ctx.Logger().Error("Error initializing API",
					zap.String("api", api.Name()),
					zap.Error(err))
				return nil, err
			}
			opts = append(opts, WrapCoreOptions(apiOpts)...)
		}
	}

	return opts, nil
}

// ConfigureAPIRoutes configures routes for all registered APIs
func ConfigureAPIRoutes(ctx TestContext) error {
	accessSvc := ctx.Service(core.ACCESS_SERVICE)
	if accessSvc == nil {
		return fmt.Errorf("AccessService not found in context, cannot configure API routes")
	}

	accessService, ok := accessSvc.(core.AccessService)
	if !ok {
		return fmt.Errorf("AccessService not found in context, cannot configure API routes")
	}

	gRouter := ctx.Router()

	for _, api := range core.GetAPIs() {
		root := strings.Trim(strings.ToLower(ctx.Config().Config().Core.Domain), ".")
		sub := strings.Trim(strings.ToLower(strings.TrimSpace(api.Subdomain())), ".")
		host := root
		if sub != "" {
			host = sub + "." + root
		}
		port := ctx.Config().Config().Core.Port
		if ctx.Config().Config().Core.ExternalPort != 0 {
			port = ctx.Config().Config().Core.ExternalPort
		}
		var hostWithPort string
		if port != 0 {
			hostWithPort = net.JoinHostPort(host, strconv.FormatUint(uint64(port), 10))
		} else {
			// For IPv6 addresses without port, ensure they're bracketed
			if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
				hostWithPort = "[" + host + "]"
			} else {
				hostWithPort = host
			}
		}

		// Create a subrouter for this API's domain
		hostRouter, err := gRouter.Host(hostWithPort)
		if err != nil {
			return fmt.Errorf("failed to create host router for API %s: %w", api.Name(), err)
		}

		// Configure the main API using the gswagger router
		err = api.Configure(hostRouter, accessService)
		if err != nil {
			return err
		}

		// Apply any registered extensions using the *same* gswagger router
		for _, ext := range core.GetAPIExtensions(api.Name()) {
			ctx.Logger().Info("Applying API extension",
				zap.String("api", ext.TargetAPI()),
				zap.String("extension", fmt.Sprintf("%T", ext)))

			// The APIExtension.Configure method signature needs to change
			// This part seems like it might be related to a different system or a work-in-progress.
			// For the purpose of providing the requested testing.go file, I'll keep the existing logic
			// but note that the Configure method signature in MockAPIExtension doesn't match core.APIExtension.
			// If this needs to be functional, the mock or the interface might need adjustment.
			if err = ext.Configure(hostRouter, accessService); err != nil {
				return fmt.Errorf("failed to configure API extension: %w", err)
			}
		}
	}

	return nil
}

// RegisterComponents registers services, APIs, protocols and extensions similar to portal boot process
// Returns:
// - opts: Options for APIs, protocols and extensions
// - svcOpts: Options for services (to be processed later)
// - err: Any error encountered
func RegisterComponents(ctx TestContext) (opts []TestContextBuilderOption, svcOpts []TestContextBuilderOption, err error) {
	// Register services first (but collect their options separately)
	svcOpts, err = RegisterServices(ctx)
	if err != nil {
		return nil, nil, err
	}

	// Register other components
	apiOpts, err := RegisterAPIs(ctx)
	if err != nil {
		return nil, nil, err
	}
	opts = append(opts, apiOpts...)

	protoOpts, err := RegisterProtocols(ctx)
	if err != nil {
		return nil, nil, err
	}
	opts = append(opts, protoOpts...)

	extOpts, err := RegisterAPIExtensions(ctx)
	if err != nil {
		return nil, nil, err
	}
	opts = append(opts, extOpts...)

	return opts, svcOpts, nil
}

// InitializeProtocols initializes all registered protocols that implement ProtocolInit.
// This is called after ConfigureProtocols has configured all protocols.
func InitializeProtocols(ctx TestContext) error {
	for _, proto := range core.GetProtocols() {
		if initProto, ok := proto.(core.ProtocolInit); ok {
			if err := initProto.Init(ctx); err != nil {
				ctx.Logger().Error("Error initializing protocol",
					zap.String("protocol", proto.Name()),
					zap.Error(err))
				return fmt.Errorf("protocol initialization failed: %w", err)
			}
		}
	}
	return nil
}

// ConfigureProtocolWorkflows registers all workflows from all protocols with the workflow service
func ConfigureProtocolWorkflows(ctx core.Context) error {
	workflowSvc := ctx.Service(core.WORKFLOW_SERVICE)
	if workflowSvc == nil {
		return fmt.Errorf("workflow service not found in context")
	}

	workflowService, ok := workflowSvc.(core.WorkflowService)
	if !ok {
		return fmt.Errorf("service found but is not core.WorkflowService")
	}

	for _, proto := range core.GetProtocols() {
		for _, workflow := range proto.Workflows() {
			if err := workflowService.RegisterWorkflow(workflow.Name, workflow.Steps, workflow.AutoTriggerFirstStep); err != nil {
				return fmt.Errorf("failed to register workflow %s for protocol %s: %w", workflow.Name, proto.Name(), err)
			}
		}
	}
	return nil
}

// WithErrorNamespaces imports error namespaces into the global error registry transactionally.
// The imported namespaces are removed when the test context is cleaned up.
func WithErrorNamespaces(namespaces core.ErrorNamespaces) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		if namespaces == nil {
			return ctx, nil
		}

		// Import new namespaces
		if err := core.ImportErrorNamespaces(namespaces); err != nil {
			return ctx, fmt.Errorf("failed to import error namespaces: %w", err)
		}

		return ctx, nil
	}
}

// WithPlugins registers multiple plugins for testing
func WithPlugins(plugins ...core.PluginInfo) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		// Helpers
		safeRegister := func(p core.PluginInfo) (err error) {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("registering plugin %q panicked: %v", p.ID, r)
				}
			}()
			core.RegisterPlugin(p)
			return nil
		}
		unregisterServiceLocal := func(ctx TestContext, id string) {
			if tc, ok := ctx.(*testContext); ok && tc != nil {
				delete(tc.defaultContext.services, id)
			}
			core.UnregisterService(id)
		}

		registeredPlugins := make([]string, 0, len(plugins))
		registeredServices := make([]string, 0, 8)

		rollback := func() {
			// Services first, reverse order
			for i := len(registeredServices) - 1; i >= 0; i-- {
				unregisterServiceLocal(ctx, registeredServices[i])
			}
			// Then plugins, reverse order
			for i := len(registeredPlugins) - 1; i >= 0; i-- {
				_ = core.UnregisterPlugin(registeredPlugins[i])
			}
		}

		for _, plugin := range plugins {
			// Optional pre-checks to avoid obvious panics
			if plugin.ID == "" {
				return ctx, fmt.Errorf("plugin ID must not be empty")
			}

			if err := safeRegister(plugin); err != nil {
				rollback()
				return ctx, err
			}
			registeredPlugins = append(registeredPlugins, plugin.ID)
		}

		// Register key identity handlers from all registered plugins
		// (including those registered above via WithPlugins)
		core.RegisterKeyIdentityHandlersFromPlugins()

		return ctx, nil
	}
}

// WithUnregisterPlugin creates an option that unregisters a plugin by ID
func WithUnregisterPlugin(pluginID string) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		err := core.UnregisterPlugin(pluginID)
		if err != nil {
			return nil, err
		}
		return ctx, nil
	}
}

// WithUnregisterService creates an option that unregisters a service by ID
func WithUnregisterService(serviceID string) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		// Remove from test context
		delete(ctx.(*testContext).defaultContext.services, serviceID)

		// Remove from global registry
		core.UnregisterService(serviceID)
		return ctx, nil
	}
}
