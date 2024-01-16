package main

import (
	"crypto/ed25519"
	"git.lumeweb.com/LumeWeb/portal/api"
	"git.lumeweb.com/LumeWeb/portal/config"
	"git.lumeweb.com/LumeWeb/portal/interfaces"
	"git.lumeweb.com/LumeWeb/portal/logger"
	"git.lumeweb.com/LumeWeb/portal/protocols"
	"go.sia.tech/core/wallet"
)

type initFunc func(p interfaces.Portal) error

func initConfig(p interfaces.Portal) error {
	return config.Init()
}

func initIdentity(p interfaces.Portal) error {
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

	p.SetIdentity(ed25519.PrivateKey(wallet.KeyFromSeed(&seed, 0)))

	return nil
}

func initCheckRequiredConfig(p interfaces.Portal) error {
	required := []string{
		"core.domain",
		"core.port",
		"core.sia.url",
		"core.sia.key",
	}

	for _, key := range required {
		if !p.Config().IsSet(key) {
			p.Logger().Fatal(key + " is required")
		}
	}

	return nil
}

func initLogger(p interfaces.Portal) error {
	p.SetLogger(logger.Init(p.Config()))

	return nil
}

func initProtocols(p interfaces.Portal) error {
	return protocols.Init(p.ProtocolRegistry())
}

func initStorage(p interfaces.Portal) error {
	p.Storage().Init()

	return nil
}

func initAPI(p interfaces.Portal) error {
	return api.Init(p.ApiRegistry())
}

func initializeProtocolRegistry(p interfaces.Portal) error {
	for _, _func := range p.ProtocolRegistry().All() {
		err := _func.Initialize(p)
		if err != nil {
			return err
		}
	}

	return nil
}

func initializeAPIRegistry(p interfaces.Portal) error {
	for protoName, _func := range p.ApiRegistry().All() {
		proto, err := p.ProtocolRegistry().Get(protoName)
		if err != nil {
			return err
		}
		err = _func.Initialize(p, proto)
		if err != nil {
			return err
		}
	}

	return nil
}

func getInitList() []initFunc {
	return []initFunc{
		initConfig,
		initIdentity,
		initCheckRequiredConfig,
		initLogger,
		initProtocols,
		initStorage,
		initAPI,
		initializeProtocolRegistry,
		initializeAPIRegistry,
	}
}
