package registry

import (
	"context"

	router2 "git.lumeweb.com/LumeWeb/portal/api/router"
	"go.uber.org/fx"
)

type API interface {
	Name() string
	Init() error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
}

type APIEntry struct {
	Key    string
	Module fx.Option
}

var apiEntryRegistry []APIEntry
var apiRegistry map[string]API
var router *router2.APIRouter

func init() {
	router = router2.NewAPIRouter()
	apiRegistry = make(map[string]API)
}

func RegisterEntry(entry APIEntry) {
	apiEntryRegistry = append(apiEntryRegistry, entry)
}

func RegisterAPI(api API) {
	apiRegistry[api.Name()] = api
}

func GetEntryRegistry() []APIEntry {
	return apiEntryRegistry
}

func GetAPI(name string) API {
	if _, ok := apiRegistry[name]; !ok {
		panic("API not found: " + name)
	}

	return apiRegistry[name]
}

func GetAllAPIs() map[string]API {
	return apiRegistry
}

func GetRouter() *router2.APIRouter {
	return router
}
