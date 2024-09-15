package service

import (
	"go.lumeweb.com/portal/core"
	"strings"
)

var _ core.PortalMetaBuilder = (*PortalMetaBuilderDefault)(nil)

// PortalMetaBuilderDefault is a builder for PortalMeta
type PortalMetaBuilderDefault struct {
	portalMeta core.PortalMeta
	plugins    core.PortalMetaPlugins
}

// NewPortalMetaBuilder creates a new PortalMetaBuilderDefault
func NewPortalMetaBuilder(domain string) *PortalMetaBuilderDefault {
	return &PortalMetaBuilderDefault{
		portalMeta: core.PortalMeta{
			Domain:       domain,
			FeatureFlags: make(map[string]bool),
		},
		plugins: core.PortalMetaPlugins{
			Plugins: make(map[string]core.PortalMetaPlugin),
		},
	}
}

// AddFeatureFlag adds a feature flag
func (b *PortalMetaBuilderDefault) AddFeatureFlag(key string, value bool) core.PortalMetaBuilder {
	key = strings.ToUpper(key)
	key = strings.ReplaceAll(key, ".", "_")
	key = strings.ReplaceAll(key, " ", "_")
	key = strings.ReplaceAll(key, "-", "_")
	b.portalMeta.FeatureFlags[key] = value
	return b
}

// AddPlugin adds a plugin without meta
func (b *PortalMetaBuilderDefault) AddPlugin(key string) core.PortalMetaBuilder {
	if _, exists := b.portalMeta.Plugins.Plugins[key]; !exists {
		b.portalMeta.Plugins.Plugins[key] = core.PortalMetaPlugin{Meta: make(map[string]any)}
	}
	return b
}

// AddPluginMeta adds or updates meta for a plugin
func (b *PortalMetaBuilderDefault) AddPluginMeta(pluginKey string, metaKey string, metaValue any) core.PortalMetaBuilder {
	b.AddPlugin(pluginKey) // Ensure the plugin exists
	b.portalMeta.Plugins.Plugins[pluginKey].Meta[metaKey] = metaValue
	return b
}

// Build returns the built PortalMeta and PortalMetaPlugins
func (b *PortalMetaBuilderDefault) Build() *core.PortalMeta {
	return &b.portalMeta
}
