package main

import (
	"crypto/ed25519"
	"git.lumeweb.com/LumeWeb/portal/api"
	"git.lumeweb.com/LumeWeb/portal/config"
	"git.lumeweb.com/LumeWeb/portal/interfaces"
	"git.lumeweb.com/LumeWeb/portal/protocols"
	"git.lumeweb.com/LumeWeb/portal/storage"
	"github.com/spf13/viper"
	"go.sia.tech/core/wallet"
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
	logger, _ := zap.NewDevelopment()

	portal := &PortalImpl{
		apiRegistry:      api.NewRegistry(),
		protocolRegistry: protocols.NewProtocolRegistry(),
		logger:           logger,
		storage:          nil,
	}

	storageServ := storage.NewStorageService(portal)
	portal.storage = storageServ

	return portal
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
	for _, initFunc := range p.getStartFuncs() {
		if err := initFunc(); err != nil {
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
func (p *PortalImpl) getInitFuncs() []func() error {
	return []func() error{
		func() error {
			return config.Init(p.Logger())
		},
		func() error {
			var seed [32]byte
			identitySeed := p.Config().GetString("core.identity")

			if identitySeed == "" {
				p.Logger().Info("Generating new identity seed")
				identitySeed = wallet.NewSeedPhrase()
				p.Config().Set("core.identity", identitySeed)
				err := p.Config().WriteConfig()
				if err != nil {
					return err
				}
			}
			err := wallet.SeedFromPhrase(&seed, identitySeed)
			if err != nil {
				return err
			}

			p.identity = ed25519.PrivateKey(wallet.KeyFromSeed(&seed, 0))

			return nil
		},
		func() error {
			required := []string{
				"core.domain",
				"core.port",
				"core.sia.url",
				"core.sia.key",
			}

			for _, key := range required {
				if !p.Config().IsSet(key) {
					p.logger.Fatal(key + " is required")
				}
			}

			return nil
		},
		func() error {
			return protocols.Init(p.protocolRegistry)
		},
		func() error {
			p.storage.Init()

			return nil
		},
		func() error {
			return api.Init(p.apiRegistry)
		},
		func() error {
			for _, _func := range p.protocolRegistry.All() {
				err := _func.Initialize(p)
				if err != nil {
					return err
				}
			}

			return nil
		}, func() error {
			for protoName, _func := range p.apiRegistry.All() {
				proto, err := p.protocolRegistry.Get(protoName)
				if err != nil {
					return err
				}
				err = _func.Initialize(p, proto)
				if err != nil {
					return err
				}
			}

			return nil
		},
	}
}
func (p *PortalImpl) Storage() interfaces.StorageService {
	return p.storage
}

func (p *PortalImpl) getStartFuncs() []func() error {
	return []func() error{
		func() error {
			for _, _func := range p.protocolRegistry.All() {
				err := _func.Start()
				if err != nil {
					return err
				}
			}

			return nil
		},
	}
}
