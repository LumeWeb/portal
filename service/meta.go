// File: service/meta.go
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
			Plugins:      make(core.PortalMetaPlugins),
			FeatureFlags: make(map[string]bool),
			Build:        build.GetInfo(),
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
	if _, exists := b.meta.Plugins[key]; !exists {
		b.meta.Plugins[key] = core.PortalMetaPlugin{
			Meta: make(map[string]any),
		}
	}
	return b
}

// AddPluginWithBuild adds a plugin with build info
func (b *PortalMetaBuilderDefault) AddPluginWithBuild(key string, buildInfo build.Info) core.PortalMetaBuilder {
	if _, exists := b.meta.Plugins[key]; !exists {
		b.meta.Plugins[key] = core.PortalMetaPlugin{
			Meta:  make(map[string]any),
			Build: buildInfo,
		}
	}
	return b
}

// AddPluginMeta adds or updates meta for a plugin
func (b *PortalMetaBuilderDefault) AddPluginMeta(pluginKey string, metaKey string, metaValue any) core.PortalMetaBuilder {
	if plugin, exists := b.meta.Plugins[pluginKey]; exists {
		plugin.Meta[metaKey] = metaValue
		b.meta.Plugins[pluginKey] = plugin
	}
	return b
}

// Build returns the built PortalMeta
func (b *PortalMetaBuilderDefault) Build() *core.PortalMeta {
	return b.meta
}
