// Package config provides configuration testing utilities.
package testing

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/fatih/structs"
	"github.com/kenshaw/snaker"
	"github.com/knadh/koanf/maps"
	"github.com/samber/lo"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.sia.tech/coreutils/wallet"
	"go.uber.org/zap"
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
	return &genericConfig{values: maps.Copy(b.values)}
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
func GetMockConfig(ctx core.Context) *config.MockManager {
	mockConfig, ok := ctx.Config().(*config.MockManager)
	if !ok {
		panic("config manager is not a mock - use NewTestContextWithConfig() with ConfigModeMock")
	}
	return mockConfig
}

// GetRealConfig returns the real config manager from the context for testing
// Panics if the config manager is not a real config
func GetRealConfig(ctx core.Context) *config.ManagerDefault {
	realConfig, ok := ctx.Config().(*config.ManagerDefault)
	if !ok {
		panic("config manager is not real - use NewTestContext() or NewTestContextWithConfig() with ConfigModeReal")
	}
	return realConfig
}

// WithAPIConfig configures an API with the given config using the config manager
func WithAPIConfig(apiID string, apiConfig config.APIConfig) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		cfg := ctx.Config()
		if err := cfg.ConfigureAPI(apiID, apiConfig); err != nil {
			return ctx, fmt.Errorf("failed to configure API %s: %w", apiID, err)
		}
		prefix := fmt.Sprintf(config.APISpecifier, apiID)
		if err := ApplyConfig(ctx, prefix, apiConfig); err != nil {
			return ctx, err
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
		cfg := ctx.Config()
		if err := cfg.ConfigureProtocol(protocolID, protocolConfig); err != nil {
			return ctx, fmt.Errorf("failed to configure protocol %s: %w", protocolID, err)
		}
		prefix := fmt.Sprintf(config.ProtocolSpecifier, protocolID)
		if err := ApplyConfig(ctx, prefix, protocolConfig); err != nil {
			return ctx, err
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
		cfg := ctx.Config()
		if err := cfg.ConfigureService(pluginName, serviceName, serviceConfig); err != nil {
			return ctx, fmt.Errorf("failed to configure service %s for plugin %s: %w", serviceName, pluginName, err)
		}
		prefix := fmt.Sprintf(config.ServiceSpecifier, pluginName, serviceName)
		if err := ApplyConfig(ctx, prefix, serviceConfig); err != nil {
			return ctx, err
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

// setConfigValue is a private helper that sets a configuration value on the test context.
// It returns an error if the config manager is not available, ensuring test setup
// issues are not silently ignored.
func setConfigValue(ctx TestContext, key string, value interface{}) error {
	tctx, ok := ctx.(*testContext)
	if !ok {
		return fmt.Errorf("test context is not a *testContext instance")
	}
	if tctx.cfg == nil {
		return fmt.Errorf("no config manager on test context")
	}
	return tctx.cfg.Set(ctx, key, value)
}

// WithConfig sets a configuration value
func WithConfig(key string, value interface{}) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		if err := setConfigValue(ctx, key, value); err != nil {
			return ctx, err
		}
		return ctx, nil
	}
}

// WithConfigMap applies multiple configuration values from a single map[string]any.
// The keys are config paths (e.g., "my.service.setting") and values are config values.
// This is useful for applying flattened configuration structs.
func WithConfigMap(configs map[string]any) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		for key, value := range configs {
			if err := setConfigValue(ctx, key, value); err != nil {
				return ctx, err
			}
		}
		return ctx, nil
	}
}

// flattenAndSnakeCase converts a map to a flattened map[string]any with snake_case keys.
// It uses knadh/koanf/maps to flatten nested maps with dot notation delimiters.
// Keys are converted from CamelCase to snake_case using snaker.CamelToSnake.
// Returns a single flattened map and any error from the flattening operation.
func flattenAndSnakeCase(inputMap map[string]any) (map[string]any, error) {
	// Flatten the nested map structure
	flatMap, err := maps.Flatten(inputMap, nil, ".")
	if err != nil {
		return nil, fmt.Errorf("failed to flatten config map: %w", err)
	}

	// Convert []map[string]any to map[string]any and convert keys to snake_case
	result := make(map[string]any, len(flatMap))
	for key, value := range flatMap {
		snakeKey := snaker.CamelToSnake(key)
		result[snakeKey] = value
	}

	return result, nil
}

