package core

import (
	"github.com/gorilla/mux"
)

type HTTPService interface {
	Router() *mux.Router
	Serve() error
}
