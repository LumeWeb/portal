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
	Core     CoreConfig             `mapstructure:"core"`
	Protocol map[string]interface{} `mapstructure:"protocol"`
}

type ManagerDefault struct {
	config  *koanf.Koanf
	root    *Config
	changes bool
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

func (m *ManagerDefault) ConfigureProtocol(name string, cfg ProtocolConfig) error {
	protocolPrefix := fmt.Sprintf("protocol.%s", name)

	err := m.setDefaultsForObject(cfg, protocolPrefix)
	if err != nil {
		return err
	}
	err = m.maybeSave()
	if err != nil {
		return err
	}

	hooks := append([]mapstructure.DecodeHookFunc{}, mapstructure.StringToTimeDurationHookFunc())

	err = m.config.UnmarshalWithConf(protocolPrefix, cfg, koanf.UnmarshalConf{
		Tag: "mapstructure",
		DecoderConfig: &mapstructure.DecoderConfig{
			DecodeHook:       mapstructure.ComposeDecodeHookFunc(hooks...),
			Metadata:         nil,
			Result:           cfg,
			WeaklyTypedInput: true,
		},
	})
	if err != nil {
		return err
	}

	err = m.validateObject(cfg)
	if err != nil {
		return err
	}

	if m.root.Protocol == nil {
		m.root.Protocol = make(map[string]interface{})
	}

	m.root.Protocol[name] = cfg

	return nil
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
