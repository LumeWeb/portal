package config

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

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

type Defaults interface {
	Defaults() map[string]interface{}
}

type Validator interface {
	Validate() error
}

type Config struct {
	Core     CoreConfig             `mapstructure:"core"`
	Protocol map[string]interface{} `mapstructure:"protocol"`
}

type Manager struct {
	viper   *viper.Viper
	root    *Config
	changes bool
}

func NewManager() (*Manager, error) {
	v, err := newConfig()
	if err != nil {
		return nil, err
	}

	var config Config

	m := &Manager{
		viper: v,
		root:  &config,
	}

	m.setDefaultsForObject(m.root.Core, "core")
	err = m.maybeSave()
	if err != nil {
		return nil, err
	}

	err = v.Unmarshal(&config, viper.DecodeHook(clusterConfigHook()), viper.DecodeHook(cacheConfigHook()))
	if err != nil {
		return nil, err
	}

	err = m.validateObject(m.root)
	if err != nil {
		return nil, err
	}

	err = m.maybeConfigureCluster()
	if err != nil {
		return m, err
	}

	return m, nil
}

func (m *Manager) ConfigureProtocol(name string, cfg ProtocolConfig) error {
	protocolPrefix := fmt.Sprintf("protocol.%s", name)

	m.setDefaultsForObject(cfg, protocolPrefix)
	err := m.maybeSave()
	if err != nil {
		return err
	}

	err = m.viper.Sub(protocolPrefix).Unmarshal(cfg)
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

func (m *Manager) setDefaultsForObject(obj interface{}, prefix string) {
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
		m.applyDefaults(setter, prefix)
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
			m.setDefaultsForObject(field.Interface(), newPrefix)
		}
	}
}

func (m *Manager) validateObject(obj interface{}) error {
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

func (m *Manager) applyDefaults(setter Defaults, prefix string) {
	defaults := setter.Defaults()
	for key, value := range defaults {
		fullKey := key
		if prefix != "" {
			fullKey = fmt.Sprintf("%s.%s", prefix, key)
		}
		if m.setDefault(fullKey, value) {
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

func (m *Manager) maybeConfigureCluster() error {
	if m.root.Core.Clustered != nil && m.root.Core.Clustered.Enabled {
		m.root.Core.DB.Cache.Mode = "redis"
		m.root.Core.DB.Cache.Options = m.root.Core.Clustered.Redis
	}

	return nil
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
func (m *Manager) LiveConfig() map[string]interface{} {
	keys := m.GetEditableConfigKeys()

	liveConfig := make(map[string]interface{})

	for _, key := range keys {
		liveConfig[key] = m.viper.Get(key)
	}

	return liveConfig
}

func (m *Manager) UpdateLiveConfig(setting string, value interface{}) error {
	// Find the corresponding field in the configuration struct
	field, err := m.findConfigField(setting)
	if err != nil {
		return err
	}

	// Check if the value type matches the field type
	if !reflect.TypeOf(value).AssignableTo(field.Type) {
		return fmt.Errorf("invalid type for setting %s: expected %s, got %s", setting, field.Type, reflect.TypeOf(value))
	}

	// Update the value in the Viper store
	m.viper.Set(setting, value)
	m.changes = true

	err = m.maybeSave()
	if err != nil {
		return err
	}

	return nil
}

func (m *Manager) GetEditableConfigKeys() []string {
	var keys []string
	m.traverseConfig(reflect.ValueOf(m.root), "", func(key string, field reflect.StructField) {
		if isFieldEditable(field) {
			keys = append(keys, key)
		}
	})
	return keys
}

func (m *Manager) traverseConfig(value reflect.Value, prefix string, callback func(string, reflect.StructField)) {
	if value.Kind() == reflect.Ptr {
		value = value.Elem()
	}

	if value.Kind() != reflect.Struct {
		return
	}

	for i := 0; i < value.NumField(); i++ {
		field := value.Type().Field(i)
		if field.PkgPath != "" {
			// Skip unexported fields
			continue
		}

		tag := field.Tag.Get("mapstructure")
		if tag == "-" {
			// Skip fields with "-" tag
			continue
		}

		fieldValue := value.Field(i)
		if fieldValue.Kind() == reflect.Ptr {
			if fieldValue.IsNil() {
				continue
			}
			fieldValue = fieldValue.Elem()
		}

		if fieldValue.Kind() == reflect.Struct {
			// Recursively traverse nested structs
			m.traverseConfig(fieldValue, prefix+tag+".", callback)
		} else {
			callback(prefix+tag, field)
		}
	}
}

func (m *Manager) findConfigField(setting string) (reflect.StructField, error) {
	parts := strings.Split(setting, ".")
	var structValue reflect.Value = reflect.ValueOf(m.root)

	for _, part := range parts {
		if structValue.Kind() == reflect.Ptr {
			structValue = structValue.Elem()
		}

		if structValue.Kind() != reflect.Struct {
			return reflect.StructField{}, fmt.Errorf("invalid setting: %s", setting)
		}

		field, ok := structValue.Type().FieldByNameFunc(func(name string) bool {
			return strings.EqualFold(name, part)
		})

		if !ok {
			return reflect.StructField{}, fmt.Errorf("setting not found: %s", setting)
		}

		structValue = structValue.FieldByIndex(field.Index)
	}

	if structValue.Kind() == reflect.Ptr {
		structValue = structValue.Elem()
	}

	if structValue.Kind() != reflect.Struct {
		return structValue.Type().Field(0), nil
	}

	return reflect.StructField{}, fmt.Errorf("setting not found: %s", setting)
}

func isFieldEditable(field reflect.StructField) bool {
	editable := field.Tag.Get("editable")
	return editable == "true"
}

func newConfig() (*viper.Viper, error) {
	logger := newFallbackLogger()

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	for _, path := range ConfigFilePaths {
		viper.AddConfigPath(path)
	}

	viper.SetEnvPrefix("LUME_WEB_PORTAL")
	viper.AutomaticEnv()

	err := viper.ReadInConfig()
	if err != nil {
		if !errors.As(err, &viper.ConfigFileNotFoundError{}) {
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
func newFallbackLogger() *zap.Logger {
	l, _ := zap.NewDevelopment()

	return l
}
