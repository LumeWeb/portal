package config

import (
	"errors"
	"fmt"
	"github.com/go-viper/mapstructure/v2"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"os"
	"path"
	"reflect"
	"sync"
)

var (
	configFilePaths = []string{
		"/etc/lumeweb/portal/config.yaml",
		"/etc/lumeweb/portal/config.yml",
		"$HOME/.lumeweb/portal/config.yaml",
		"$HOME/.lumeweb/portal/config.yml",
		"./portal.yaml",
		"./portal.yml",
	}
	errConfigFileNotFound = errors.New("config file not found")
)

var _ Manager = (*ManagerDefault)(nil)

type Config struct {
	Core   CoreConfig              `mapstructure:"core"`
	Plugin map[string]PluginEntity `mapstructure:"plugin"`
}

type ManagerDefault struct {
	config  *koanf.Koanf
	root    *Config
	changes bool
	lock    sync.RWMutex
}

func NewManager() (*ManagerDefault, error) {
	k, err := newConfig()
	if err != nil && err != errConfigFileNotFound {
		return nil, err
	}

	exists := err == nil

	return &ManagerDefault{
		config:  k,
		changes: !exists,
		lock:    sync.RWMutex{},
	}, nil
}

func (m *ManagerDefault) hooks() []mapstructure.DecodeHookFunc {
	return []mapstructure.DecodeHookFunc{
		clusterConfigHook(),
		cacheConfigHook(m),
	}
}

func (m *ManagerDefault) Init() error {
	m.root = &Config{}

	err := m.setDefaultsForObject(m.root.Core, "core")
	if err != nil {
		return err
	}
	err = m.maybeSave()
	if err != nil {
		return err
	}

	hooks := m.hooks()
	hooks = append(hooks, mapstructure.StringToTimeDurationHookFunc())

	err = m.config.UnmarshalWithConf("", &m.root, koanf.UnmarshalConf{
		Tag: "mapstructure",
		DecoderConfig: &mapstructure.DecoderConfig{
			DecodeHook:       mapstructure.ComposeDecodeHookFunc(m.hooks()...),
			Metadata:         nil,
			Result:           &m.root,
			WeaklyTypedInput: true,
		},
	})
	if err != nil {
		return err
	}

	err = m.validateObject(m.root)
	if err != nil {
		return err
	}

	err = m.maybeConfigureCluster()
	if err != nil {
		return err
	}

	return nil
}

func (m *ManagerDefault) ConfigureProtocol(pluginName string, cfg ProtocolConfig) error {
	if cfg == nil {
		return nil
	}

	m.initPlugin(pluginName)
	m.lock.Lock()
	defer m.lock.Unlock()

	prefix := fmt.Sprintf("plugin.%s.protocol", pluginName)
	section, err := m.configureSection(prefix, cfg)
	if err != nil {
		return err
	}

	pluginEntity := m.root.Plugin[pluginName]
	pluginEntity.Protocol = section.(ProtocolConfig)
	m.root.Plugin[pluginName] = pluginEntity

	return nil
}

func (m *ManagerDefault) ConfigureAPI(pluginName string, cfg APIConfig) error {
	if cfg == nil {
		return nil
	}

	m.initPlugin(pluginName)
	m.lock.Lock()
	defer m.lock.Unlock()

	prefix := fmt.Sprintf("plugin.%s.api", pluginName)
	section, err := m.configureSection(prefix, cfg)
	if err != nil {
		return err
	}

	pluginEntity := m.root.Plugin[pluginName]
	pluginEntity.API = section.(APIConfig)
	m.root.Plugin[pluginName] = pluginEntity

	return nil
}

func (m *ManagerDefault) ConfigureService(pluginName string, serviceName string, cfg ServiceConfig) error {
	if cfg == nil {
		return nil
	}

	m.initPlugin(pluginName)
	m.lock.Lock()
	defer m.lock.Unlock()
	prefix := fmt.Sprintf("plugin.%s.service.%s", pluginName, serviceName)
	section, err := m.configureSection(prefix, cfg)

	if err != nil {
		return err
	}

	m.root.Plugin[pluginName].Service[serviceName] = section.(ServiceConfig)
	return nil
}

