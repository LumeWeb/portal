package core

import (
	"github.com/gorilla/mux"
)

const HTTP_SERVICE = "http"

type HTTPService interface {
	Router() *mux.Router
	Serve() error

	Service
}
