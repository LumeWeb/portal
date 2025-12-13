package testing

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"go.lumeweb.com/configmanager"
	"go.lumeweb.com/configmanager/source"
	"go.lumeweb.com/portal/config"
	"go.uber.org/zap"
)

type ConfigMode int

const (
	ConfigModeReal ConfigMode = iota
	ConfigModeMock
)

// NewTestConfigManager creates a config manager for testing based on specified mode
func NewTestConfigManager(t *testing.T, mode ConfigMode) config.Manager {
	switch mode {
	case ConfigModeReal:
		return newRealTestConfig(t)
	case ConfigModeMock:
		return config.NewMockManager(t)
	default:
		panic("unknown config mode")
	}
}

// newRealTestConfig creates a real config manager with temp directories for testing
func newRealTestConfig(t *testing.T) config.Manager {
	// Create temp directory for test isolation
	tempDir, err := os.MkdirTemp("", "portal-test-")
	if err != nil {
		panic(fmt.Sprintf("failed to create temp dir: %v", err))
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})

	// Create basic config manager with memory source
	cm, err := configmanager.NewConfigManager(
		configmanager.UsingSources(source.NewMemoryConfigSource(map[string]any{
			"core.domain":      "portal.local",
			"core.portal_name": "portal",
		})),
		configmanager.WithLogger(zap.NewNop()),
	)
	if err != nil {
		panic(fmt.Sprintf("failed to create configmanager: %v", err))
	}

	// Register core config structs
	if err := cm.RegisterStruct("core", config.CoreConfig{}); err != nil {
		panic(fmt.Sprintf("failed to register core config struct: %v", err))
	}

	if err := cm.RegisterStruct("", config.Config{}); err != nil {
		panic(fmt.Sprintf("failed to register config struct: %v", err))
	}

	// Create file source for temp directory
	configFile := filepath.Join(tempDir, "core.yaml")
	fileSource := source.NewFileSource(configFile)
	cm.RegisterSource(fileSource)
	cm.RegisterNamespace("core", fileSource)
	cm.RegisterSource(source.NewDefaultConfigSource(cm, source.WithDefaultSourceGlobal()))

	if err := cm.Load(); err != nil {
		panic(fmt.Sprintf("failed to load config: %v", err))
	}

	// Create real manager using the same logic as production
	manager, err := config.NewManager(nil)
	if err != nil {
		panic(fmt.Sprintf("failed to create manager: %v", err))
	}

	return manager
}
