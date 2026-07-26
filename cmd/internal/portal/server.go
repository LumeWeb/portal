package portal

import (
	"github.com/urfave/cli/v3"
	"go.lumeweb.com/portal"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.uber.org/zap"
	"os"
)

func StartServer(cmd *cli.Command) error {
	// Continue with normal portal initialization
	cfg, err := config.NewManager()
	if err != nil {
		// Create a basic logger for fatal error since we can't use core.NewLogger without config
		logger, _ := zap.NewDevelopment()
		logger.Fatal("Failed to load config", zap.Error(err))
		return err
	}
	logger := core.NewLogger(cfg)

	ctx, err := core.NewContext(cfg, logger)
	if err != nil {
		logger.Fatal("Failed to create context", zap.Error(err))
		return err
	}

	core.RegisterServicesFromPlugins()
	core.RegisterKeyIdentityHandlersFromPlugins()
	
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

	return nil
}
