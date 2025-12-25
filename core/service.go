package core

import (
	"fmt"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/samber/lo"
	"go.lumeweb.com/portal/core/internal"
)

type ServiceFactory func() (Service, []ContextBuilderOption, error)

type Service interface {
	Component
}

type ServiceInit interface {
	Init() error
}

var (
	services                 = make(map[string]ServiceInfo)
	servicesOrdered          []ServiceInfo
	servicesMu               sync.RWMutex
	servicesOrderedMu        sync.RWMutex
	pluginServices           = make(map[string][]string)
	pluginServicesMu         sync.RWMutex
	globallyRequiredServices = []string{ACCESS_SERVICE, HTTP_SERVICE}
)

type ServiceInfo struct {
	ID      string
	Factory ServiceFactory
	Depends []string
	Metrics []prometheus.Collector
}

func RegisterServicesFromPlugins() {
	pluginsMu.RLock()
	defer pluginsMu.RUnlock()

	for _, plugin := range plugins {
		if PluginHasServices(plugin) {
			svcs, err := plugin.Services()
			if err != nil {
				panic(fmt.Errorf("plugin %s service factory returned an error: %w", plugin.ID, err))
			}

			for _, svc := range svcs {
				RegisterService(svc, plugin.ID)
			}
		}
	}
}

func PluginHasServices(plugin PluginInfo) bool {
	return plugin.Services != nil
}

func RegisterService(service ServiceInfo, plugin ...string) {
	if service.ID == "" {
		panic("service ID must not be empty")
	}

	if service.Factory == nil {
		panic("service factory must not be nil")
	}

	servicesMu.Lock()
	defer servicesMu.Unlock()

	servicesOrderedMu.Lock()
	defer servicesOrderedMu.Unlock()

	if _, ok := services[service.ID]; ok {
		panic("service already registered: " + service.ID)
	}

	if servicesOrdered != nil && len(servicesOrdered) > 0 {
		servicesOrdered = make([]ServiceInfo, 0)
	}

	services[service.ID] = service

	if len(plugin) > 0 {
		pluginServicesMu.Lock()
		defer pluginServicesMu.Unlock()

		pluginServices[plugin[0]] = append(pluginServices[plugin[0]], service.ID)
	}
}

func UnregisterService(id string) {
	servicesMu.Lock()
	defer servicesMu.Unlock()

	servicesOrderedMu.Lock()
	defer servicesOrderedMu.Unlock()

	if _, ok := services[id]; !ok {
		return
	}

	// Remove from services map
	delete(services, id)

	// Reset ordered services to force rebuild
	if servicesOrdered != nil {
		servicesOrdered = make([]ServiceInfo, 0)
	}

	// Remove from plugin services if registered under a plugin
	pluginServicesMu.Lock()
	defer pluginServicesMu.Unlock()

	for plugin, svcs := range pluginServices {
		newSvcs := make([]string, 0, len(svcs))
		for _, svc := range svcs {
			if svc != id {
				newSvcs = append(newSvcs, svc)
			}
		}
		if len(newSvcs) == 0 {
			delete(pluginServices, plugin)
		} else {
			pluginServices[plugin] = newSvcs
		}
	}
}

func IsCoreService(id string) bool {
	servicesMu.RLock()
	defer servicesMu.RUnlock()

	// First, check if the service is registered at all
	if _, ok := services[id]; !ok {
		return false
	}

	pluginServicesMu.Lock()
	defer pluginServicesMu.Unlock()

	for _, svcs := range pluginServices {
		for _, svc := range svcs {
			if svc == id {
				return false
			}
		}
	}

	return true
}

func GetServiceInfo(id string) *ServiceInfo {
	servicesMu.RLock()
	defer servicesMu.RUnlock()

	svc, ok := services[id]

	if !ok {
		return nil
	}

	return &svc
}

func GetPluginForService(id string) string {
	pluginServicesMu.RLock()
	defer pluginServicesMu.RUnlock()

	for k, v := range pluginServices {
		for _, svc := range v {
			if svc == id {
				return k
			}
		}
	}

	return ""
}

func GetServices() []ServiceInfo {
	servicesMu.RLock()
	defer servicesMu.RUnlock()

	servicesOrderedMu.RLock()
	defer servicesOrderedMu.RUnlock()

	if len(servicesOrdered) > 0 {
		return servicesOrdered
	}

	graph := internal.NewDependsGraph()

	for _, k := range services {
		finalDepends := append([]string(nil), k.Depends...)

		if !lo.Contains(globallyRequiredServices, k.ID) {
			finalDepends = append(finalDepends, globallyRequiredServices...)
			finalDepends = lo.Uniq(finalDepends)
		}

		graph.AddNode(k.ID, finalDepends...)
	}

	list, err := graph.Build()

	if err != nil {
		panic(err)
	}

	var svcList []ServiceInfo

	for _, k := range list {
		svcList = append(svcList, services[k])
	}

	servicesOrdered = svcList

	return svcList
}

// Unsafe_GetServiceMap returns the internal service map for testing.
// This function is intended for testing purposes only and should not be used in production code.
func Unsafe_GetServiceMap() map[string]ServiceInfo {
	return services
}

// Unsafe_GetServiceMapMutex returns the internal service map mutex for testing.
// This function is intended for testing purposes only and should not be used in production code.
func Unsafe_GetServiceMapMutex() *sync.RWMutex {
	return &servicesMu
}

// Unsafe_GetPluginServices returns the internal plugin services map for testing.
// This function is intended for testing purposes only and should not be used in production code.
func Unsafe_GetPluginServices() map[string][]string {
	return pluginServices
}

// Unsafe_GetPluginServicesMutex returns the internal plugin services mutex for testing.
// This function is intended for testing purposes only and should not be used in production code.
func Unsafe_GetPluginServicesMutex() *sync.RWMutex {
	return &pluginServicesMu
}

func ResetServices() {
	servicesMu.Lock()
	defer servicesMu.Unlock()
	services = make(map[string]ServiceInfo)

	servicesOrderedMu.Lock()
	defer servicesOrderedMu.Unlock()
	servicesOrdered = nil

	pluginServicesMu.Lock()
	defer pluginServicesMu.Unlock()
	pluginServices = make(map[string][]string)
}