// flattenConfigMap converts a struct implementing config.Defaults to a flattened map[string]any.
// It uses fatih/structs to convert struct to a map, then calls flattenAndSnakeCase.
// Returns a single flattened map and any error from the flattening operation.
func flattenConfigMap(cfg config.Defaults) (map[string]any, error) {
	// Convert struct to map using fatih/structs
	structMap := structs.Map(cfg)
	return flattenAndSnakeCase(structMap)
}

// ApplyConfig flattens a config struct and applies all values to the test context.
// It converts the config struct to a flattened map using flattenConfigMap,
// then filters out nil values and keys that match defaults (unchanged values).
// The prefix is prepended to each key (e.g., "plugin.myservice.service.").
// Returns an error if flattening fails or any config value fails to set.
func ApplyConfig(ctx TestContext, prefix string, cfg config.Defaults) error {
	flatConfig, err := flattenConfigMap(cfg)
	if err != nil {
		return err
	}

	// Get defaults to filter out unchanged values
	defaultsMap := cfg.Defaults()
	flatDefaults, err := flattenAndSnakeCase(defaultsMap)
	if err != nil {
		return err
	}

	// Filter out nil values and values that match defaults
	flatConfig = lo.OmitBy(flatConfig, func(key string, value any) bool {
		// Filter out nil values
		if value == nil {
			return true
		}

		// Filter out values that match defaults
		if defaultValue, exists := flatDefaults[key]; exists {
			return reflect.DeepEqual(value, defaultValue)
		}

		return false
	})

	for key, value := range flatConfig {
		fullKey := prefix + key
		if err := setConfigValue(ctx, fullKey, value); err != nil {
			return err
		}
	}
	return nil
}

// envVarName converts a config key to an environment variable name
// by replacing dots with double underscores and converting to uppercase
func envVarName(key string) string {
	return strings.ToUpper(strings.ReplaceAll(key, ".", "__"))
}

// getEnvValue tries to get a value from environment variables in order,
// returning the first value found (empty values are allowed).
// If an env var is not set, the default at the same index in defaultValues is used (if non-nil).
// Environment variable names are converted to uppercase before lookup.
func getEnvValue(envVars []string, defaultValues []interface{}) (string, error) {
	for i, envVar := range envVars {
		value, exists := os.LookupEnv(envVar)
		if exists {
			// Return the value (even if empty) when var is present
			return value, nil
		}

		// Try default value if env var not set
		if i < len(defaultValues) && defaultValues[i] != nil {
			defaultValue := defaultValues[i]
			return fmt.Sprintf("%v", defaultValue), nil
		}
	}

	// If we get here, no env vars were set
	if len(defaultValues) > 0 {
		return "", fmt.Errorf("none of the environment variables %v are set and no valid default values provided", envVars)
	}
	return "", fmt.Errorf("none of the environment variables %v are set", envVars)
}

// WithEnvConfig reads a configuration value from environment variables
// and sets it in the test context. If none of the env vars are set, it logs a
// warning and leaves the config unset (no error). If no envVars provided, the key
// will be converted to an env var name. Empty values are allowed; callers that
// require non-empty values should validate after retrieval or use
// WithEnvConfigOrDefault with a non-empty default.
func WithEnvConfig(key string, envVars ...string) TestContextBuilderOption {
	if len(envVars) == 0 {
		envVars = []string{envVarName(key)}
	}
	return func(ctx TestContext) (TestContext, error) {
		value, err := getEnvValue(envVars, nil)
		if err != nil {
			// Don't fail if no value found—just skip setting
			ctx.Logger().Warn("no environment variables found", zap.String("key", key), zap.Strings("envVars", envVars))
			return ctx, nil
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
			if envVar, ok := arg.(string); ok && envVar != "" {
				envVars = append(envVars, strings.ToUpper(envVar))
			}
		} else {
			// Odd index: default value
			defaultValues = append(defaultValues, arg)
		}
	}

	// If no valid env vars provided (either none or all empty), compute from key
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
