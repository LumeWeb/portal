package interfaces

import (
	"git.lumeweb.com/LumeWeb/portal/api/router"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

type API interface {
	Initialize(config *viper.Viper, logger *zap.Logger) error
}

type APIRegistry interface {
	Register(name string, APIRegistry API) error
	Get(name string) (API, error)
	Router() *router.ProtocolRouter
}
