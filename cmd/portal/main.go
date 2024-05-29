package main

import (
	"github.com/LumeWeb/portal"
	"github.com/LumeWeb/portal/config"
	"github.com/LumeWeb/portal/core"
	"go.uber.org/zap"
	"os"
)

func main() {
	cfg, err := config.NewManager()
	logger := core.NewLogger(cfg)
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	err = cfg.Init()
	if err != nil {
		logger.Fatal("Failed to initialize config", zap.Error(err))
	}

	logger.SetLevelFromConfig()

	portal.NewActivePortal(core.NewBaseContext(cfg, logger))

	err = portal.Init()

	if err != nil {
		logger.Fatal("Failed to initialize portal", zap.Error(err))
		os.Exit(exitCodeFailedStartup)
	}

	err = portal.Start()

	if err != nil {
		logger.Error("Failed to start portal", zap.Error(err))
		os.Exit(exitCodeFailedStartup)
	}

	trapSignals()

	err = portal.Serve()
	if err != nil {
		logger.Error("Failed to serve portal", zap.Error(err))
		os.Exit(exitCodeFailedStartup)
	}
}
