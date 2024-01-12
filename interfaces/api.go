package interfaces

import (
	"git.lumeweb.com/LumeWeb/portal/api/router"
)

type API interface {
	Initialize(portal Portal, protocol Protocol) error
}

type APIRegistry interface {
	All() map[string]API
	Register(name string, APIRegistry API)
	Get(name string) (API, error)
	Router() *router.ProtocolRouter
}
