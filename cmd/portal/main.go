package main

import (
	"flag"
	"github.com/LumeWeb/portal/metadata"
	"github.com/LumeWeb/portal/sync"
	"net/http"

	"github.com/LumeWeb/portal/cron"

	_import "github.com/LumeWeb/portal/import"

	"github.com/LumeWeb/portal/mailer"

	"github.com/LumeWeb/portal/config"

	"github.com/LumeWeb/portal/account"
	"github.com/LumeWeb/portal/api"
	"github.com/LumeWeb/portal/db"
	_logger "github.com/LumeWeb/portal/logger"
	"github.com/LumeWeb/portal/protocols"
	"github.com/LumeWeb/portal/renter"
	"github.com/LumeWeb/portal/storage"
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
		metadata.Module,
		sync.Module,
		storage.Module,
		account.Module,
		_import.Module,
		mailer.Module,
		cron.Module,
		protocols.BuildProtocols(cfg),
		api.BuildApis(cfg),
		fx.Provide(api.NewCasbin),
		fx.Invoke(protocols.SetupLifecycles),
		fx.Invoke(api.SetupLifecycles),
		fx.Invoke(cron.Start),
		fx.Provide(NewServer),
		fx.Invoke(func(*http.Server) {}),
	).Run()
}
