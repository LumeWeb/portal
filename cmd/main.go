package portalcmd

import (
	"go.lumeweb.com/portal"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.uber.org/zap"
	"os"
)

func Main() {
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
		os.Exit(core.ExitCodeFailedStartup)
	}

	err = portal.Start()

	if err != nil {
		logger.Error("Failed to start portal", zap.Error(err))
		os.Exit(core.ExitCodeFailedStartup)
	}

	trapSignals()

	err = portal.Serve()
	if err != nil {
		logger.Error("Failed to serve portal", zap.Error(err))
		os.Exit(core.ExitCodeFailedStartup)
	}
}
