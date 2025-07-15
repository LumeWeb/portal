package testing

import (
	"go.lumeweb.com/portal/build"
	"go.lumeweb.com/portal/core"
)

// MockPluginBuilder is a builder for creating PluginInfo structs for testing.
type MockPluginBuilder struct {
	plugin core.PluginInfo
}

// NewMockPluginBuilder creates a new MockPluginBuilder with a default ID.
func NewMockPluginBuilder(id string) *MockPluginBuilder {
	return &MockPluginBuilder{
		plugin: core.PluginInfo{
			ID:      id,
			Version: build.New("", "", "", "", "", "", ""),
		},
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

// WithServices sets the ServicesFactory of the plugin.
func (b *MockPluginBuilder) WithServices(services core.ServicesFactory) *MockPluginBuilder {
	b.plugin.Services = services
	return b
}

// WithAPIExtensions sets the APIExtensionsFactory of the plugin.
func (b *MockPluginBuilder) WithAPIExtensions(extensions core.APIExtensionsFactory) *MockPluginBuilder {
	b.plugin.APIExtensions = extensions
	return b
}

// WithModels sets the Models of the plugin.
func (b *MockPluginBuilder) WithModels(models []any) *MockPluginBuilder {
	b.plugin.Models = models
	return b
}

// WithMigrations sets the Migrations of the plugin.
func (b *MockPluginBuilder) WithMigrations(migrations core.DBMigration) *MockPluginBuilder {
	b.plugin.Migrations = migrations
	return b
}

// WithDepends sets the Depends of the plugin.
func (b *MockPluginBuilder) WithDepends(depends []string) *MockPluginBuilder {
	b.plugin.Depends = depends
	return b
}

// WithCronJobs sets the CronJobs of the plugin.
func (b *MockPluginBuilder) WithCronJobs(cronJobs []core.PluginCronJob) *MockPluginBuilder {
	b.plugin.CronJobs = cronJobs
	return b
}

// WithMailerTemplates sets the MailerTemplates of the plugin.
func (b *MockPluginBuilder) WithMailerTemplates(mailerTemplates core.MailerTemplates) *MockPluginBuilder {
	b.plugin.MailerTemplates = mailerTemplates
	return b
}

// WithWebBundles sets the WebBundles of the plugin.
func (b *MockPluginBuilder) WithWebBundles(webBundles []*core.WebBundle) *MockPluginBuilder {
	b.plugin.WebBundles = webBundles
	return b
}

// WithTargetApps sets the TargetApps of the plugin.
func (b *MockPluginBuilder) WithTargetApps(targetApps []string) *MockPluginBuilder {
	b.plugin.TargetApps = targetApps
	return b
}

// Build returns the constructed PluginInfo.
func (b *MockPluginBuilder) Build() core.PluginInfo {
	return b.plugin
}

// PluginOption returns a ContextBuilderOption that registers the built plugin.
func (b *MockPluginBuilder) BuilderOption() core.ContextBuilderOption {
	return func(ctx core.Context) (core.Context, error) {
		core.RegisterPlugin(b.plugin)
		return ctx, nil
	}
}
