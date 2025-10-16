package testing

import (
	"fmt"
	"reflect"

	"go.lumeweb.com/portal/build"
	"go.lumeweb.com/portal/core"
)

type mockServiceEntry struct {
	id      string
	factory func(tb TB, ctx TestContext) any
	depends []string
}

type mockServiceFactoryEntry struct {
	id      string
	factory interface{}
	depends []string
}

// MockPluginBuilder is a builder for creating PluginInfo structs for testing.
type MockPluginBuilder struct {
	plugin               core.PluginInfo
	services             []core.ServiceInfo
	extensions           []core.APIExtensionFactory
	mockServices         []mockServiceEntry
	mockServiceFactories []mockServiceFactoryEntry
	ctx                  TestContext
}

// NewMockPluginBuilder creates a new MockPluginBuilder with a default ID.
func NewMockPluginBuilder(id string) *MockPluginBuilder {
	return &MockPluginBuilder{
		plugin: core.PluginInfo{
			ID:      id,
			Version: build.New("", "", "", "", "", "", ""),
		},
		services:             make([]core.ServiceInfo, 0),
		extensions:           make([]core.APIExtensionFactory, 0),
		mockServices:         make([]mockServiceEntry, 0),
		mockServiceFactories: make([]mockServiceFactoryEntry, 0),
	}
}

// WithVersion sets the version of the plugin.
func (b *MockPluginBuilder) WithVersion(version string) *MockPluginBuilder {
	b.plugin.Version = build.New(version, "", "", "", "", "", "")
	return b
}

// WithMeta sets the MetaFactory of the plugin.
func (b *MockPluginBuilder) WithMeta(meta core.MetaFactory) *MockPluginBuilder {
	b.plugin.Meta = meta
	return b
}

// WithAPI sets the APIFactory of the plugin.
func (b *MockPluginBuilder) WithAPI(api core.APIFactory) *MockPluginBuilder {
	b.plugin.API = api
	return b
}

// WithProtocol sets the ProtocolFactory of the plugin.
func (b *MockPluginBuilder) WithProtocol(protocol core.ProtocolFactory) *MockPluginBuilder {
	b.plugin.Protocol = protocol
	return b
}

// WithService adds an individual service to the plugin with its dependencies.
func (b *MockPluginBuilder) WithService(id string, factory core.ServiceFactory, depends ...string) *MockPluginBuilder {
	b.services = append(b.services, core.ServiceInfo{ID: id, Factory: factory, Depends: depends})
	return b
}

// WithMockService adds a mock service to the plugin with its dependencies.
func (b *MockPluginBuilder) WithMockService(id string, factory func(tb TB, ctx TestContext) any, depends ...string) *MockPluginBuilder {
	b.mockServices = append(b.mockServices, mockServiceEntry{id: id, factory: factory, depends: depends})
	return b
}

// WithMockServiceFactory adds a mock service factory to the plugin with its dependencies.
func (b *MockPluginBuilder) WithMockServiceFactory(id string, factory interface{}, depends ...string) *MockPluginBuilder {
	b.mockServiceFactories = append(b.mockServiceFactories, mockServiceFactoryEntry{id: id, factory: factory, depends: depends})
	return b
}

// WithAPIExtension adds individual API extensions to the plugin with their dependencies.
func (b *MockPluginBuilder) WithAPIExtension(extension core.APIExtensionFactory) *MockPluginBuilder {
	b.extensions = append(b.extensions, extension)
	return b
}

// WithModels sets the Models of the plugin.
func (b *MockPluginBuilder) WithModels(models ...any) *MockPluginBuilder {
	b.plugin.Models = models
	return b
}

// WithMigrations sets the Migrations of the plugin.
func (b *MockPluginBuilder) WithMigrations(migrations core.DBMigration) *MockPluginBuilder {
	b.plugin.Migrations = migrations
	return b
}

// WithDepends sets the Depends of the plugin.
func (b *MockPluginBuilder) WithDepends(depends ...string) *MockPluginBuilder {
	b.plugin.Depends = append(b.plugin.Depends, depends...)
	return b
}

// WithCronJobs sets the CronJobs of the plugin.
func (b *MockPluginBuilder) WithCronJobs(cronJobs ...core.PluginCronJob) *MockPluginBuilder {
	b.plugin.CronJobs = append(b.plugin.CronJobs, cronJobs...)
	return b
}

// WithMailerTemplates sets the MailerTemplates of the plugin.
func (b *MockPluginBuilder) WithMailerTemplates(mailerTemplates core.MailerTemplates) *MockPluginBuilder {
	b.plugin.MailerTemplates = mailerTemplates
	return b
}

