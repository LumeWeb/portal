package core_tests

import (
	"errors"
	core "go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/testing/mocks"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegisterService(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	service := newTestServiceInfo(t, "test-service")
	core.RegisterService(service)

	retrievedService := core.GetServiceInfo("test-service")
	assert.NotNil(t, retrievedService)
	assert.Equal(t, service.ID, retrievedService.ID)
}

func TestRegisterService_WithPluginID(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	service := newTestServiceInfo(t, "test-plugin-service")
	pluginID := "test-plugin"
	core.RegisterService(service, pluginID)

	retrievedService := core.GetServiceInfo("test-plugin-service")
	assert.NotNil(t, retrievedService)
	assert.Equal(t, service.ID, retrievedService.ID)

	// Use the unsafe helper to check the plugin ID
	pluginServicesMap := core.Unsafe_GetPluginServices()
	pluginServicesMutex := core.Unsafe_GetPluginServicesMutex()

	pluginServicesMutex.RLock()
	defer pluginServicesMutex.RUnlock()

	servicesForPlugin, ok := pluginServicesMap[pluginID]
	assert.True(t, ok, "Plugin ID should exist in pluginServices map")
	assert.Contains(t, servicesForPlugin, service.ID, "Service ID should be associated with the plugin ID")
}

func TestRegisterService_DuplicateID(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	service1 := newTestServiceInfo(t, "duplicate-service")
	core.RegisterService(service1)

	service2 := newTestServiceInfo(t, "duplicate-service") // Same ID

	assert.PanicsWithValue(t, "service already registered: duplicate-service", func() {
		core.RegisterService(service2)
	})
}

func TestRegisterService_EmptyID(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	service := core.ServiceInfo{
		ID: "",
		Factory: func() (core.Service, []core.ContextBuilderOption, error) {
			return mocks.NewMockService(t), nil, nil
		},
	}

	assert.PanicsWithValue(t, "service ID must not be empty", func() {
		core.RegisterService(service)
	})
}

func TestRegisterService_NilFactory(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	service := core.ServiceInfo{
		ID:      "nil-factory-service",
		Factory: nil,
	}

	assert.PanicsWithValue(t, "service factory must not be nil", func() {
		core.RegisterService(service)
	})
}

func TestGetServiceInfo(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	service := newTestServiceInfo(t, "get-service")
	core.RegisterService(service)

	retrievedService := core.GetServiceInfo("get-service")
	assert.NotNil(t, retrievedService)
	assert.Equal(t, service.ID, retrievedService.ID)
}

func TestGetServiceInfo_NotFound(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	retrievedService := core.GetServiceInfo("non-existent-service")
	assert.Nil(t, retrievedService) // Should return nil
}

func TestGetServices_Ordering(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	// Register dummy globally required services for this test
	core.RegisterService(newTestServiceInfo(t, core.ACCESS_SERVICE))
	core.RegisterService(newTestServiceInfo(t, core.HTTP_SERVICE))

	// Services with dependencies
	serviceA := newTestServiceInfo(t, "service-a")
	serviceB := newTestServiceInfo(t, "service-b", "service-a")
	serviceC := newTestServiceInfo(t, "service-c", "service-b")
	serviceD := newTestServiceInfo(t, "service-d") // No dependencies

	core.RegisterService(serviceC) // Register out of order
	core.RegisterService(serviceA)
	core.RegisterService(serviceD)
	core.RegisterService(serviceB)

	services := core.GetServices()

	// Expected order: service-a, service-d, service-b, service-c (or service-d, service-a, service-b, service-c)
	// The graph build ensures dependencies come first. Order of independent nodes can vary.
	// We need to check the relative order of dependent nodes.
	assert.Len(t, services, 6) // Including the two dummy services

	// Find indices
	idxA := -1
	idxB := -1
	idxC := -1
	idxD := -1
	idxAccess := -1
	idxHTTP := -1
	for i, s := range services {
		switch s.ID {
		case "service-a":
			idxA = i
		case "service-b":
			idxB = i
		case "service-c":
			idxC = i
		case "service-d":
			idxD = i
		case core.ACCESS_SERVICE:
			idxAccess = i
		case core.HTTP_SERVICE:
			idxHTTP = i
		}
	}

	assert.True(t, idxA != -1 && idxB != -1 && idxC != -1 && idxD != -1 && idxAccess != -1 && idxHTTP != -1, "All services should be in the list")
	assert.Less(t, idxA, idxB, "service-a should come before service-b")
	assert.Less(t, idxB, idxC, "service-b should come before service-c")

	// Globally required services should come before services that depend on them
	assert.Less(t, idxAccess, idxA, "ACCESS_SERVICE should come before service-a")
	assert.Less(t, idxHTTP, idxA, "HTTP_SERVICE should come before service-a")
	assert.Less(t, idxAccess, idxB, "ACCESS_SERVICE should come before service-b")
	assert.Less(t, idxHTTP, idxB, "HTTP_SERVICE should come before service-b")
	assert.Less(t, idxAccess, idxC, "ACCESS_SERVICE should come before service-c")
	assert.Less(t, idxHTTP, idxC, "HTTP_SERVICE should come before service-c")
	assert.Less(t, idxAccess, idxD, "ACCESS_SERVICE should come before service-d")
	assert.Less(t, idxHTTP, idxD, "HTTP_SERVICE should come before service-d")
}

func TestGetServices_CycleDetection(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	// Register dummy globally required services for this test
	core.RegisterService(newTestServiceInfo(t, core.ACCESS_SERVICE))
	core.RegisterService(newTestServiceInfo(t, core.HTTP_SERVICE))

	serviceA := newTestServiceInfo(t, "service-a", "service-c") // Depends on C
	serviceB := newTestServiceInfo(t, "service-b", "service-a") // Depends on A
	serviceC := newTestServiceInfo(t, "service-c", "service-b") // Depends on B (Cycle: A -> B -> C -> A)

	core.RegisterService(serviceA)
	core.RegisterService(serviceB)
	core.RegisterService(serviceC)

	assert.Panics(t, func() {
		core.GetServices()
	}, "Should panic on dependency cycle")

	// Check the panic message contains cycle information
	defer func() {
		if r := recover(); r != nil {
			err, ok := r.(error)
			assert.True(t, ok, "Panic value should be an error")
			assert.Contains(t, err.Error(), "cycle detected", "Panic message should indicate a cycle")
		}
	}()

	core.GetServices() // This call should panic
}

func TestGetServices_NoServices(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	// Register dummy globally required services for this test
	core.RegisterService(newTestServiceInfo(t, core.ACCESS_SERVICE))
	core.RegisterService(newTestServiceInfo(t, core.HTTP_SERVICE))

	services := core.GetServices()
	assert.Len(t, services, 2) // Only the two dummy services should be present

	// Check that both globally required services are present, order doesn't matter
	serviceIDs := []string{services[0].ID, services[1].ID}
	assert.Contains(t, serviceIDs, core.ACCESS_SERVICE)
	assert.Contains(t, serviceIDs, core.HTTP_SERVICE)
}

func TestIsCoreService(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	// Register a core service
	coreService := newTestServiceInfo(t, "core-service")
	core.RegisterService(coreService) // No plugin ID provided, should be core

	// Register a plugin service
	plugin := newTestPluginInfoWithComponent(t, "test-plugin", "Services")
	core.RegisterPlugin(plugin)
	// Manually register the service from the plugin for this test, providing the plugin ID
	pluginServices, _ := plugin.Services()
	core.RegisterService(pluginServices[0], plugin.ID)

	assert.True(t, core.IsCoreService("core-service"))
	assert.False(t, core.IsCoreService("test-plugin-svc")) // Assuming the plugin service ID is "test-plugin-svc"
	assert.False(t, core.IsCoreService("non-existent-service"))
}

func TestGetPluginForService(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	// Register a core service
	coreService := newTestServiceInfo(t, "core-service")
	core.RegisterService(coreService) // No plugin ID provided, should be core

	// Register a plugin service
	plugin := newTestPluginInfoWithComponent(t, "test-plugin", "Services")
	core.RegisterPlugin(plugin)
	// Manually register the service from the plugin for this test, providing the plugin ID
	pluginServices, _ := plugin.Services()
	core.RegisterService(pluginServices[0], plugin.ID)

	assert.Equal(t, "", core.GetPluginForService("core-service"))
	assert.Equal(t, "test-plugin", core.GetPluginForService("test-plugin-svc")) // Assuming the plugin service ID is "test-plugin-svc"
	assert.Equal(t, "", core.GetPluginForService("non-existent-service"))
}

func TestUnregisterService(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	// Register a service
	service := newTestServiceInfo(t, "test-service")
	core.RegisterService(service)

	// Register a plugin service 
	plugin := newTestPluginInfoWithComponent(t, "test-plugin", "Services")
	core.RegisterPlugin(plugin)
	pluginServices, _ := plugin.Services()
	core.RegisterService(pluginServices[0], plugin.ID)

	// Verify services exist
	assert.NotNil(t, core.GetServiceInfo("test-service"))
	assert.NotNil(t, core.GetServiceInfo("test-plugin-svc"))
	assert.Equal(t, "test-plugin", core.GetPluginForService("test-plugin-svc"))

	// Unregister the services
	core.UnregisterService("test-service")
	core.UnregisterService("test-plugin-svc")

	// Verify services no longer exist
	assert.Nil(t, core.GetServiceInfo("test-service"))
	assert.Nil(t, core.GetServiceInfo("test-plugin-svc"))
	assert.Equal(t, "", core.GetPluginForService("test-plugin-svc"))

	// Verify plugin services map is cleaned up
	pluginServicesMap := core.Unsafe_GetPluginServices()
	pluginServicesMutex := core.Unsafe_GetPluginServicesMutex()

	pluginServicesMutex.RLock()
	defer pluginServicesMutex.RUnlock()
	
	servicesForPlugin, ok := pluginServicesMap["test-plugin"]
	assert.False(t, ok || len(servicesForPlugin) > 0, "Plugin services map should be empty after unregister")
}

func TestUnregisterService_NotFound(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	// Try to unregister non-existent service - should not panic
	assert.NotPanics(t, func() {
		core.UnregisterService("non-existent-service")
	})
}

func TestResetServices(t *testing.T) {
	// Register some test services
	service1 := newTestServiceInfo(t, "test-service-1")
	service2 := newTestServiceInfo(t, "test-service-2")
	core.RegisterService(service1)
	core.RegisterService(service2, "test-plugin")

	// Check services exist
	assert.NotNil(t, core.GetServiceInfo("test-service-1"))
	assert.NotNil(t, core.GetServiceInfo("test-service-2"))
	assert.Equal(t, "test-plugin", core.GetPluginForService("test-service-2"))

	// Reset services
	core.ResetServices()

	// Check services no longer exist
	assert.Nil(t, core.GetServiceInfo("test-service-1"))
	assert.Nil(t, core.GetServiceInfo("test-service-2"))
	assert.Empty(t, core.Unsafe_GetServiceMap())
	assert.Empty(t, core.Unsafe_GetPluginServices())
}

func TestRegisterServicesFromPlugins(t *testing.T) {
	t.Run("Successful Registration", func(t *testing.T) {
		core.ResetState()
		defer core.ResetState()

		// Register a plugin with services
		plugin := newTestPluginInfoWithComponent(t, "plugin-with-services", "Services")
		core.RegisterPlugin(plugin)

		// Register another plugin without services
		pluginWithoutServices := newTestPluginInfo("plugin-without-services")
		core.RegisterPlugin(pluginWithoutServices)

		// Register a plugin whose Services factory returns nil slice but no error
		pluginWithNilServices := newTestPluginInfo("plugin-with-nil-services")
		pluginWithNilServices.Services = func() ([]core.ServiceInfo, error) {
			return nil, nil
		}
		core.RegisterPlugin(pluginWithNilServices)

		// Register a plugin whose Services factory returns an empty slice but no error
		pluginWithEmptyServices := newTestPluginInfo("plugin-with-empty-services")
		pluginWithEmptyServices.Services = func() ([]core.ServiceInfo, error) {
			return []core.ServiceInfo{}, nil
		}
		core.RegisterPlugin(pluginWithEmptyServices)

		// Register a plugin with a service that has a dependency on a service from another plugin
		pluginWithDependentService := newTestPluginInfo("plugin-with-dependent-service")
		pluginWithDependentService.Services = func() ([]core.ServiceInfo, error) {
			return []core.ServiceInfo{
				newTestServiceInfo(t, "dependent-service", "plugin-with-services-svc"),
			}, nil
		}
		core.RegisterPlugin(pluginWithDependentService)

		// Register the services from plugins
		assert.NotPanics(t, func() {
			core.RegisterServicesFromPlugins()
		}, "RegisterServicesFromPlugins should not panic on successful registration")

		// Check if the service from the first plugin is registered
		serviceFromPlugin := core.GetServiceInfo("plugin-with-services-svc")
		assert.NotNil(t, serviceFromPlugin)
		assert.Equal(t, "plugin-with-services-svc", serviceFromPlugin.ID)
		assert.Equal(t, "plugin-with-services", core.GetPluginForService("plugin-with-services-svc")) // Check plugin ID is set

		// Check if the dependent service is registered
		dependentService := core.GetServiceInfo("dependent-service")
		assert.NotNil(t, dependentService)
		assert.Equal(t, "dependent-service", dependentService.ID)
		assert.Contains(t, dependentService.Depends, "plugin-with-services-svc")
		assert.Equal(t, "plugin-with-dependent-service", core.GetPluginForService("dependent-service")) // Check plugin ID is set

		// Check that the service from the plugin without services is not registered (as expected)
		serviceFromPluginWithoutServices := core.GetServiceInfo("plugin-without-services-svc")
		assert.Nil(t, serviceFromPluginWithoutServices)

		// Check that the services from the plugins with nil or empty service slices are not registered
		serviceFromNilServicesPlugin := core.GetServiceInfo("plugin-with-nil-services-svc")
		assert.Nil(t, serviceFromNilServicesPlugin)
		serviceFromEmptyServicesPlugin := core.GetServiceInfo("plugin-with-empty-services-svc")
		assert.Nil(t, serviceFromEmptyServicesPlugin)
	})

	t.Run("Service Factory Error", func(t *testing.T) {
		core.ResetState()
		defer core.ResetState()

		// Register a plugin whose Services factory returns an error
		pluginWithError := newTestPluginInfo("plugin-with-service-error")
		pluginWithError.Services = func() ([]core.ServiceInfo, error) {
			return nil, errors.New("failed to get services")
		}
		core.RegisterPlugin(pluginWithError)

		// Test the error case
		assert.Panics(t, func() {
			core.RegisterServicesFromPlugins()
		}, "RegisterServicesFromPlugins should panic on service factory error")

		// Check the panic message contains the error information
		defer func() {
			if r := recover(); r != nil {
				err, ok := r.(error)
				assert.True(t, ok, "Panic value should be an error")
				assert.Contains(t, err.Error(), "plugin plugin-with-service-error service factory returned an error: failed to get services", "Panic message should indicate the service factory error")
			}
		}()

		core.RegisterServicesFromPlugins() // This call should panic
	})
}