func (m *ManagerDefault) GetPlugin(pluginName string) *PluginEntity {
	m.lock.RLock()
	defer m.lock.RUnlock()

	if plugin, ok := m.root.Plugin[pluginName]; ok {
		return &plugin
	}

	return nil
}

func (m *ManagerDefault) GetService(serviceName string) ServiceConfig {
	m.lock.RLock()
	defer m.lock.RUnlock()
	for _, plugin := range m.root.Plugin {
		if service, ok := plugin.Service[serviceName]; ok {
			return service
		}
	}

	return nil
}

func (m *ManagerDefault) GetAPI(pluginName string) APIConfig {
	m.lock.RLock()
	defer m.lock.RUnlock()
	if plugin, ok := m.root.Plugin[pluginName]; ok {
		return plugin.API
	}

	return nil
}

func (m *ManagerDefault) GetProtocol(pluginName string) ProtocolConfig {
	m.lock.RLock()
	defer m.lock.RUnlock()
	if plugin, ok := m.root.Plugin[pluginName]; ok {
		return plugin.Protocol
	}

	return nil
}

func (m *ManagerDefault) initPlugin(name string) {
	m.lock.Lock()
	defer m.lock.Unlock()
	if m.root.Plugin == nil {
		m.root.Plugin = make(map[string]PluginEntity)
	}

	if _, ok := m.root.Plugin[name]; ok {
		return
	}

	m.root.Plugin[name] = PluginEntity{
		Service: make(map[string]ServiceConfig),
	}
}

