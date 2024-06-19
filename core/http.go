package core

import (
	"github.com/gorilla/mux"
)

const HTTP_SERVICE = "http"

type HTTPService interface {
	Router() *mux.Router
	Init() error
	Serve() error

	Service
}
