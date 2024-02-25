package main

import (
	"context"
	"crypto/ed25519"
	"net"
	"net/http"
	"strconv"

	"git.lumeweb.com/LumeWeb/portal/api/router"

	"git.lumeweb.com/LumeWeb/portal/config"

	"git.lumeweb.com/LumeWeb/portal/api/registry"
	"go.sia.tech/core/wallet"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

func initCheckRequiredConfig(logger *zap.Logger, config *config.Manager) error {
	required := []string{
		"core.domain",
		"core.port",
		"core.sia.url",
		"core.sia.key",
		"core.db.username",
		"core.db.password",
		"core.db.host",
		"core.db.name",
		"core.storage.s3.buffer_bucket",
		"core.storage.s3.endpoint",
		"core.storage.s3.region",
		"core.storage.s3.access_key",
		"core.storage.s3.secret_key",
	}

	for _, key := range required {
		if !config.Viper().IsSet(key) {
			logger.Fatal(key + " is required")
		}
	}

	return nil
}

func NewIdentity(config *config.Manager, logger *zap.Logger) (ed25519.PrivateKey, error) {
	var seed [32]byte
	identitySeed := config.Config().Core.Identity

	if identitySeed == "" {
		logger.Info("Generating new identity seed")
		identitySeed = wallet.NewSeedPhrase()
		config.Viper().Set("core.identity", identitySeed)
		err := config.Save()
		if err != nil {
			return nil, err
		}
	}
	err := wallet.SeedFromPhrase(&seed, identitySeed)
	if err != nil {
		return nil, err
	}

	return ed25519.PrivateKey(wallet.KeyFromSeed(&seed, 0)), nil
}

type NewServerParams struct {
	fx.In
	Config *config.Manager
	Logger *zap.Logger
	APIs   []registry.API `group:"api"`
}

func NewServer(lc fx.Lifecycle, params NewServerParams) (*http.Server, error) {
	r := registry.GetRouter()

	r.SetConfig(params.Config)
	r.SetLogger(params.Logger)

	for _, api := range params.APIs {
		routableAPI, ok := interface{}(api).(router.RoutableAPI)

		if !ok {
			params.Logger.Fatal("API does not implement RoutableAPI", zap.String("api", api.Name()))
		}

		r.RegisterAPI(routableAPI)
	}

	srv := &http.Server{
		Addr:    ":" + strconv.FormatUint(uint64(params.Config.Config().Core.Port), 10),
		Handler: r,
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			ln, err := net.Listen("tcp", srv.Addr)
			if err != nil {
				return err
			}

			go func() {
				err := srv.Serve(ln)
				if err != nil {
					params.Logger.Fatal("Failed to serve", zap.Error(err))
				}
			}()

			return nil
		},
		OnStop: func(ctx context.Context) error {
			return srv.Shutdown(ctx)
		},
	})
	return srv, nil
}
