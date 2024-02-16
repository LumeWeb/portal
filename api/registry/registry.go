package registry

import (
	"context"

	"github.com/julienschmidt/httprouter"

	router2 "git.lumeweb.com/LumeWeb/portal/api/router"
	"go.uber.org/fx"
)

type API interface {
	Name() string
	Init() error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Routes() *httprouter.Router
}

type APIEntry struct {
	Key    string
	Module fx.Option
}

var apiRegistry []APIEntry
var router router2.ProtocolRouter

func init() {
	router = make(router2.ProtocolRouter)
}

func Register(entry APIEntry) {
	apiRegistry = append(apiRegistry, entry)
}

func GetRegistry() []APIEntry {
	return apiRegistry
}

func GetRouter() router2.ProtocolRouter {
	return router
}
