package config

import (
	"errors"
	"fmt"

	_logger "git.lumeweb.com/LumeWeb/portal/logger"
	"github.com/docker/go-units"
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

var (
	ConfigFilePaths = []string{
		"/etc/lumeweb/portal/",
		"$HOME/.lumeweb/portal/",
		".",
	}
)

type Config struct {
	Core CoreConfig `mapstructure:"core"`
}

type Manager struct {
	viper   *viper.Viper
	root    *Config
	changes bool
}

func NewManager(logger *zap.Logger) (*Manager, error) {
	v, err := newConfig(logger)
	if err != nil {
		return nil, err
	}

	var config Config

	m := &Manager{
		viper: v,
		root:  &config,
	}

	m.setDefaults(m.coreDefaults(), "")
	err = m.maybeSave()
	if err != nil {
		return nil, err
	}

	err = v.Unmarshal(&config)
	if err != nil {
		return nil, err
	}

	return m, nil
}

func (m *Manager) ConfigureProtocol(name string, cfg ProtocolConfig) error {
	defaults := cfg.Defaults()

	m.setDefaults(defaults, fmt.Sprintf("protocol.%s", name))
	err := m.maybeSave()
	if err != nil {
		return err
	}

	return m.viper.Unmarshal(cfg)
}

func (m *Manager) setDefaults(defaults map[string]interface{}, prefix string) {
	for key, value := range defaults {
		if prefix != "" {
			key = fmt.Sprintf("%s.%s", prefix, key)
		}
		if m.setDefault(key, value) {
			m.changes = true
		}
	}
}

func (m *Manager) setDefault(key string, value interface{}) bool {
	if !m.viper.IsSet(key) {
		m.viper.SetDefault(key, value)
		return true
	}

	return false
}

func (m *Manager) maybeSave() error {
	if m.changes {
		ret := m.viper.WriteConfig()
		if ret != nil {
			return ret
		}
		m.changes = false
	}

	return nil
}

func (m *Manager) coreDefaults() map[string]interface{} {
	return map[string]interface{}{
		"core.post-upload-limit": units.MiB * 100,
		"core.log.level":         "info",
		"core.db.charset":        "utf8mb4",
		"core.db.port":           3306,
		"core.db.name":           "portal",
	}
}

func (m *Manager) Config() *Config {
	return m.root
}

func (m *Manager) Viper() *viper.Viper {
	return m.viper
}

func (m *Manager) Save() error {
	err := m.viper.WriteConfig()
	if err != nil {
		return err
	}

	err = m.viper.Unmarshal(&m.root)
	if err != nil {
		return err
	}

	return nil
}

func newConfig(logger *zap.Logger) (*viper.Viper, error) {
	if logger == nil {
		logger = _logger.NewFallbackLogger()
	}

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	for _, path := range ConfigFilePaths {
		viper.AddConfigPath(path)
	}

	viper.SetEnvPrefix("LUME_WEB_PORTAL")
	viper.AutomaticEnv()

	err := viper.ReadInConfig()
	if err != nil {
		if !errors.Is(err, &viper.ConfigFileNotFoundError{}) {
			return nil, err
		}

		logger.Info("Config file not found, using default settings.")
		err := viper.SafeWriteConfig()
		if err != nil {
			return nil, err
		}

		return viper.GetViper(), nil

	}

	return viper.GetViper(), nil
}
