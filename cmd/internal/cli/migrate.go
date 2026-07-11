package cli

import (
	"context"
	"fmt"

	"github.com/pterm/pterm"
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"

	"go.lumeweb.com/portal"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/service/migrator"
)

func newMigrateRenterdCommand() *cli.Command {
	return &cli.Command{
		Name:  "migrate-renterd-objects",
		Usage: "Migrate objects from a renterd backend to the indexd backend",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "renterd-url",
				Usage:    "renterd API URL (e.g. http://localhost:9980)",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "renterd-password",
				Usage:    "renterd API password",
				Required: true,
			},
			&cli.BoolFlag{
				Name:  "dry-run",
				Usage: "List objects that would be migrated without uploading",
			},
		},
		Action: migrateRenterdAction,
	}
}

func migrateRenterdAction(ctx context.Context, cmd *cli.Command) error {
	renterdURL := cmd.String("renterd-url")
	renterdPassword := cmd.String("renterd-password")
	dryRun := cmd.Bool("dry-run")

	// Boot the portal context (init only, no HTTP server).
	cfg, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	logger := core.NewLogger(cfg)

	portalCtx, err := core.NewContext(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to create context: %w", err)
	}

	core.RegisterServicesFromPlugins()

	portal.NewActivePortal(portalCtx)

	if err := portal.Init(); err != nil {
		// portal.Shutdown calls os.Exit, which would swallow the error.
		// Log it, close what we can, then return the error directly.
		logger.Error("failed to initialize portal", zap.Error(err))
		portal.Shutdown(portal.ActivePortal(), logger.Logger)
		return fmt.Errorf("failed to initialize portal: %w", err)
	}
	defer portal.Shutdown(portal.ActivePortal(), logger.Logger)

	// Get the RenterService — the migrator uses it directly for uploads.
	svc := core.GetServiceOptional[core.RenterService](portalCtx, core.RENTER_SERVICE)
	if svc == nil {
		return fmt.Errorf("renter service not found in registry")
	}

	// Collect all registered protocols that implement StorageProtocol.
	var protocols []core.StorageProtocol
	for _, proto := range core.GetProtocolList() {
		if sp, ok := proto.(core.StorageProtocol); ok {
			protocols = append(protocols, sp)
		}
	}

	if len(protocols) == 0 {
		return fmt.Errorf("no storage protocols registered")
	}

	names := make([]string, len(protocols))
	for i, p := range protocols {
		names[i] = p.Name()
	}
	pterm.Info.Printf("Found %d storage protocol(s): %v\n", len(protocols), names)

	if dryRun {
		pterm.Warning.Println("[dry-run] no objects will be uploaded")
	}

	// Create the renterd client.
	renterdClient := migrator.NewRenterdClient(renterdURL, renterdPassword)

	// Create the migrator.
	m := &migrator.Migrator{
		Renter:     svc,
		Lister:     renterdClient,
		Downloader: renterdClient,
		Logger:     portalCtx.Logger(),
		DryRun:     dryRun,
	}

	pterm.Info.Println("Starting migration...")
	stats, err := m.Migrate(ctx, protocols)
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	pterm.Success.Printf("Migration complete: %s\n", stats.String())
	return nil
}

// (no trailing helpers needed)
