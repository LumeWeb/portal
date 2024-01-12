package api

import (
	"errors"
	"git.lumeweb.com/LumeWeb/portal/api/router"
	"git.lumeweb.com/LumeWeb/portal/interfaces"
)

var (
	_ interfaces.APIRegistry = (*APIRegistryImpl)(nil)
)

type APIRegistryImpl struct {
	apis   map[string]interfaces.API
	router *router.ProtocolRouter
}

func NewRegistry() interfaces.APIRegistry {
	return &APIRegistryImpl{
		apis:   make(map[string]interfaces.API),
		router: &router.ProtocolRouter{},
	}
}

func (r *APIRegistryImpl) Register(name string, APIRegistry interfaces.API) error {
	if _, exists := r.apis[name]; exists {
		return errors.New("api already registered")
	}
	r.apis[name] = APIRegistry
	return nil
}

func (r *APIRegistryImpl) Get(name string) (interfaces.API, error) {
	APIRegistry, exists := r.apis[name]
	if !exists {
		return nil, errors.New("api not found")
	}
	return APIRegistry, nil
}
func (r *APIRegistryImpl) Router() *router.ProtocolRouter {
	return r.router
}
