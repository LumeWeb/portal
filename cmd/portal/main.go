package main

import (
	"flag"
	"net/http"
	"time"

	"git.lumeweb.com/LumeWeb/portal/cron"

	_import "git.lumeweb.com/LumeWeb/portal/import"

	"git.lumeweb.com/LumeWeb/portal/mailer"

	"git.lumeweb.com/LumeWeb/portal/config"

	"git.lumeweb.com/LumeWeb/portal/account"
	"git.lumeweb.com/LumeWeb/portal/api"
	"git.lumeweb.com/LumeWeb/portal/db"
	_logger "git.lumeweb.com/LumeWeb/portal/logger"
	"git.lumeweb.com/LumeWeb/portal/metadata"
	"git.lumeweb.com/LumeWeb/portal/protocols"
	"git.lumeweb.com/LumeWeb/portal/renter"
	"git.lumeweb.com/LumeWeb/portal/storage"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func main() {
	cfg, err := config.NewManager()

	logger, logLevel := _logger.NewLogger(cfg)

	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	var fxDebug bool

	flag.BoolVar(&fxDebug, "fx-debug", false, "Enable fx framework debug logging")
	flag.Parse()

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
		fx.Supply(cfg),
		fx.Supply(logger, logLevel),
		fxLogger,
		fx.Provide(NewIdentity),
		db.Module,
		renter.Module,
		storage.Module,
		account.Module,
		metadata.Module,
		_import.Module,
		mailer.Module,
		cron.Module,
		protocols.BuildProtocols(cfg),
		api.BuildApis(cfg),
		fx.Provide(api.NewCasbin),
		fx.Invoke(protocols.SetupLifecycles),
		fx.Invoke(api.SetupLifecycles),
		fx.Provide(NewServer),
		fx.Invoke(func(*http.Server) {}),
	).Run()
}
