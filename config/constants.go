package config

import (
	"strings"
)

const (
	//
	ENV_PREFIX    = "PORTAL__"
	ENV_SEPARATOR = "__"
	// Environment variable names
	ENV_CONFIG_PATHS = ENV_PREFIX + "CONFIG_PATHS"
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
	ProtocolSpecifier = "plugin.%s.protocol"
	APISpecifier      = "plugin.%s.api"
	ServiceSpecifier  = "plugin.%s.service.%s"
)

// EnvVarFor converts a dotted config key (e.g. "core.storage.sia.url")
// into its corresponding environment variable name (e.g.
// "PORTAL__CORE__STORAGE__SIA__URL").
func EnvVarFor(key string) string {
	return ENV_PREFIX + strings.ToUpper(strings.ReplaceAll(key, ".", ENV_SEPARATOR))
}

var (
	// DefaultConfigPaths defines the default search paths for configuration files
	DefaultConfigPaths = getDefaultConfigPaths()
)

func getDefaultConfigPaths() []string {
	return []string{
		"/etc/lumeweb/portal",
		"$HOME/.lumeweb/portal",
		"./",
	}
}
