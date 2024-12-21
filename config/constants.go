package config

const (
	//
	ENV_PREFIX    = "PORTAL__"
	ENV_SEPARATOR = "__"

	// Configuration keys and flags
	CLUSTER_CONFIG_KEY = "config"
	FLAG_SYNC          = "sync"
	FLAG_VOLATILE      = "volatile"

	// File extensions and names
	CONFIG_EXTENSION  = ".yaml"
	CoreConfigFile    = "core" + CONFIG_EXTENSION
	SectionConfigFile = "default" + CONFIG_EXTENSION

	// Directory names
	PluginsDir = "plugins.d"
	ProtoDir   = "proto.d"
	ServiceDir = "service.d"
	APIDir     = "api.d"

	// Section specifiers
	protoSectionSpecifier   = "plugin.%s.protocol"
	apiSectionSpecifier     = "plugin.%s.api"
	serviceSectionSpecifier = "plugin.%s.service.%s"

	// Struct tags
	mapStructureTag = "config"
)

var (
	// Default config directory paths
	configDirPaths = []string{
		"/etc/lumeweb/portal",
		"$HOME/.lumeweb/portal",
		"./portal.yaml",
		"./",
	}
)
