package main

import (
	"crypto/ed25519"
	"git.lumeweb.com/LumeWeb/portal/api"
	"git.lumeweb.com/LumeWeb/portal/interfaces"
	"git.lumeweb.com/LumeWeb/portal/protocols"
	"git.lumeweb.com/LumeWeb/portal/storage"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"net/http"
	"strconv"
)

var (
	_ interfaces.Portal = (*PortalImpl)(nil)
)

type PortalImpl struct {
	apiRegistry      interfaces.APIRegistry
	protocolRegistry protocols.ProtocolRegistry
	logger           *zap.Logger
	identity         ed25519.PrivateKey
	storage          interfaces.StorageService
}

func NewPortal() interfaces.Portal {

	portal := &PortalImpl{
		apiRegistry:      api.NewRegistry(),
		protocolRegistry: protocols.NewProtocolRegistry(),
		storage:          nil,
	}

	storageServ := storage.NewStorageService(portal)
	portal.storage = storageServ

	return portal
}

func (p *PortalImpl) Initialize() error {
	for _, initFunc := range getInitList() {
		if err := initFunc(p); err != nil {
			return err
		}
	}

	return nil
}
func (p *PortalImpl) Run() {
	for _, initFunc := range getStartList() {
		if err := initFunc(p); err != nil {
			p.logger.Fatal("Failed to start", zap.Error(err))
		}
	}
	p.logger.Fatal("HTTP server stopped", zap.Error(http.ListenAndServe(":"+strconv.FormatUint(uint64(p.Config().GetUint("core.port")), 10), p.apiRegistry.Router())))
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
func (p *PortalImpl) ApiRegistry() interfaces.APIRegistry {
	return p.apiRegistry
}

func (p *PortalImpl) Identity() ed25519.PrivateKey {
	return p.identity
}
func (p *PortalImpl) Storage() interfaces.StorageService {
	return p.storage
}

func (p *PortalImpl) SetIdentity(identity ed25519.PrivateKey) {
	p.identity = identity
}

func (p *PortalImpl) SetLogger(logger *zap.Logger) {
	p.logger = logger
}
func (p *PortalImpl) ProtocolRegistry() protocols.ProtocolRegistry {
	return p.protocolRegistry
}
