package service

import (
	"fmt"
	"go.lumeweb.com/portal/build"
	"go.lumeweb.com/portal/core"
	"strings"
)

var _ core.PortalMetaBuilder = (*portalMetaBuilder)(nil)
var _ core.PluginMetaBuilder = (*pluginMetaBuilder)(nil)

type portalMetaBuilder struct {
	meta *core.PortalMeta
}

type pluginMetaBuilder struct {
	meta       *core.PortalMeta
	pluginID   string
	pluginMeta core.PortalMetaPlugin
}

func NewPortalMetaBuilder(domain string) core.PortalMetaBuilder {
	return &portalMetaBuilder{
		meta: &core.PortalMeta{
			Domain:       domain,
			Plugins:      make(core.PortalMetaPlugins),
			FeatureFlags: make(map[string]bool),
		},
	}
}

func (b *portalMetaBuilder) AddFeatureFlag(key string, value bool) core.PortalMetaBuilder {
	key = strings.ToUpper(key)
	key = strings.ReplaceAll(key, ".", "_")
	key = strings.ReplaceAll(key, " ", "_")
	key = strings.ReplaceAll(key, "-", "_")
	b.meta.FeatureFlags[key] = value
	return b
}

func (b *portalMetaBuilder) AddCoreBuildInfo(buildInfo build.Info) core.PortalMetaBuilder {
	b.meta.Build = buildInfo
	return b
}

func (b *portalMetaBuilder) AddPlugin(pluginID string) (core.PluginMetaBuilder, error) {
	if _, exists := b.meta.Plugins[pluginID]; exists {
		return nil, fmt.Errorf("plugin %s already exists in meta", pluginID)
	}

	pluginMeta := core.PortalMetaPlugin{
		Meta:       make(map[string]any),
		WebBundles: make([]string, 0),
	}
	b.meta.Plugins[pluginID] = pluginMeta

	return &pluginMetaBuilder{
		meta:       b.meta,
		pluginID:   pluginID,
		pluginMeta: pluginMeta,
	}, nil
}

// Plugin returns the PluginMetaBuilder for a registered plugin.
func (b *portalMetaBuilder) Plugin(pluginID string) (core.PluginMetaBuilder, error) {
	pluginMeta, exists := b.meta.Plugins[pluginID]
	if !exists {
		return nil, fmt.Errorf("plugin %s not found in meta", pluginID)
	}
	
	return &pluginMetaBuilder{
		meta:       b.meta,
		pluginID:   pluginID,
		pluginMeta: pluginMeta,
	}, nil
}

func (b *portalMetaBuilder) Build() *core.PortalMeta {
	return b.meta
}

func (p *pluginMetaBuilder) AddBuildInfo(buildInfo build.Info) core.PluginMetaBuilder {
	p.pluginMeta.Build = buildInfo
	p.meta.Plugins[p.pluginID] = p.pluginMeta
	return p
}

func (p *pluginMetaBuilder) AddMeta(key string, value any) core.PluginMetaBuilder {
	p.pluginMeta.Meta[key] = value
	p.meta.Plugins[p.pluginID] = p.pluginMeta
	return p
}

func (p *pluginMetaBuilder) AddWebBundle(bundleURI string) core.PluginMetaBuilder {
	p.pluginMeta.WebBundles = append(p.pluginMeta.WebBundles, bundleURI)
	p.meta.Plugins[p.pluginID] = p.pluginMeta
	return p
}
