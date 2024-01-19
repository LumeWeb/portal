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
	return config.Init(p)
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
		"core.db.username",
		"core.db.password",
		"core.db.host",
		"core.db.name",
		"core.storage.s3.bufferBucket",
		"core.storage.s3.endpoint",
		"core.storage.s3.region",
		"core.storage.s3.accessKey",
		"core.storage.s3.secretKey",
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

func initAccess(p interfaces.Portal) error {
	p.SetCasbin(api.GetCasbin(p.Logger()))
	return nil
}

func initDatabase(p interfaces.Portal) error {
	return p.DatabaseService().Init()
}

func initProtocols(p interfaces.Portal) error {
	return protocols.Init(p.ProtocolRegistry())
}

func initStorage(p interfaces.Portal) error {
	return p.Storage().Init()
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

func initCron(p interfaces.Portal) error {
	return p.CronService().Init()
}

func getInitList() []initFunc {
	return []initFunc{
		initConfig,
		initIdentity,
		initLogger,
		initCheckRequiredConfig,
		initAccess,
		initDatabase,
		initProtocols,
		initStorage,
		initAPI,
		initializeProtocolRegistry,
		initializeAPIRegistry,
		initCron,
	}
}
