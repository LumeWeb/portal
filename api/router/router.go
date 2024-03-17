package router

import (
	"net/http"

	"git.lumeweb.com/LumeWeb/portal/config"

	"go.uber.org/zap"

	"github.com/julienschmidt/httprouter"
)

type RoutableAPI interface {
	Name() string
	Domain() string
	Can(w http.ResponseWriter, r *http.Request) bool
	Handle(w http.ResponseWriter, r *http.Request)
	Routes() (*httprouter.Router, error)
}

type APIRouter struct {
	apis        map[string]RoutableAPI
	apiDomain   map[string]string
	apiHandlers map[string]http.Handler
	logger      *zap.Logger
	config      *config.Manager
}

// Implement the ServeHTTP method on our new type
func (hs APIRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if handler := hs.getHandlerByDomain(r.Host); handler != nil {
		handler.ServeHTTP(w, r)
		return
	}

	for _, api := range hs.apis {
		if api.Can(w, r) {
			api.Handle(w, r)
			return
		}
	}

	http.NotFound(w, r)
}

func (hs *APIRouter) RegisterAPI(impl RoutableAPI) {
	name := impl.Name()
	hs.apis[name] = impl
	hs.apiDomain[name+"."+hs.config.Config().Core.Domain] = name
}

func (hs *APIRouter) getHandlerByDomain(domain string) http.Handler {
	if apiName := hs.apiDomain[domain]; apiName != "" {
		return hs.getHandler(apiName)
	}

	return nil
}

func (hs *APIRouter) getHandler(protocol string) http.Handler {
	if handler := hs.apiHandlers[protocol]; handler == nil {
		if proto := hs.apis[protocol]; proto == nil {
			hs.logger.Fatal("Protocol not found", zap.String("protocol", protocol))
			return nil
		}

		routes, err := hs.apis[protocol].Routes()

		if err != nil {
			hs.logger.Fatal("Error getting routes", zap.Error(err))
			return nil
		}

		hs.apiHandlers[protocol] = routes
	}

	return hs.apiHandlers[protocol]
}

func NewAPIRouter() *APIRouter {
	return &APIRouter{
		apis:        make(map[string]RoutableAPI),
		apiHandlers: make(map[string]http.Handler),
		apiDomain:   make(map[string]string),
	}
}

func (hs *APIRouter) SetLogger(logger *zap.Logger) {
	hs.logger = logger
}

func (hs *APIRouter) SetConfig(config *config.Manager) {
	hs.config = config
}
