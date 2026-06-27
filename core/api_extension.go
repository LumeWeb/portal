package core

import (
	"github.com/prometheus/client_golang/prometheus"
	"go.lumeweb.com/portal-router"
	"sync"
)

type APIExtensionFactory func() (APIExtension, []ContextBuilderOption, error)

// APIExtension defines how plugins can extend existing APIs
type APIExtension interface {
	Component
	// TargetAPI returns the name of the API this extension targets
	TargetAPI() string

	// Configure is called after the main API routes are registered
	Configure(router router.Router, accessSvc AccessService) error
}

// APIExtensionMetrics is an optional interface that API extensions can implement
// to declare metrics that should be registered on the target API's subdomain.
// When implemented, these metrics are registered into PluginMetricsRegistry(TargetAPI())
// and served on that API's /metrics endpoint.
type APIExtensionMetrics interface {
	APIExtension
	Metrics() []prometheus.Collector
}

var (
	apiExtensions   = make(map[string][]APIExtension) // map[apiName][]APIExtension
	apiExtensionsMu sync.RWMutex
)

// registerAPIExtension is a private helper that implements the core registration logic
// with proper mutex handling for appending to the extension slice map.
// This wrapper exists to allow for future validation or logging extensions if needed.
func registerAPIExtension(ext APIExtension) {
	apiExtensionsMu.Lock()
	defer apiExtensionsMu.Unlock()

	target := ext.TargetAPI()
	apiExtensions[target] = append(apiExtensions[target], ext)
}

// RegisterAPIExtension registers an extension for an API
func RegisterAPIExtension(ext APIExtension) {
	registerAPIExtension(ext)
}

// GetAPIExtensions returns all extensions for a given API
func GetAPIExtensions(apiName string) []APIExtension {
	apiExtensionsMu.RLock()
	defer apiExtensionsMu.RUnlock()

	exts, ok := apiExtensions[apiName]
	if !ok {
		return nil
	}
	return exts
}
