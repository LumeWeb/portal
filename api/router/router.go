package router

import (
	"net/http"
	"sync"

	"github.com/LumeWeb/portal/config"

	"go.uber.org/zap"

	"github.com/julienschmidt/httprouter"
)

type RoutableAPI interface {
	Name() string
	Domain() string
	AuthTokenName() string
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
	mutex       *sync.RWMutex
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
	hs.mutex.RLock()
	handler, ok := hs.apiHandlers[protocol]
	hs.mutex.RUnlock()

	if ok {
		return handler
	}

	hs.mutex.Lock()
	defer hs.mutex.Unlock()

	// Double-check if the handler was created while acquiring the write lock
	if handler, ok := hs.apiHandlers[protocol]; ok {
		return handler
	}

	proto, ok := hs.apis[protocol]
	if !ok {
		hs.logger.Fatal("Protocol not found", zap.String("protocol", protocol))
		return nil
	}

	routes, err := proto.Routes()
	if err != nil {
		hs.logger.Fatal("Error getting routes", zap.Error(err))
		return nil
	}

	hs.apiHandlers[protocol] = routes
	return routes
}

func NewAPIRouter() *APIRouter {
	return &APIRouter{
		apis:        make(map[string]RoutableAPI),
		apiHandlers: make(map[string]http.Handler),
		apiDomain:   make(map[string]string),
		mutex:       &sync.RWMutex{},
	}
}

func (hs *APIRouter) SetLogger(logger *zap.Logger) {
	hs.logger = logger
}

func (hs *APIRouter) SetConfig(config *config.Manager) {
	hs.config = config
}

func BuildSubdomain(api RoutableAPI, cfg *config.Manager) string {
	return api.Name() + "." + cfg.Config().Core.Domain
}
