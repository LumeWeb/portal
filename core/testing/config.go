// Package config provides configuration testing utilities.
package testing

import (
	"fmt"
	"maps"
	"os"
	"strings"

	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.sia.tech/coreutils/wallet"
)

// ConfigBuilder is a generic builder for any config type that implements
// the Defaults pattern (map[string]any)
type ConfigBuilder struct {
	values map[string]any
}

// NewConfigBuilder creates a new ConfigBuilder
func NewConfigBuilder() *ConfigBuilder {
	return &ConfigBuilder{
		values: make(map[string]any),
	}
}

// With adds a configuration key-value pair
func (b *ConfigBuilder) With(key string, value any) *ConfigBuilder {
	b.values[key] = value
	return b
}

// Build creates a config object implementing the Defaults interface
func (b *ConfigBuilder) Build() config.Defaults {
	// Create a copy to avoid shared reference issues
	return &genericConfig{values: maps.Clone(b.values)}
}

// AsOptions converts the ConfigBuilder's values into TestContextBuilderOption functions
func (b *ConfigBuilder) AsOptions() []TestContextBuilderOption {
	var opts []TestContextBuilderOption
	for key, value := range b.values {
		opts = append(opts, WithConfig(key, value))
	}
	return opts
}

// genericConfig implements config.Defaults with custom values
type genericConfig struct {
	values map[string]any
}

func (c *genericConfig) Defaults() map[string]any {
	return c.values
}

// GetMockConfig returns the mock config manager from the context for testing
// Panics if the config manager is not a mock
func GetMockConfig(ctx core.Context) *MockConfigManager {
	mockConfig, ok := ctx.Config().(*MockConfigManager)
	if !ok {
		panic("config manager is not a mock - use NewTestContext() for testing")
	}
	return mockConfig
}

// WithAPIConfig sets an expectation on the mock ConfigManager
// to return the provided config when GetAPI is called with the given ID.
// The expectation is set to Maybe() to allow but not require the call.
func WithAPIConfig(apiID string, apiConfig config.APIConfig) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		mockConfig := GetMockConfig(ctx)
		err := mockConfig.ConfigureAPI(apiID, apiConfig)
		if err != nil {
			return ctx, fmt.Errorf("failed to configure API %s: %w", apiID, err)
		}
		return ctx, nil
	}
}

// WithCustomAPIConfig creates API config using builder pattern
func WithCustomAPIConfig(apiID string, builder *ConfigBuilder) TestContextBuilderOption {
	return WithAPIConfig(apiID, builder.Build())
}

// WithProtocolConfig configures a protocol with the given config using the config manager
func WithProtocolConfig(protocolID string, protocolConfig config.ProtocolConfig) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		mockConfig := GetMockConfig(ctx)
		err := mockConfig.ConfigureProtocol(protocolID, protocolConfig)
		if err != nil {
			return ctx, fmt.Errorf("failed to configure protocol %s: %w", protocolID, err)
		}
		return ctx, nil
	}
}

// WithCustomProtocolConfig creates protocol config using builder pattern
func WithCustomProtocolConfig(protocolID string, builder *ConfigBuilder) TestContextBuilderOption {
	return WithProtocolConfig(protocolID, builder.Build())
}

// WithServiceConfig configures a service with the given config using the config manager
func WithServiceConfig(pluginName string, serviceName string, serviceConfig config.ServiceConfig) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		mockConfig := GetMockConfig(ctx)
		err := mockConfig.ConfigureService(pluginName, serviceName, serviceConfig)
		if err != nil {
			return ctx, fmt.Errorf("failed to configure service %s for plugin %s: %w", serviceName, pluginName, err)
		}
		return ctx, nil
	}
}

// WithCustomServiceConfig creates service config using builder pattern
func WithCustomServiceConfig(pluginName string, serviceName string, builder *ConfigBuilder) TestContextBuilderOption {
	return WithServiceConfig(pluginName, serviceName, builder.Build())
}

// WithDomain configures the test context with a specific domain
func WithDomain(domain string) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		if err := ctx.Config().Set(ctx, "core.domain", domain); err != nil {
			return nil, err
		}
		return ctx, nil
	}
}

// WithSeedPhrase configures the test context with a specific seed phrase
func WithSeedPhrase(seedPhrase string) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		if err := ctx.Config().Set(ctx, "core.identity", seedPhrase); err != nil {
			return nil, err
		}
		return ctx, nil
	}
}

// WithRandomSeedPhrase configures the test context with a randomly generated seed phrase
func WithRandomSeedPhrase() TestContextBuilderOption {
	return WithSeedPhrase(wallet.NewSeedPhrase())
}

// WithConfig sets a configuration value
func WithConfig(key string, value interface{}) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		if ctx.(*testContext).cfg != nil {
			// Use Update method from config.Manager interface
			_ = ctx.(*testContext).cfg.Set(ctx, key, value)
		}
		return ctx, nil
	}
}

// envVarName converts a config key to an environment variable name
// by replacing dots with double underscores
func envVarName(key string) string {
	return strings.ReplaceAll(key, ".", "__")
}

// getEnvValue tries to get a value from environment variables in order,
// returning the first non-empty value found. If no env vars are provided
// or all are empty, it returns the default value if provided.
func getEnvValue(envVars []string, defaultValues []interface{}) (string, error) {
	for i, envVar := range envVars {
		value := os.Getenv(envVar)
		if value != "" {
			return value, nil
		}
		
		// If env var is not set, try default value
		if i < len(defaultValues) && defaultValues[i] != nil {
			defaultValue := defaultValues[i]
			return fmt.Sprintf("%v", defaultValue), nil
		}
	}
	
	// If we get here, no env vars were set and no defaults were provided
	if len(defaultValues) > 0 {
		return "", fmt.Errorf("none of the environment variables %v are set and no valid default values provided", envVars)
	}
	return "", fmt.Errorf("none of the environment variables %v are set", envVars)
}

// WithEnvConfig reads a configuration value from environment variable
// and sets it in the test context. Returns error if none of the env vars are set.
// If no envVars provided, the key will be converted to an env var name.
func WithEnvConfig(key string, envVars ...string) TestContextBuilderOption {
	if len(envVars) == 0 {
		envVars = []string{envVarName(key)}
	}
	return func(ctx TestContext) (TestContext, error) {
		value, err := getEnvValue(envVars, nil)
		if err != nil {
			return nil, err
		}
		return WithConfig(key, value)(ctx)
	}
}

// WithEnvConfigOrDefault reads a configuration value from environment variable
// and sets it in the test context. Uses default value if none of the env vars are set.
// If no envVars provided, the key will be converted to an env var name.
func WithEnvConfigOrDefault(key string, envVarsAndDefaults ...interface{}) TestContextBuilderOption {
	var envVars []string
	var defaultValues []interface{}

	// Process the variadic arguments
	for i, arg := range envVarsAndDefaults {
		if i%2 == 0 {
			// Even index: environment variable name
			if envVar, ok := arg.(string); ok {
				envVars = append(envVars, envVar)
			}
		} else {
			// Odd index: default value
			defaultValues = append(defaultValues, arg)
		}
	}

	// If no env vars provided, compute from key
	if len(envVars) == 0 {
		envVars = []string{envVarName(key)}
	}

	// Ensure defaultValues matches envVars length
	for len(defaultValues) < len(envVars) {
		defaultValues = append(defaultValues, nil)
	}

	return func(ctx TestContext) (TestContext, error) {
		value, err := getEnvValue(envVars, defaultValues)
		if err != nil {
			return nil, err
		}
		return WithConfig(key, value)(ctx)
	}
}
