package core

import (
	"errors"
	"fmt"
	"github.com/LumeWeb/portal/config"
	gorilla "github.com/gorilla/mux"
	"net/http"
	"sort"
	"sync"
)

type PluginFactory func() PluginInfo

type PluginInfo struct {
	ID          string
	GetAPI      func(*Context) (API, error)
	GetProtocol func(*Context) (Protocol, error)
	Models      []any
}

var (
	plugins     = make(map[string]PluginInfo)
	pluginsMu   sync.RWMutex
	apis        = make(map[string]API)
	apisMu      sync.RWMutex
	protocols   = make(map[string]Protocol)
	protocolsMu sync.RWMutex
)

var (
	ErrInvalidModel = errors.New("model is invalid")
)

func (pi PluginInfo) String() string {
	return pi.ID
}

func RegisterPlugin(factory PluginFactory) {
	info := factory()

	if info.ID == "" {
		panic("plugin ID must not be empty")
	}

	if info.GetAPI == nil && info.GetProtocol == nil {
		panic("plugin must have at least one of GetAPI or GetProtocol")
	}

	pluginsMu.Lock()
	defer pluginsMu.Unlock()

	if _, ok := plugins[info.ID]; ok {
		panic(fmt.Sprintf("plugin already registered: %s", info.ID))
	}

	plugins[info.ID] = info
}

func RegisterAPI(id string, api API) {
	apisMu.Lock()
	defer apisMu.Unlock()

	apis[id] = api
}

func RegisterProtocol(id string, protocol Protocol) {
	protocolsMu.Lock()
	defer protocolsMu.Unlock()

	protocols[id] = protocol
}

func GetAPI(id string) (API, error) {
	apisMu.RLock()
	defer apisMu.RUnlock()

	api, ok := apis[id]

	if !ok {
		return nil, fmt.Errorf("api not found: %s", id)
	}

	return api, nil
}

func GetProtocol(id string) (Protocol, error) {
	protocolsMu.RLock()
	defer protocolsMu.RUnlock()

	protocol, ok := protocols[id]

	if !ok {
		return nil, fmt.Errorf("protocol not found: %s", id)
	}

	return protocol, nil
}

func GetPlugin(name string) (PluginInfo, error) {
	pluginsMu.RLock()
	defer pluginsMu.RUnlock()

	plugin, ok := plugins[name]

	if !ok {
		return PluginInfo{}, fmt.Errorf("plugin not found: %s", name)
	}

	return plugin, nil
}

func GetPlugins() []PluginInfo {
	pluginsMu.RLock()
	defer pluginsMu.RUnlock()

	var pluginList []PluginInfo
	for _, plugin := range plugins {
		pluginList = append(pluginList, plugin)
	}

	// make return value deterministic
	sort.Slice(pluginList, func(i, j int) bool {
		return pluginList[i].ID < pluginList[j].ID
	})

	return pluginList
}

func GetProtocols() []Protocol {
	protocolsMu.RLock()
	defer protocolsMu.RUnlock()

	var protocolList []Protocol
	for _, protocol := range protocols {
		protocolList = append(protocolList, protocol)
	}

	return protocolList
}

func GetAPIs() []API {
	apisMu.RLock()
	defer apisMu.RUnlock()

	var apiList []API
	for _, api := range apis {
		apiList = append(apiList, api)
	}

	return apiList
}

func PluginHasAPI(plugin PluginInfo) bool {
	return plugin.GetAPI != nil
}

func PluginHasProtocol(plugin PluginInfo) bool {
	return plugin.GetProtocol != nil
}

type API interface {
	Subdomain() string
	Configure(router *gorilla.Router) error
	AuthTokenName() string
}

type APIInit interface {
	Init(ctx *Context) error
}

type RoutableAPI interface {
	Can(w http.ResponseWriter, r *http.Request) bool
	Handle(w http.ResponseWriter, r *http.Request)
}

type Protocol interface {
	Name() string
	Config() config.ProtocolConfig
}

type ProtocolInit interface {
	Init(ctx *Context) error
}

type ProtocolStart interface {
	Start(ctx Context) error
}

type ProtocolStop interface {
	Stop(ctx Context) error
}