func (m *ManagerDefault) configureSection(name string, cfg Defaults) (Defaults, error) {
	err := m.setDefaultsForObject(cfg, name)
	if err != nil {
		return nil, err
	}
	err = m.maybeSave()
	if err != nil {
		return nil, err
	}

	hooks := append([]mapstructure.DecodeHookFunc{}, mapstructure.StringToTimeDurationHookFunc())

	err = m.config.UnmarshalWithConf(name, cfg, koanf.UnmarshalConf{
		Tag: "mapstructure",
		DecoderConfig: &mapstructure.DecoderConfig{
			DecodeHook:           mapstructure.ComposeDecodeHookFunc(hooks...),
			Metadata:             nil,
			Result:               cfg,
			WeaklyTypedInput:     true,
			IgnoreUntaggedFields: true,
		},
	})
	if err != nil {
		return nil, err
	}

	err = m.validateObject(cfg)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func (m *ManagerDefault) setDefaultsForObject(obj interface{}, prefix string) error {
	// Reflect on the object to traverse its fields
	objValue := reflect.ValueOf(obj)
	objType := reflect.TypeOf(obj)

	// If the object is a pointer, we need to work with its element
	if objValue.Kind() == reflect.Ptr {
		objValue = objValue.Elem()
		objType = objType.Elem()
	}

	// Check if the object itself implements Defaults
	if setter, ok := obj.(Defaults); ok {
		err := m.applyDefaults(setter, prefix)
		if err != nil {
			return err
		}
	}

	// Recursively handle struct fields
	for i := 0; i < objValue.NumField(); i++ {
		field := objValue.Field(i)
		fieldType := objType.Field(i)

		// Check if the field is exported and can be interfaced
		if !field.CanInterface() {
			continue
		}

		mapstructureTag := fieldType.Tag.Get("mapstructure")

		// Construct new prefix based on the mapstructure tag, if available
		newPrefix := prefix
		if mapstructureTag != "" && mapstructureTag != "-" {
			if newPrefix != "" {
				newPrefix += "."
			}
			newPrefix += mapstructureTag
		}

		// If field is a struct or pointer to a struct, recurse
		if field.Kind() == reflect.Struct || (field.Kind() == reflect.Ptr && field.Elem().Kind() == reflect.Struct) {
			if field.Kind() == reflect.Ptr && field.IsNil() {
				// Initialize nil pointer to struct
				field.Set(reflect.New(fieldType.Type.Elem()))
			}
			err := m.setDefaultsForObject(field.Interface(), newPrefix)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (m *ManagerDefault) validateObject(obj interface{}) error {
	// Reflect on the object to traverse its fields
	objValue := reflect.ValueOf(obj)
	objType := reflect.TypeOf(obj)

	// If the object is a pointer, we need to work with its element
	if objValue.Kind() == reflect.Ptr {
		objValue = objValue.Elem()
		objType = objType.Elem()
	}

	// Check if the object itself implements Defaults
	if validator, ok := obj.(Validator); ok {
		err := validator.Validate()
		if err != nil {
			return err
		}
	}

	// Recursively handle struct fields
	for i := 0; i < objValue.NumField(); i++ {
		field := objValue.Field(i)
		fieldType := objType.Field(i)

		if !field.CanInterface() {
			continue
		}

		// If field is a struct or pointer to a struct, recurse
		if field.Kind() == reflect.Struct || (field.Kind() == reflect.Ptr && field.Elem().Kind() == reflect.Struct) {
			if field.Kind() == reflect.Ptr && field.IsNil() {
				// Initialize nil pointer to struct
				field.Set(reflect.New(fieldType.Type.Elem()))
			}
			err := m.validateObject(field.Interface())
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (m *ManagerDefault) applyDefaults(setter Defaults, prefix string) error {
	defaults := setter.Defaults()
	for key, value := range defaults {
		fullKey := key
		if prefix != "" {
			fullKey = fmt.Sprintf("%s.%s", prefix, key)
		}
		if ret, err := m.setDefault(fullKey, value); err != nil || ret {
			if err != nil {
				return err
			}

			if ret {
				m.changes = true
			}
		}
	}

	return nil
}

func (m *ManagerDefault) setDefault(key string, value interface{}) (bool, error) {
	if !m.config.Exists(key) {
		err := m.config.Set(key, value)
		if err != nil {
			return false, err
		}
		return true, nil
	}

	return false, nil
}

func (m *ManagerDefault) maybeSave() error {
	if m.changes {
		data, err := m.config.Marshal(yaml.Parser())
		if err != nil {
			return err
		}

		configFile := findConfigFile(true, true)

		err = os.MkdirAll(path.Dir(configFile), 0755)
		if err != nil {
			return err
		}
		err = os.WriteFile(configFile, data, 0644)

		if err != nil {
			return err
		}

		m.changes = false
	}

	return nil
}

func (m *ManagerDefault) maybeConfigureCluster() error {
	if m.root.Core.Clustered != nil && m.root.Core.Clustered.Enabled {
		if m.root.Core.DB.Cache == nil {
			m.root.Core.DB.Cache = &CacheConfig{}
		}
		m.root.Core.DB.Cache.Mode = "redis"
		m.root.Core.DB.Cache.Options = m.root.Core.Clustered.Redis
	}

	return nil
}

func (m *ManagerDefault) Config() *Config {
	return m.root
}

func (m *ManagerDefault) Save() error {
	m.changes = true
	return m.maybeSave()
}

func (m *ManagerDefault) ConfigFile() string {
	return findConfigFile(false, false)
}

func (m *ManagerDefault) ConfigDir() string {
	return path.Dir(m.ConfigFile())
}

func newConfig() (*koanf.Koanf, error) {
	k := koanf.New(".")

	configFile := findConfigFile(false, false)

	if configFile == "" {
		return k, errConfigFileNotFound
	}

	if err := k.Load(file.Provider(configFile), yaml.Parser()); err != nil {
		return nil, err
	}

	return k, nil
}

func findConfigFile(dirCheck bool, ignoreExist bool) string {
	for _, _path := range configFilePaths {
		expandedPath := os.ExpandEnv(_path)
		_, err := os.Stat(expandedPath)
		if err == nil {
			return expandedPath
		} else if os.IsNotExist(err) {
			if dirCheck {
				_, err := os.Stat(path.Dir(expandedPath))
				if err == nil || ignoreExist {
					return expandedPath
				}
			}

			return ""
		}
	}

	return ""
}
