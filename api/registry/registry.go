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
var router *router2.APIRouter

func init() {
	router = router2.NewAPIRouter()
}

func RegisterEntry(entry APIEntry) {
	apiEntryRegistry = append(apiEntryRegistry, entry)
}

func GetEntryRegistry() []APIEntry {
	return apiEntryRegistry
}

func GetRouter() *router2.APIRouter {
	return router
}
