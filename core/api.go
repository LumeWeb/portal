package core

import (
	"fmt"
	gorilla "github.com/gorilla/mux"
	"go.lumeweb.com/portal/config"
	"net/http"
	"sort"
	"sync"
)

var (
	apis   = make(map[string]API)
	apisMu sync.RWMutex
)

type API interface {
	Name() string
	Subdomain() string
	Configure(router *gorilla.Router, accessSvc AccessService) error
	AuthTokenName() string
	Config() config.APIConfig
}

type APIInit interface {
	Init() ([]ContextBuilderOption, error)
}

type RoutableAPI interface {
	Can(w http.ResponseWriter, r *http.Request) bool
	Handle(w http.ResponseWriter, r *http.Request)
}

func RegisterAPI(id string, api API) {
	apisMu.Lock()
	defer apisMu.Unlock()

	if _, ok := apis[id]; ok {
		panic(fmt.Sprintf("api already registered: %s", id))
	}

	apis[id] = api
}

func GetAPI(id string) API {
	apisMu.RLock()
	defer apisMu.RUnlock()

	api, ok := apis[id]

	if !ok {
		return nil
	}

	return api
}

func APIExists(id string) bool {
	apisMu.RLock()
	defer apisMu.RUnlock()

	_, ok := apis[id]

	return ok
}

func GetAPIs() map[string]API {
	apisMu.RLock()
	defer apisMu.RUnlock()

	return apis
}

func GetAPIList() []API {
	apisMu.RLock()
	defer apisMu.RUnlock()

	keys := make([]string, 0, len(apis))
	for k := range apis {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var apiList []API
	for _, k := range keys {
		apiList = append(apiList, apis[k])
	}

	return apiList
}

func PluginHasAPI(plugin PluginInfo) bool {
	return plugin.API != nil
}
