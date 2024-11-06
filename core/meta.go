// File: core/meta.go
package core

import (
	"go.lumeweb.com/portal/build"
)

// PortalMeta represents the portal metadata
type PortalMeta struct {
	Domain       string            `json:"domain"`
	Plugins      PortalMetaPlugins `json:"plugins"`
	FeatureFlags map[string]bool   `json:"feature_flags"`
	Build        build.Info        `json:"build"` // Core build info
}

type PortalMetaPlugins = map[string]PortalMetaPlugin

type PortalMetaPlugin struct {
	Build build.Info     `json:"build,omitempty"` // Plugin build info
	Meta  map[string]any `json:"meta"`
}

// PortalMetaBuilder interface for building portal metadata
type PortalMetaBuilder interface {
	// Core functionality
	AddFeatureFlag(key string, value bool) PortalMetaBuilder

	// Plugin management
	AddPlugin(key string) PortalMetaBuilder
	AddPluginWithBuild(key string, buildInfo build.Info) PortalMetaBuilder
	AddPluginMeta(pluginKey string, metaKey string, metaValue any) PortalMetaBuilder

	Build() *PortalMeta
}
