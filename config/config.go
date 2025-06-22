package config

import (
	"fmt"
	"github.com/Oudwins/zog"
	"github.com/Oudwins/zog/conf"
	"go.lumeweb.com/configmanager"
	"go.lumeweb.com/configmanager/source"
	"go.uber.org/zap"
)

type Defaults = source.ConfigDefaults

type Validator interface {
	Validate() error
}

type ConfigSchemaProvider = configmanager.ConfigSchemaProvider

type Manager interface {
	configmanager.Manager

	// Custom methods specific to our implementation
	Init() error
	SetLogger(logger *zap.Logger)
	Config() *Config
	EnableSync(opts ...configmanager.ConfigOption) error

	// Plugin configuration methods
	ConfigureProtocol(pluginName string, cfg ProtocolConfig) error
	ConfigureAPI(pluginName string, cfg APIConfig) error
	ConfigureService(pluginName string, serviceName string, cfg ServiceConfig) error
	GetService(pluginName string, serviceName string) ServiceConfig
	GetProtocol(pluginName string) ProtocolConfig
	GetAPI(pluginName string) APIConfig
}

type Config struct {
	Core   CoreConfig              `config:"core"`
	Plugin map[string]PluginEntity `config:"plugin"`
}

func ZogUInt(opts ...zog.SchemaOption) *zog.NumberSchema[uint] {
	s := &zog.NumberSchema[uint]{}

	// Custom coercer to handle the type alias conversion during coercion.
	customCoercer := func(data any) (any, error) {
		num, err := conf.Coercers.Int(data) // Use the default string coercer first
		if err != nil {
			return nil, err
		}

		// Ensure the value is non-negative for unsigned types
		if num.(int) < 0 {
			return nil, fmt.Errorf("value must be non-negative, got %d", num.(int))
		}

		return num, nil
	}

	opts = append(opts, zog.WithCoercer(customCoercer))

	for _, opt := range opts {
		opt(s)
	}
	return s
}

func ZogUInt64(opts ...zog.SchemaOption) *zog.NumberSchema[uint64] {
	s := &zog.NumberSchema[uint64]{}

	// Custom coercer to handle the type alias conversion during coercion.
	customCoercer := func(data any) (any, error) {
		num, err := conf.Coercers.Int(data) // Use the default string coercer first
		if err != nil {
			return nil, err
		}

		// Ensure the value is non-negative for unsigned types
		if num.(int) < 0 {
			return nil, fmt.Errorf("value must be non-negative, got %d", num.(int))
		}

		return num, nil
	}

	opts = append(opts, zog.WithCoercer(customCoercer))

	for _, opt := range opts {
		opt(s)
	}
	return s
}
