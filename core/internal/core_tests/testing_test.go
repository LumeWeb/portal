package core_tests

import (
	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"go.lumeweb.com/portal/build"
	"go.lumeweb.com/portal/config"
	. "go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/testing/mocks"
	"testing"
	"time"
)

// MockServiceConfig is a mock implementation of config.ServiceConfig for testing.
type MockServiceConfig struct{}

func (m MockServiceConfig) Defaults() map[string]any {
	return map[string]any{}
}

var mockServiceConfig = MockServiceConfig{}

type MockProtocolConfig struct{}

func (m MockProtocolConfig) Defaults() map[string]any {
	return map[string]any{}
}

var mockProtocolConfig = MockProtocolConfig{}

type MockAPIConfig struct{}

func (m MockAPIConfig) Defaults() map[string]any {
	return map[string]any{}
}

var mockAPIConfig = MockAPIConfig{}

// Helper function to create a mock ConfigManager with required expectations
func newMockConfigManager(t *testing.T, cfg *config.Config) *config.MockManager {
	mockConfigManager := config.NewMockManager(t)
	mockConfigManager.On("GetConfig").Return(cfg)
	mockConfigManager.On("SetLogger", mock.Anything).Return()
	return mockConfigManager
}

// Helper function to create a minimal PluginInfo for testing
func newTestPluginInfo(id string, depends ...string) PluginInfo {
	return PluginInfo{
		ID:      id,
		Version: build.New("test-version", "", "", "", "", "", ""),
		// Add a minimal component to satisfy the hasComponent check
		WebBundles: []*WebBundle{{}}, // Added this line
		Depends:    depends,
	}
}

// Helper function to create a PluginInfo with a specific component
func newTestPluginInfoWithComponent(t *testing.T, id string, componentType string, depends ...string) PluginInfo {
	info := PluginInfo{
		ID:      id,
		Version: build.New("test-version", "", "", "", "", "", ""),
		Depends: depends,
	}
	switch componentType {
	case "API":
		info.API = func() (API, []ContextBuilderOption, error) {
			mockAPI := mocks.NewMockAPI(t)
			// Minimal mocks to satisfy the interface
			mockAPI.On("Name").Return(id + "-api")
			// Removed expectations for Subdomain, AuthTokenName, GetConfig, OpenAPIInfo, Configure
			return mockAPI, nil, nil
		}
	case "Protocol":
		info.Protocol = func() (Protocol, []ContextBuilderOption, error) {
			mockProtocol := mocks.NewMockProtocol(nil)
			// Minimal mocks to satisfy the interface
			mockProtocol.On("Name").Return(id + "-protocol")
			mockProtocol.On("GetConfig").Return(nil)
			mockProtocol.On("Operations").Return(nil)
			return mockProtocol, nil, nil
		}
	case "Services":
		info.Services = func() ([]ServiceInfo, error) {
			return []ServiceInfo{{ID: id + "-svc", Factory: func() (Service, []ContextBuilderOption, error) { return mocks.NewMockService(t), nil, nil }}}, nil
		}
	case "APIExtensions":
		info.APIExtensions = func(ctx Context) ([]APIExtensionFactory, error) {
			return []APIExtensionFactory{
				func() (APIExtension, []ContextBuilderOption, error) {
					mockExt := mocks.NewMockAPIExtension(t)
					// Minimal mocks to satisfy the interface
					mockExt.On("TargetAPI").Return("some-api")
					mockExt.On("Configure", mock.Anything, mock.Anything).Return(nil) // Use mock.Anything for router and accessSvc
					return mockExt, nil, nil
				},
			}, nil
		}
	case "WebBundles":
		info.WebBundles = []*WebBundle{{}} // Minimal valid WebBundle
	case "Cron":
		// Add a test cron job
		jobName := "test-job"
		info.CronJobs = []PluginCronJob{
			{
				Name: jobName,
				Factory: func() (CronJob, error) {
					return NewBaseCronJob(
						uuid.New(),
						JobOriginPlugin,
						id,
						"Test Job",
						&CronScheduleDefinition{
							Type:     CronScheduleTypeDaily,
							Interval: 1,
							AtTime:   time.Now().Add(time.Hour),
						},
						nil,
						"", // jobType computed from origin and sourceID
					), nil
				},
				Schedule: &CronScheduleDefinition{
					Type:     CronScheduleTypeDaily,
					Interval: 1,
					AtTime:   time.Now().Add(time.Hour),
				},
			},
		}
	default:
		t.Fatalf("unknown component type: %s", componentType)
	}
	return info
}

// Helper function to create a minimal ServiceInfo for testing
func newTestServiceInfo(t *testing.T, id string, depends ...string) ServiceInfo {
	return ServiceInfo{
		ID: id,
		Factory: func() (Service, []ContextBuilderOption, error) {
			return mocks.NewMockService(t), nil, nil
		},
		Depends: depends,
	}
}