// WithWebBundles sets the WebBundles of the plugin.
func (b *MockPluginBuilder) WithWebBundles(webBundles ...*core.WebBundle) *MockPluginBuilder {
	b.plugin.WebBundles = append(b.plugin.WebBundles, webBundles...)
	return b
}

// WithTargetApps sets the TargetApps of the plugin.
func (b *MockPluginBuilder) WithTargetApps(targetApps ...string) *MockPluginBuilder {
	b.plugin.TargetApps = append(b.plugin.TargetApps, targetApps...)
	return b
}

// Build returns the constructed PluginInfo.
func (b *MockPluginBuilder) Build() (core.Service, []core.ContextBuilderOption, error) {
	if b.ctx == nil {
		return nil, nil, fmt.Errorf("test context is nil - Build() requires a non-nil context")
	}

	if len(b.extensions) > 0 {
		b.plugin.APIExtensions = func(context core.Context) ([]core.APIExtensionFactory, error) {
			return b.extensions, nil
		}
	}

	if len(b.services) > 0 || len(b.mockServices) > 0 || len(b.mockServiceFactories) > 0 {
		allServices := make([]core.ServiceInfo, 0, len(b.services)+len(b.mockServices)+len(b.mockServiceFactories))
		allServices = append(allServices, b.services...)

		// Process mock services - add to services list with no-op factories
		for _, mockSvc := range b.mockServices {
			id := mockSvc.id
			factory := mockSvc.factory
			depends := mockSvc.depends

			// Create the mock instance immediately using stored context
			mockInstance := factory(b.ctx.T(), b.ctx)
			if mockInstance == nil {
				return nil, nil, fmt.Errorf("mock service factory for '%s' returned nil", id)
			}

			// Capture the mock instance in the closure
			mockInst := mockInstance

			// Add to services list with a factory that returns the mock instance
			allServices = append(allServices, core.ServiceInfo{
				ID: id,
				Factory: func() (core.Service, []core.ContextBuilderOption, error) {
					return mockInst.(core.Service), nil, nil
				},
				Depends: depends,
			})
		}

		// Process mock service factories - add to services list with no-op factories
		for _, mockSvcFactory := range b.mockServiceFactories {
			id := mockSvcFactory.id
			factory := mockSvcFactory.factory
			depends := mockSvcFactory.depends

			// Validate factory is a function with correct signature
			factoryValue := reflect.ValueOf(factory)
			if factoryValue.Kind() != reflect.Func {
				return nil, nil, fmt.Errorf("mock service factory for '%s' is not a function", id)
			}

			factoryType := factoryValue.Type()
			if factoryType.NumIn() != 1 {
				return nil, nil, fmt.Errorf("mock service factory for '%s' does not have exactly 1 input parameter", id)
			}

			expectedType := reflect.TypeOf(b.ctx.T())
			if !factoryType.In(0).AssignableTo(expectedType) {
				return nil, nil, fmt.Errorf("mock service factory for '%s' first parameter is not assignable from %v", id, expectedType)
			}

			// Call the factory function immediately with stored context's TB
			tbValue := reflect.ValueOf(b.ctx.T())
			
			// Wrap the call in a recover to prevent panics from crashing the process
			var results []reflect.Value
			var callErr error
			func() {
				defer func() {
					if r := recover(); r != nil {
						callErr = fmt.Errorf("mock service factory for '%s' panicked: %v", id, r)
					}
				}()
				results = factoryValue.Call([]reflect.Value{tbValue})
			}()
			
			if callErr != nil {
				return nil, nil, callErr
			}

			// Get the mock instance from the results
			if len(results) == 0 {
				return nil, nil, fmt.Errorf("mock service factory for '%s' returned no values", id)
			}

			mockInstance := results[0].Interface()
			if mockInstance == nil {
				return nil, nil, fmt.Errorf("mock service factory for '%s' returned nil", id)
			}

			// Validate that mockInstance implements core.Service
			mockInst, ok := mockInstance.(core.Service)
			if !ok {
				return nil, nil, fmt.Errorf("mock service factory for '%s' did not return a core.Service implementation", id)
			}

			// Add to services list with a factory that returns the mock instance
			allServices = append(allServices, core.ServiceInfo{
				ID: id,
				Factory: func() (core.Service, []core.ContextBuilderOption, error) {
					return mockInst, nil, nil
				},
				Depends: depends,
			})
		}

		b.plugin.Services = func() ([]core.ServiceInfo, error) {
			return allServices, nil
		}
	}

	return nil, []core.ContextBuilderOption{}, nil
}

// PluginOption returns a ContextBuilderOption that registers the built plugin.
func (b *MockPluginBuilder) BuilderOption() TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		b.ctx = ctx // Store the context
		_, _, err := b.Build()
		if err != nil {
			return ctx, err
		}
		core.RegisterPlugin(b.plugin)
		return ctx, nil
	}
}
