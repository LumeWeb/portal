package main

import (
	"context"
	"git.lumeweb.com/LumeWeb/portal/account"
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
	"go.uber.org/zap/zapcore"
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
		fx.WithLogger(func(logger *zap.Logger) fxevent.Logger {
			log := &fxevent.ZapLogger{Logger: logger}
			log.UseLogLevel(zapcore.InfoLevel)
			log.UseErrorLevel(zapcore.ErrorLevel)
			return log
		}),
		fx.Invoke(initCheckRequiredConfig),
		fx.Provide(NewIdentity),
		db.Module,
		storage.Module,
		cron.Module,
		account.Module,
		protocols.BuildProtocols(config),
		api.BuildApis(config),
		fx.Provide(api.NewCasbin),
		fx.Invoke(protocols.SetupLifecycles),
		fx.Invoke(api.SetupLifecycles),
		fx.Provide(NewServer),
		fx.Invoke(func(*http.Server) {}),
	).Run()
}
