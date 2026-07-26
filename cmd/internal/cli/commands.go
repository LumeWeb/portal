package cli

import (
	"context"
	"fmt"
	"github.com/urfave/cli/v3"
	"go.lumeweb.com/portal/build"
	"go.lumeweb.com/portal/cmd/internal/portal"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.uber.org/zap"
)

func NewPortalCLI() *cli.Command {
	return &cli.Command{
		Name:    "portal",
		Usage:   "Lume Web Portal Server",
		Version: getVersion(),
		Commands: []*cli.Command{
			{
				Name:  "version",
				Usage: "Print version information",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "full",
						Usage: "Print detailed version information",
					},
				},
				Action: versionAction,
			},
			{
				Name:   "config-env",
				Usage:  "Print all available environment variables for configuration, including active values, and the source config setting",
				Action: configEnvAction,
			},
			{
				Name:  "sia",
				Usage: "Manage Sia indexer connection",
				Commands: []*cli.Command{
					newSiaLoginCommand(),
				},
			},
			newMigrateRenterdCommand(),
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return portal.StartServer(cmd)
		},
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "env",
				Usage: "Use environment variables for configuration",
			},
		},
	}
}

// versionAction handles the version command
var versionAction cli.ActionFunc = func(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args()
	for i := 0; i < args.Len(); i++ {
		if args.Get(i) == "--full" {
			info := build.Default.Info()
			fmt.Printf("Version:      %s\n", info.Version)
			fmt.Printf("Git Commit:   %s\n", info.GitCommit)
			fmt.Printf("Git Branch:   %s\n", info.GitBranch)
			fmt.Printf("Build Time:   %s\n", info.BuildTime)
			fmt.Printf("Go Version:   %s\n", info.GoVersion)
			fmt.Printf("Platform:     %s\n", info.Platform)
			fmt.Printf("Architecture: %s\n", info.Architecture)
			return nil
		}
	}

	// Print short version
	info := build.Default.Info()
	version := info.GetVersion()
	commit := info.GetCommit()
	if len(commit) > 8 {
		commit = commit[:8]
	}
	fmt.Printf("%s-%s\n", version, commit)
	return nil
}

// configEnvAction handles the config-env command
var configEnvAction cli.ActionFunc = func(ctx context.Context, cmd *cli.Command) error {
	err := cmd.Set("env", "true")
	if err != nil {
		return err
	}
	// Create a new config manager instance
	manager, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to create config manager: %w", err)
	}

	// Create a logger for the config manager
	logger, err := zap.NewDevelopment()
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}

	manager.SetLogger(logger)

	// Register services from plugins (this just registers them, doesn't start them)
	core.RegisterServicesFromPlugins()
	core.RegisterKeyIdentityHandlersFromPlugins()

	// Disable validation for config-env command since we may not have all required values
	manager.DisableValidation()

	// Initialize just the config manager to process configs
	// We ignore validation errors here since we just want to display the config
	_ = manager.Init()

	// Re-enable validation
	manager.EnableValidation()

	// Register service configs
	for _, svcInfo := range core.GetServices() {
		svc, _, err := svcInfo.Factory()
		if err != nil {
			continue // Skip services that fail to create
		}

		if !core.IsCoreService(svcInfo.ID) {
			if configurableSvc, ok := svc.(core.Configurable); ok {
				cfg, err := configurableSvc.GetConfig()
				if err != nil {
					continue
				}

				svcConfig, ok := cfg.(config.ServiceConfig)
				if !ok {
					continue
				}

				plugin := core.GetPluginForService(svcInfo.ID)
				if plugin == "" {
					continue
				}

				_ = manager.ConfigureService(plugin, svcInfo.ID, svcConfig)
			}
		}
	}

	// Register plugin configs
	for _, plugin := range core.GetPlugins() {
		// Register protocol configs
		if core.PluginHasProtocol(plugin) {
			if proto, _, err := plugin.Protocol(); err == nil && proto != nil {
				_ = manager.ConfigureProtocol(plugin.ID, proto.GetConfig())
			}
		}

		// Register API configs
		if core.PluginHasAPI(plugin) {
			if api, _, err := plugin.API(); err == nil && api != nil {
				_ = manager.ConfigureAPI(plugin.ID, api.GetConfig())
			}
		}
	}

	// Get all config keys from the manager
	allConfigs := manager.All()

	// Convert config keys to environment variables and print them
	for key, val := range allConfigs {
		envVar := config.EnvVarFor(key)
		fmt.Printf("%s=%s=%v\n", key, envVar, val)
	}

	return nil
}
