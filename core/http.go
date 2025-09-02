package core

import (
	router "go.lumeweb.com/portal-router"
)

const HTTP_SERVICE = "http"

type HTTPService interface {
	Router() router.Router
	Init() error
	Serve() error
	APISubdomain(id string, proto bool) string
	RegisterGlobalPath(path string) error

	Service
}
