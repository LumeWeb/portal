package core

type PortalMeta struct {
	Domain       string            `json:"domain"`
	Plugins      PortalMetaPlugins `json:"plugins"`
	FeatureFlags map[string]bool   `json:"feature_flags"`
}

type PortalMetaPlugins struct {
	Plugins map[string]PortalMetaPlugin `json:"plugins"`
}

type PortalMetaPlugin struct {
	Meta map[string]any `json:"meta"`
}

type PortalMetaBuilder interface {
	// AddFeatureFlag adds a feature flag
	AddFeatureFlag(key string, value bool) PortalMetaBuilder

	// AddPlugin adds a plugin without meta
	AddPlugin(key string) PortalMetaBuilder

	// AddPluginMeta adds or updates meta for a plugin
	AddPluginMeta(pluginKey string, metaKey string, metaValue any) PortalMetaBuilder

	// Build returns the built PortalMeta
	Build() *PortalMeta
}
