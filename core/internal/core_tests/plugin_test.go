package core_tests

import (
	"go.lumeweb.com/portal/build"
	core "go.lumeweb.com/portal/core"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegisterPlugin(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	plugin := newTestPluginInfo("test-plugin")
	core.RegisterPlugin(plugin)

	retrievedPlugin := core.GetPlugin("test-plugin")
	assert.Equal(t, plugin.ID, retrievedPlugin.ID)
	assert.Equal(t, plugin.Version, retrievedPlugin.Version)
}

func TestRegisterPlugin_DuplicateID(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	plugin1 := newTestPluginInfo("duplicate-plugin")
	core.RegisterPlugin(plugin1)

	plugin2 := newTestPluginInfo("duplicate-plugin") // Same ID

	assert.PanicsWithValue(t, "plugin already registered: duplicate-plugin", func() {
		core.RegisterPlugin(plugin2)
	})
}

func TestRegisterPlugin_MissingComponent(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	plugin := core.PluginInfo{
		ID:      "no-component-plugin",
		Version: build.New("test-version", "", "", "", "", "", ""),
		// No API, Protocol, Services, APIExtensions, or WebBundles
	}

	assert.PanicsWithValue(t, "plugin must have at least one of API, Protocol, Service, APIExtension, WebBundle, CronJob, or KeyIdentityHandler", func() {
		core.RegisterPlugin(plugin)
	})
}

func TestRegisterPlugin_KeyIdentityHandler_NilHandlerAndEmptyTypeRejected(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	// All entries are invalid: nil handler or empty type
	plugin := core.PluginInfo{
		ID:      "invalid-keyidentity-plugin",
		Version: build.New("test-version", "", "", "", "", "", ""),
		KeyIdentityHandlers: []core.KeyIdentityHandlerRegistration{
			{Type: "", Handler: nil},               // both invalid
			{Type: "valid_type", Handler: nil},     // nil handler
			{Type: "", Handler: nil},              // empty type
		},
	}

	assert.PanicsWithValue(t, "plugin must have at least one of API, Protocol, Service, APIExtension, WebBundle, CronJob, or KeyIdentityHandler", func() {
		core.RegisterPlugin(plugin)
	})
}

func TestGetPlugin(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	plugin := newTestPluginInfo("get-plugin")
	core.RegisterPlugin(plugin)

	retrievedPlugin := core.GetPlugin("get-plugin")
	assert.Equal(t, plugin.ID, retrievedPlugin.ID)
}

func TestGetPlugin_NotFound(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	retrievedPlugin := core.GetPlugin("non-existent-plugin")
	assert.Equal(t, core.PluginInfo{}, retrievedPlugin) // Should return zero value
}

func TestGetPlugins_Ordering(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	// Plugins with dependencies
	pluginA := newTestPluginInfo("plugin-a")
	pluginB := newTestPluginInfo("plugin-b", "plugin-a")
	pluginC := newTestPluginInfo("plugin-c", "plugin-b")
	pluginD := newTestPluginInfo("plugin-d") // No dependencies

	core.RegisterPlugin(pluginC) // Register out of order
	core.RegisterPlugin(pluginA)
	core.RegisterPlugin(pluginD)
	core.RegisterPlugin(pluginB)

	plugins := core.GetPlugins()

	// Expected order: plugin-a, plugin-d, plugin-b, plugin-c (or plugin-d, plugin-a, plugin-b, plugin-c)
	// The graph build ensures dependencies come first. Order of independent nodes can vary.
	// We need to check the relative order of dependent nodes.
	assert.Len(t, plugins, 4)

	// Find indices
	idxA := -1
	idxB := -1
	idxC := -1
	idxD := -1
	for i, p := range plugins {
		switch p.ID {
		case "plugin-a":
			idxA = i
		case "plugin-b":
			idxB = i
		case "plugin-c":
			idxC = i
		case "plugin-d":
			idxD = i
		}
	}

	assert.True(t, idxA != -1 && idxB != -1 && idxC != -1 && idxD != -1, "All plugins should be in the list")
	assert.Less(t, idxA, idxB, "plugin-a should come before plugin-b")
	assert.Less(t, idxB, idxC, "plugin-b should come before plugin-c")
}

func TestGetPlugins_CycleDetection(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	pluginA := newTestPluginInfo("plugin-a", "plugin-c") // Depends on C
	pluginB := newTestPluginInfo("plugin-b", "plugin-a") // Depends on A
	pluginC := newTestPluginInfo("plugin-c", "plugin-b") // Depends on B (Cycle: A -> B -> C -> A)

	core.RegisterPlugin(pluginA)
	core.RegisterPlugin(pluginB)
	core.RegisterPlugin(pluginC)

	assert.Panics(t, func() {
		core.GetPlugins()
	}, "Should panic on dependency cycle")

	// Check the panic message contains cycle information
	defer func() {
		if r := recover(); r != nil {
			err, ok := r.(error)
			assert.True(t, ok, "Panic value should be an error")
			assert.Contains(t, err.Error(), "cycle detected", "Panic message should indicate a cycle")
		}
	}()

	core.GetPlugins() // This call should panic
}

func TestGetPlugins_NoPlugins(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	plugins := core.GetPlugins()
	assert.Empty(t, plugins)
}

func TestPluginHasAPI(t *testing.T) {
	pluginWithAPI := newTestPluginInfoWithComponent(t, "api-plugin", "API")
	pluginWithoutAPI := newTestPluginInfo("no-api-plugin")

	assert.True(t, core.PluginHasAPI(pluginWithAPI))
	assert.False(t, core.PluginHasAPI(pluginWithoutAPI))
}

func TestPluginHasProtocol(t *testing.T) {
	pluginWithProtocol := newTestPluginInfoWithComponent(t, "protocol-plugin", "Protocol")
	pluginWithoutProtocol := newTestPluginInfo("no-protocol-plugin")

	assert.True(t, core.PluginHasProtocol(pluginWithProtocol))
	assert.False(t, core.PluginHasProtocol(pluginWithoutProtocol))
}

func TestPluginHasServices(t *testing.T) {
	pluginWithServices := newTestPluginInfoWithComponent(t, "services-plugin", "Services")
	pluginWithoutServices := newTestPluginInfo("no-services-plugin")

	assert.True(t, core.PluginHasServices(pluginWithServices))
	assert.False(t, core.PluginHasServices(pluginWithoutServices))
}

func TestPluginHasAPIExtensions(t *testing.T) {
	pluginWithExtensions := newTestPluginInfoWithComponent(t, "extensions-plugin", "APIExtensions")
	pluginWithoutExtensions := newTestPluginInfo("no-extensions-plugin")

	assert.True(t, core.PluginHasAPIExtensions(pluginWithExtensions))
	assert.False(t, core.PluginHasAPIExtensions(pluginWithoutExtensions))
}

func TestPluginHasCron(t *testing.T) {
	pluginWithCron := newTestPluginInfoWithComponent(t, "cron-plugin", "Cron")
	pluginWithoutCron := newTestPluginInfo("no-cron-plugin")

	assert.True(t, core.PluginHasCron(pluginWithCron))
	assert.False(t, core.PluginHasCron(pluginWithoutCron))
}
