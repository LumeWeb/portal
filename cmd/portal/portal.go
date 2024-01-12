package main

import (
	"git.lumeweb.com/LumeWeb/portal/api"
	"git.lumeweb.com/LumeWeb/portal/interfaces"
	"git.lumeweb.com/LumeWeb/portal/protocols"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"net/http"
)

var (
	_ interfaces.Portal = (*PortalImpl)(nil)
)

type PortalImpl struct {
	apiRegistry interfaces.APIRegistry
	logger      *zap.Logger
}

func NewPortal() interfaces.Portal {
	logger, _ := zap.NewDevelopment()
	return &PortalImpl{
		apiRegistry: api.NewRegistry(),
		logger:      logger,
	}
}

func (p *PortalImpl) Initialize() error {
	for _, initFunc := range p.getInitFuncs() {
		if err := initFunc(); err != nil {
			return err
		}
	}

	return nil
}
func (p *PortalImpl) Run() {
	p.logger.Fatal("HTTP server stopped", zap.Error(http.ListenAndServe(":8080", p.apiRegistry.Router())))
}

func (p *PortalImpl) Config() *viper.Viper {
	return viper.GetViper()
}

func (p *PortalImpl) Logger() *zap.Logger {
	return p.logger
}

func (p *PortalImpl) Db() *gorm.DB {
	return nil
}
func (p *PortalImpl) getInitFuncs() []func() error {
	return []func() error{
		func() error {
			return api.Init(p.apiRegistry)
		},
		func() error {
			return protocols.Init(p.Config())
		},
	}
}
