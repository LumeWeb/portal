package main

import (
	"context"
	"git.lumeweb.com/LumeWeb/portal/api"
	"git.lumeweb.com/LumeWeb/portal/api/registry"
	_config "git.lumeweb.com/LumeWeb/portal/config"
	"git.lumeweb.com/LumeWeb/portal/cron"
	"git.lumeweb.com/LumeWeb/portal/db"
	_logger "git.lumeweb.com/LumeWeb/portal/logger"
	"git.lumeweb.com/LumeWeb/portal/protocols"
	"git.lumeweb.com/LumeWeb/portal/storage"
	"github.com/spf13/viper"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
	"net"
	"net/http"
)

func main() {

	logger := _logger.NewLogger()
	config, err := _config.NewConfig(logger)

	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	protocols.RegisterProtocols()
	api.RegisterApis()

	fx.New(
		fx.Provide(_logger.NewFallbackLogger),
		fx.Provide(func() *viper.Viper {
			return config
		}),

		fx.Decorate(func() *zap.Logger {
			return logger
		}),
		fx.WithLogger(func(logger *zap.Logger) *fxevent.ZapLogger {
			return &fxevent.ZapLogger{Logger: logger}
		}),
		fx.Invoke(initCheckRequiredConfig),
		fx.Provide(NewIdentity),
		db.Module,
		storage.Module,
		cron.Module,
		protocols.BuildProtocols(config),
		api.BuildApis(config),
		fx.Provide(api.NewCasbin),
		fx.Invoke(protocols.SetupLifecycles),
		fx.Invoke(api.SetupLifecycles),
		fx.Provide(func(lc fx.Lifecycle, config *viper.Viper) *http.Server {
			srv := &http.Server{
				Addr:    config.GetString("core.port"),
				Handler: registry.GetRouter(),
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
							logger.Fatal("Failed to serve", zap.Error(err))
						}
					}()

					return nil
				},
				OnStop: func(ctx context.Context) error {
					return srv.Shutdown(ctx)
				},
			})
			return srv
		}),
	).Run()
}
