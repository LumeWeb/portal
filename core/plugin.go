package core

import (
	"errors"
	"fmt"
	_ "github.com/gorilla/mux"
	_ "go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core/internal"
	"gorm.io/gorm"
	_ "net/http"
	"sync"
)

type PluginFactory func() PluginInfo

type CronFactory func(Context) (Cronable, error)
type MailerTemplates map[string]MailerTemplate

type DBMigration func(*gorm.DB) error

type PluginInfo struct {
	ID              string
	API             func() (API, []ContextBuilderOption, error)
	Protocol        func() (Protocol, []ContextBuilderOption, error)
	Services        func() ([]ServiceInfo, error)
	Models          []any
	Migrations      []DBMigration
	Events          []Eventer
	Depends         []string
	Cron            func() CronFactory
	MailerTemplates MailerTemplates
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

	if info.API == nil && info.Protocol == nil && info.Services == nil {
		panic("plugin must have at least one of GetAPI, GetProtocol, or GetServices")
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
