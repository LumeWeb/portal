package core

import (
	"errors"
	"fmt"
	"go.lumeweb.com/portal/build"
	_ "go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core/internal"
	_ "net/http"
	"sync"
)

type PluginFactory func() PluginInfo

type CronFactory func(Context) (Cronable, error)
type MailerTemplates map[string]MailerTemplate

type PluginInfo struct {
	ID              string
	Version         build.BuildInfo
	Meta            func(Context, PortalMetaBuilder) error
	API             func() (API, []ContextBuilderOption, error)
	Protocol        func() (Protocol, []ContextBuilderOption, error)
	Services        func() ([]ServiceInfo, error)
	APIExtensions   func(Context) ([]APIExtensionFactory, error)
	Models          []any
	Migrations      DBMigration
	Events          []Eventer
	Depends         []string
	Cron            func() CronFactory
	MailerTemplates MailerTemplates
	WebBundles      []*WebBundle
	TargetApps      []string
}

type Configurable interface {
	Config() (any, error)
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

	hasComponent := info.API != nil || info.Protocol != nil || info.Services != nil || 
		info.APIExtensions != nil || len(info.WebBundles) > 0
	if !hasComponent {
		panic("plugin must have at least one of API, Protocol, Service, APIExtension, or WebBundle")
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

	plugin, ok := plugins[name]

	if !ok {
		return PluginInfo{}
	}

	return plugin
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
