package core

import (
	"go.lumeweb.com/portal-router"
	"sync"
)

type APIExtensionFactory func() (APIExtension, []ContextBuilderOption, error)

// APIExtension defines how plugins can extend existing APIs
type APIExtension interface {
	// TargetAPI returns the name of the API this extension targets
	TargetAPI() string

	// Configure is called after the main API routes are registered
	Configure(router router.Router, accessSvc AccessService) error
}

var (
	apiExtensions   = make(map[string][]APIExtension) // map[apiName][]APIExtension
	apiExtensionsMu sync.RWMutex
)

// RegisterAPIExtension registers an extension for an API
func RegisterAPIExtension(ext APIExtension) {
	apiExtensionsMu.Lock()
	defer apiExtensionsMu.Unlock()

	target := ext.TargetAPI()
	apiExtensions[target] = append(apiExtensions[target], ext)
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
