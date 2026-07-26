package core

import (
	"errors"
	"fmt"

	_ "net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"go.lumeweb.com/portal/build"
	_ "go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core/internal"
)

type PluginFactory func() PluginInfo

type MailerTemplates map[string]MailerTemplate

type MetaFactory func(Context, PortalMetaBuilder) error
type APIFactory func() (API, []ContextBuilderOption, error)
type ProtocolFactory func() (Protocol, []ContextBuilderOption, error)
type ServicesFactory func() ([]ServiceInfo, error)
type APIExtensionsFactory func(Context) ([]APIExtensionFactory, error)

type WorkflowFactory func(Context) ([]WorkflowDefinition, error)

type WorkflowOperationsFactory func(Context) ([]Operation, error)

type PluginCronJob struct {
	Name     string                  // Unique type identifier for the job
	Factory  CronJobFactoryFunc      // Function to create the CronJob instance
	Schedule *CronScheduleDefinition // Default schedule for the job
}

type PluginInfo struct {
	ID                 string
	Version            build.BuildInfo
	Meta               MetaFactory
	API                APIFactory
	Protocol           ProtocolFactory
	Services           ServicesFactory
	APIExtensions      APIExtensionsFactory
	Models             []any
	Migrations         DBMigration
	Depends            []string
	CronJobs           []PluginCronJob
	MailerTemplates    MailerTemplates
	WebBundles         []*WebBundle
	TargetApps         []string
	Operations         WorkflowOperationsFactory
	Workflows          WorkflowFactory
	Metrics            []prometheus.Collector
	KeyIdentityHandlers []KeyIdentityHandlerRegistration
}

type Configurable interface {
	GetConfig() (any, error)
}

var (
	plugins          = make(map[string]PluginInfo)
	pluginsMu        sync.RWMutex
	pluginsOrdered   []PluginInfo
	pluginsOrderedMu sync.RWMutex
)

var (
	ErrInvalidModel = errors.New("model is invalid")
)

func (pi PluginInfo) String() string {
	return pi.ID
}

func RegisterPlugin(info PluginInfo) {
	if info.ID == "" {
		panic("plugin ID must not be empty")
	}

	// Check if any KeyIdentityHandlers are valid (non-nil Handler, non-empty Type)
	hasKeyIdentityHandler := false
	for _, h := range info.KeyIdentityHandlers {
		if h.Handler != nil && h.Type != "" {
			hasKeyIdentityHandler = true
			break
		}
	}

	hasComponent := info.API != nil || info.Protocol != nil || info.Services != nil ||
		info.APIExtensions != nil || len(info.WebBundles) > 0 || len(info.CronJobs) > 0 ||
		hasKeyIdentityHandler
	if !hasComponent {
		panic("plugin must have at least one of API, Protocol, Service, APIExtension, WebBundle, CronJob, or KeyIdentityHandler")
	}

	pluginsMu.Lock()
	defer pluginsMu.Unlock()

	pluginsOrderedMu.Lock()
	defer pluginsOrderedMu.Unlock()

	if _, ok := plugins[info.ID]; ok {
		panic(fmt.Sprintf("plugin already registered: %s", info.ID))
	}

	if pluginsOrdered != nil && len(pluginsOrdered) > 0 {
		pluginsOrdered = make([]PluginInfo, 0)
	}

	plugins[info.ID] = info
}

func UnregisterPlugin(name string) error {
	pluginsMu.Lock()
	defer pluginsMu.Unlock()

	if _, exists := plugins[name]; !exists {
		return fmt.Errorf("plugin %s not found", name)
	}

	delete(plugins, name)
	return nil
}

func GetProtocol(id string) Protocol {
	protocolsMu.RLock()
	defer protocolsMu.RUnlock()

	protocol, ok := protocols[id]

	if !ok {
		return nil
	}

	return protocol
}

func ProtocolExists(id string) bool {
	protocolsMu.RLock()
	defer protocolsMu.RUnlock()

	_, ok := protocols[id]

	return ok
}

func GetPlugin(name string) PluginInfo {
	pluginsMu.RLock()
	defer pluginsMu.RUnlock()
	return plugins[name] // Returns zero-value if not found
}

func PluginExists(name string) bool {
	pluginsMu.RLock()
	defer pluginsMu.RUnlock()
	_, ok := plugins[name]
	return ok
}

func GetPlugins() []PluginInfo {
	pluginsMu.RLock()
	defer pluginsMu.RUnlock()

	pluginsOrderedMu.Lock()
	defer pluginsOrderedMu.Unlock()

	if len(pluginsOrdered) > 0 {
		return pluginsOrdered
	}

	graph := internal.NewDependsGraph()

	for _, k := range plugins {
		graph.AddNode(k.ID, k.Depends...)
	}

	list, err := graph.Build()

	if err != nil {
		panic(err)
	}

	var pluginList []PluginInfo

	for _, k := range list {
		pluginList = append(pluginList, plugins[k])
	}

	pluginsOrdered = pluginList

	return pluginList
}

func PluginHasAPIExtensions(plugin PluginInfo) bool {
	return plugin.APIExtensions != nil
}
