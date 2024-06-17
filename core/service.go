package core

import (
	"go.lumeweb.com/portal/core/internal"
	"sync"
)

type ServiceFactory func() (Service, []ContextBuilderOption, error)

type Service interface{}

var (
	services          = make(map[string]ServiceInfo)
	servicesOrdered   []ServiceInfo
	servicesMu        sync.RWMutex
	servicesOrderedMu sync.RWMutex
)

type ServiceInfo struct {
	ID      string
	Factory ServiceFactory
	Depends []string
}

func RegisterServicesFromPlugins() {
	pluginsMu.RLock()
	defer pluginsMu.RUnlock()

	for _, plugin := range plugins {
		if PluginHasServices(plugin) {
			svcs, err := plugin.Services()
			if err != nil {
				panic(err)
			}

			for _, svc := range svcs {
				RegisterService(svc)
			}
		}
	}
}

func PluginHasServices(plugin PluginInfo) bool {
	return plugin.Services != nil
}

func RegisterService(service ServiceInfo) {
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
		graph.AddNode(k.ID, k.Depends...)
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
