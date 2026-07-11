package cli

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/pterm/pterm"
	"github.com/urfave/cli/v3"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.uber.org/zap"
	sdk "go.sia.tech/siastorage"
)

const (
	siaLoginUsage = `Usage: portal sia login

Register the portal with the Sia indexer and obtain an app key.

This is an interactive command that will:
  1. Prompt for the indexer URL
  2. Prompt for a 12-word recovery phrase (or generate a new one)
  3. Open a connection request with the indexer
  4. Wait for approval (visit the provided URL in a browser)
  5. Register the app key
  6. Print the environment variables to set for container deployments
`
)

func newSiaLoginCommand() *cli.Command {
	return &cli.Command{
		Name:      "login",
		Usage:     "Register the portal with the Sia indexer",
		UsageText: siaLoginUsage,
		Action:    siaLoginAction,
	}
}

func siaLoginAction(ctx context.Context, cmd *cli.Command) error {
	// Load existing config to check for existing app key.
	manager, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to create config manager: %w", err)
	}

	logger, err := zap.NewDevelopment()
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}

	manager.SetLogger(logger)
	manager.DisableValidation()
	_ = manager.Init()
	manager.EnableValidation()

	// Read existing Sia config values for pre-fill.
	existingURL := ""
	if val, _, gErr := manager.Get("core.storage.sia.url"); gErr == nil {
		if s, ok := val.(string); ok {
			existingURL = s
		}
	}
	existingAppKey := ""
	if val, _, gErr := manager.Get("core.storage.sia.app_key"); gErr == nil {
		if s, ok := val.(string); ok {
			existingAppKey = s
		}
	}

	if existingAppKey != "" {
		pterm.Warning.Println("An app key is already configured. Running login again will replace it.")
		confirmed, err := pterm.DefaultInteractiveConfirm.
			WithDefaultText("Continue").
			WithDefaultValue(false).
			Show()
		if err != nil {
			return fmt.Errorf("failed to read confirmation: %w", err)
		}
		if !confirmed {
			return nil
		}
	}

	// Step 1: Prompt for indexer URL (no default).
	indexerURL, err := pterm.DefaultInteractiveTextInput.
		WithDefaultText("").
		Show("Enter the indexer URL (e.g. https://sia.storage)")
	if err != nil {
		return fmt.Errorf("failed to read indexer URL: %w", err)
	}
	indexerURL = strings.TrimSpace(indexerURL)
	if indexerURL == "" {
		if existingURL != "" {
			indexerURL = existingURL
		} else {
			return fmt.Errorf("indexer URL is required")
		}
	}

	// Step 2: Prompt for recovery phrase (or generate new).
	pterm.Println("Enter your 12-word recovery phrase.")
	pterm.Println("Leave blank to generate a new one.")

	phrase, err := pterm.DefaultInteractiveTextInput.
		WithDefaultText("").
		Show("Recovery phrase")
	if err != nil {
		return fmt.Errorf("failed to read recovery phrase: %w", err)
	}
	phrase = strings.TrimSpace(phrase)

	if phrase == "" {
		phrase = sdk.NewSeedPhrase()
		pterm.Println()
		pterm.Info.Println("A new recovery phrase has been generated.")
		pterm.Warning.Println("Write it down and keep it safe. It cannot be recovered if lost.")
		pterm.Println()
		pterm.FgLightBlue.Printf("  Recovery Phrase: %s\n", phrase)
		pterm.Println()

		// Confirm.
		confirm, err := pterm.DefaultInteractiveTextInput.
			WithDefaultText("").
			Show("Confirm recovery phrase")
		if err != nil {
			return fmt.Errorf("failed to read confirmation: %w", err)
		}
		if strings.TrimSpace(confirm) != phrase {
			return fmt.Errorf("recovery phrases do not match")
		}
	}

	// Step 3: Build the SDK builder and request connection.
	pterm.Info.Printf("Requesting connection to indexer at %s...\n", indexerURL)

	builder := sdk.NewBuilder(indexerURL, core.PortalAppMetadata())

	respURL, err := builder.RequestConnection(ctx)
	if err != nil {
		return fmt.Errorf("failed to request app connection: %w", err)
	}

	pterm.Println()
	pterm.Info.Println("Please approve the app connection by visiting the following URL:")
	pterm.FgLightBlue.Println("  " + respURL)
	pterm.Println()

	// Step 4: Wait for approval.
	pterm.Info.Println("Waiting for approval... (Ctrl+C to cancel)")
	if err := builder.WaitForApproval(ctx); err != nil {
		return fmt.Errorf("connection approval failed: %w", err)
	}

	pterm.Success.Println("Connection approved!")

	// Step 5: Register and get the SDK.
	sdkClient, err := builder.Register(ctx, phrase)
	if err != nil {
		return fmt.Errorf("failed to register app: %w", err)
	}

	appKey := sdkClient.AppKey()
	appKeyHex := hex.EncodeToString(appKey[:])

	// Save to config.
	if err := manager.BulkSetAtomic(ctx, map[string]any{
		"core.storage.sia.app_key": appKeyHex,
		"core.storage.sia.url":     indexerURL,
	}); err != nil {
		return fmt.Errorf("failed to save app key to config: %w", err)
	}

	// Step 6: Output env vars for container deployments.
	pterm.Println()
	pterm.Success.Println("Login successful! The app key has been saved to config.")
	pterm.Println()
	pterm.Info.Println("For container deployments, set the following environment variables:")
	pterm.Println()

	envKey := config.EnvVarFor("core.storage.sia.app_key")
	envURL := config.EnvVarFor("core.storage.sia.url")

	pterm.FgGreen.Printf("  %s=%s\n", envKey, appKeyHex)
	pterm.FgGreen.Printf("  %s=%s\n", envURL, indexerURL)

	pterm.Println()
	pterm.Info.Println("Or save these to your .env file / docker-compose.yml / k8s secret.")

	return nil
}
