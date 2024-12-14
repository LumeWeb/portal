package portalcmd

import (
	"context"
	"go.lumeweb.com/portal"
	"go.lumeweb.com/portal/cmd/internal/cli"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.uber.org/zap"
	"os"
)

func Main() {
	// Initialize CLI first
	command := cli.NewPortalCLI()
	if len(os.Args) > 1 {
		ctx := context.Background()
		if err := command.Run(ctx, os.Args); err != nil {
			os.Exit(1)
		}
		return
	}

	// Continue with normal portal initialization
	cfg, err := config.NewManager()
	logger := core.NewLogger(cfg)
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	ctx, err := core.NewContext(cfg, logger)

	if err != nil {
		logger.Fatal("Failed to create context", zap.Error(err))
	}

	core.RegisterServicesFromPlugins()

	err = cfg.Init()
	if err != nil {
		logger.Fatal("Failed to initialize config", zap.Error(err))
	}

	logger.SetLevelFromConfig()

	portal.NewActivePortal(ctx)

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
