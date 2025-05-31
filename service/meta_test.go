package service

import (
	"github.com/stretchr/testify/assert"
	"go.lumeweb.com/portal/build"
	"testing"
)

func TestPortalMetaBuilder(t *testing.T) {
	t.Run("NewBuilder", func(t *testing.T) {
		builder := NewPortalMetaBuilder("example.com")
		assert.NotNil(t, builder)
	})

	t.Run("AddFeatureFlag", func(t *testing.T) {
		builder := NewPortalMetaBuilder("example.com")
		builder = builder.AddFeatureFlag("test_flag", true)

		meta := builder.Build()
		assert.True(t, meta.FeatureFlags["TEST_FLAG"])
	})

	t.Run("AddCoreBuildInfo", func(t *testing.T) {
		builder := NewPortalMetaBuilder("example.com")
		buildInfo := build.Info{
			Version: "1.0.0",
		}
		builder = builder.AddCoreBuildInfo(buildInfo)

		meta := builder.Build()
		assert.Equal(t, "1.0.0", meta.Build.GetVersion())
	})

	t.Run("AddPlugin", func(t *testing.T) {
		builder := NewPortalMetaBuilder("example.com")
		pluginBuilder, err := builder.AddPlugin("test_plugin")
		assert.NoError(t, err)
		assert.NotNil(t, pluginBuilder)

		meta := builder.Build()
		assert.Contains(t, meta.Plugins, "test_plugin")
	})

	t.Run("AddDuplicatePlugin", func(t *testing.T) {
		builder := NewPortalMetaBuilder("example.com")
		_, err := builder.AddPlugin("test_plugin")
		assert.NoError(t, err)

		_, err = builder.AddPlugin("test_plugin")
		assert.Error(t, err)
	})

	t.Run("GetExistingPlugin", func(t *testing.T) {
		builder := NewPortalMetaBuilder("example.com")
		_, err := builder.AddPlugin("test_plugin")
		assert.NoError(t, err)

		pluginBuilder, err := builder.Plugin("test_plugin")
		assert.NoError(t, err)
		assert.NotNil(t, pluginBuilder)

		// Verify we can add meta to the retrieved builder
		pluginBuilder.AddMeta("key", "value")
		meta := builder.Build()
		assert.Equal(t, "value", meta.Plugins["test_plugin"].Meta["key"])
	})

	t.Run("GetNonExistentPlugin", func(t *testing.T) {
		builder := NewPortalMetaBuilder("example.com")
		_, err := builder.Plugin("nonexistent")
		assert.Error(t, err)
		assert.Equal(t, "plugin nonexistent not found in meta", err.Error())
	})
}

func TestPluginMetaBuilder(t *testing.T) {
	t.Run("AddBuildInfo", func(t *testing.T) {
		builder := NewPortalMetaBuilder("example.com")
		pluginBuilder, _ := builder.AddPlugin("test_plugin")

		buildInfo := build.Info{
			Version: "1.0.0",
		}
		pluginBuilder = pluginBuilder.AddBuildInfo(buildInfo)

		meta := builder.Build()
		pluginMeta := meta.Plugins["test_plugin"]
		assert.Equal(t, "1.0.0", pluginMeta.Build.GetVersion())
	})

	t.Run("AddMeta", func(t *testing.T) {
		builder := NewPortalMetaBuilder("example.com")
		pluginBuilder, _ := builder.AddPlugin("test_plugin")

		pluginBuilder = pluginBuilder.AddMeta("key", "value")

		meta := builder.Build()
		assert.Equal(t, "value", meta.Plugins["test_plugin"].Meta["key"])
	})

	t.Run("AddWebBundle", func(t *testing.T) {
		builder := NewPortalMetaBuilder("example.com")
		pluginBuilder, _ := builder.AddPlugin("test_plugin")

		pluginBuilder = pluginBuilder.AddWebBundle("/bundle/path")

		meta := builder.Build()
		assert.Contains(t, meta.Plugins["test_plugin"].WebBundles, "/bundle/path")
	})

	t.Run("Chaining", func(t *testing.T) {
		builder := NewPortalMetaBuilder("example.com")
		pluginBuilder, _ := builder.AddPlugin("test_plugin")

		pluginBuilder.
			AddBuildInfo(build.Info{Version: "1.0.0"}).
			AddMeta("key", "value").
			AddWebBundle("/bundle/path")

		meta := builder.Build()
		pluginMeta := meta.Plugins["test_plugin"]
		assert.Equal(t, "1.0.0", pluginMeta.Build.GetVersion())
		assert.Equal(t, "value", pluginMeta.Meta["key"])
		assert.Contains(t, pluginMeta.WebBundles, "/bundle/path")
	})
}

func TestHTTPServiceIntegration(t *testing.T) {
	t.Run("BuildMetaForPlugins", func(t *testing.T) {
		builder := NewPortalMetaBuilder("example.com")

		// Simulate adding core build info
		builder = builder.AddCoreBuildInfo(build.Info{
			Version: "1.0.0",
		})

		// Simulate adding a plugin
		pluginBuilder, err := builder.AddPlugin("test_plugin")
		assert.NoError(t, err)

		// Simulate adding plugin build info
		pluginBuilder = pluginBuilder.AddBuildInfo(build.Info{
			Version: "2.0.0",
		})

		// Simulate adding web bundles
		pluginBuilder = pluginBuilder.AddWebBundle("/api/meta/plugin/test_plugin/bundle/0/mf-manifest.json")

		// Simulate adding plugin metadata
		pluginBuilder = pluginBuilder.AddMeta("custom", "value")

		meta := builder.Build()

		// Verify the built meta
		assert.Equal(t, "1.0.0", meta.Build.GetVersion())
		assert.Contains(t, meta.Plugins, "test_plugin")
		pluginMeta := meta.Plugins["test_plugin"]
		assert.Equal(t, "2.0.0", pluginMeta.Build.GetVersion())
		assert.Contains(t, pluginMeta.WebBundles, "/api/meta/plugin/test_plugin/bundle/0/mf-manifest.json")
		assert.Equal(t, "value", pluginMeta.Meta["custom"])
	})
}
