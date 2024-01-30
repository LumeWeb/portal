package main

import (
	"git.lumeweb.com/LumeWeb/portal/account"
	"git.lumeweb.com/LumeWeb/portal/api"
	_config "git.lumeweb.com/LumeWeb/portal/config"
	"git.lumeweb.com/LumeWeb/portal/cron"
	"git.lumeweb.com/LumeWeb/portal/db"
	_logger "git.lumeweb.com/LumeWeb/portal/logger"
	"git.lumeweb.com/LumeWeb/portal/protocols"
	"git.lumeweb.com/LumeWeb/portal/storage"
	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	_ "go.uber.org/fx/fxevent"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	_ "go.uber.org/zap/zapcore"
	"net/http"
)

func main() {

	logger := _logger.NewLogger()
	config, err := _config.NewConfig(logger)

	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	var fxDebug bool

	flag.BoolVar(&fxDebug, "fx-debug", false, "Enable fx framework debug logging")
	flag.Parse()

	protocols.RegisterProtocols()
	api.RegisterApis()

	var fxLogger fx.Option

	fxLogger = fx.WithLogger(func(logger *zap.Logger) fxevent.Logger {
		log := &fxevent.ZapLogger{Logger: logger}
		log.UseLogLevel(zapcore.InfoLevel)
		log.UseErrorLevel(zapcore.ErrorLevel)
		return log
	})

	if fxDebug {
		fxLogger = fx.Options()
	}

	fx.New(
		fx.Provide(_logger.NewFallbackLogger),
		fx.Provide(func() *viper.Viper {
			return config
		}),

		fx.Decorate(func() *zap.Logger {
			return logger
		}),
		fxLogger,
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
