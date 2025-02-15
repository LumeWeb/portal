package service

import (
	"go.lumeweb.com/portal/build"
	"go.lumeweb.com/portal/core"
	"strings"
)

var _ core.PortalMetaBuilder = (*PortalMetaBuilderDefault)(nil)

// PortalMetaBuilderDefault implements core.PortalMetaBuilder
type PortalMetaBuilderDefault struct {
	meta *core.PortalMeta
}

// NewPortalMetaBuilder creates a new PortalMetaBuilderDefault
func NewPortalMetaBuilder(domain string) *PortalMetaBuilderDefault {
	return &PortalMetaBuilderDefault{
		meta: &core.PortalMeta{
			Domain:       domain,
			Build:        build.GetInfo(),
			Plugins:      make(core.PortalMetaPlugins),
			FeatureFlags: make(map[string]bool),
		},
	}
}

// AddFeatureFlag adds a feature flag
func (b *PortalMetaBuilderDefault) AddFeatureFlag(key string, value bool) core.PortalMetaBuilder {
	key = strings.ToUpper(key)
	key = strings.ReplaceAll(key, ".", "_")
	key = strings.ReplaceAll(key, " ", "_")
	key = strings.ReplaceAll(key, "-", "_")
	b.meta.FeatureFlags[key] = value
	return b
}

// AddPlugin adds a plugin without build info
func (b *PortalMetaBuilderDefault) AddPlugin(key string) core.PortalMetaBuilder {
	return b.maybeAddPlugin(key, core.PortalMetaPlugin{
		Meta: make(map[string]any),
	})
}

// AddPluginWithBuild adds a plugin with build info
func (b *PortalMetaBuilderDefault) AddPluginBuildInfo(key string, buildInfo build.Info) core.PortalMetaBuilder {
	return b.maybeUpdatePlugin(key, func(pluginData core.PortalMetaPlugin) core.PortalMetaPlugin {
		pluginData.Meta[key] = buildInfo
		return pluginData
	})
}

func (b *PortalMetaBuilderDefault) maybeAddPlugin(key string, pluginData core.PortalMetaPlugin) core.PortalMetaBuilder {
	if _, exists := b.meta.Plugins[key]; !exists {
		b.meta.Plugins[key] = pluginData
	}
	return b
}

func (b *PortalMetaBuilderDefault) maybeUpdatePlugin(key string, cb func(pluginData core.PortalMetaPlugin) core.PortalMetaPlugin) core.PortalMetaBuilder {
	if _, exists := b.meta.Plugins[key]; exists {
		b.meta.Plugins[key] = cb(b.meta.Plugins[key])
	}
	return b
}

// AddPluginMeta adds or updates meta for a plugin
func (b *PortalMetaBuilderDefault) AddPluginMeta(pluginKey string, metaKey string, metaValue any) core.PortalMetaBuilder {
	return b.maybeUpdatePlugin(pluginKey, func(pluginData core.PortalMetaPlugin) core.PortalMetaPlugin {
		pluginData.Meta[metaKey] = metaValue
		return pluginData
	})
}

func (b *PortalMetaBuilderDefault) AddPluginWebBundle(pluginKey string, bundleUri string) core.PortalMetaBuilder {
	return b.maybeUpdatePlugin(pluginKey, func(pluginData core.PortalMetaPlugin) core.PortalMetaPlugin {
		pluginData.WebBundles = append(pluginData.WebBundles, bundleUri)
		return pluginData
	})
}

// Build returns the built PortalMeta
func (b *PortalMetaBuilderDefault) Build() *core.PortalMeta {
	return b.meta
}
