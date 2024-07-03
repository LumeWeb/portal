package config

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-viper/mapstructure/v2"
	"github.com/invopop/jsonschema"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"github.com/samber/lo"
	"github.com/stoewer/go-strcase"
	clientv3 "go.etcd.io/etcd/client/v3"
	"os"
	"path"
	"reflect"
	"strings"
	"sync"
	"time"
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

const CLUSTER_CONFIG_KEY = "config"
const FLAG_NOSYNC = "nosync"
const FLAG_LIVE = "live"

type fieldProcessor func(field reflect.StructField, value reflect.Value, prefix string) error

type Config struct {
	Core   CoreConfig              `mapstructure:"core"`
	Plugin map[string]PluginEntity `mapstructure:"plugin"`
}

type ManagerDefault struct {
	config  *koanf.Koanf
	root    *Config
	changes bool
	lock    sync.RWMutex
	flags   map[string][]string
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
		flags:   make(map[string][]string),
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

	m.lock.Lock()
	defer m.lock.Unlock()

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

	err = m.config.UnmarshalWithConf("core", &m.root.Core, koanf.UnmarshalConf{
		Tag: "mapstructure",
		DecoderConfig: &mapstructure.DecoderConfig{
			DecodeHook:       mapstructure.ComposeDecodeHookFunc(m.hooks()...),
			Metadata:         nil,
			Result:           &m.root.Core,
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

	err = m.processFlags(m.root, "core")
	if err != nil {
		return err
	}

	err = m.maybeConfigureCluster()
	if err != nil {
		return err
	}

	err = m.loadClusterSpace("core")
	if err != nil {
		return err
	}

	err = m.maybeSave()
	if err != nil {
		return err
	}

	err = m.saveClusterSpace("core", false)

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

	err = m.processFlags(m.root, name)
	if err != nil {
		return nil, err
	}

	err = m.loadClusterSpace(name)
	if err != nil {
		return nil, err
	}

	err = m.saveClusterSpace(name, false)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}
func (m *ManagerDefault) setDefaultsForObject(obj interface{}, prefix string) error {
	return m.processObject(obj, prefix, m.defaultProcessor)
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

			m.changes = true
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

func (m *ManagerDefault) validateObject(obj interface{}) error {
	return m.processObject(obj, "", m.validateProcessor)
}

func (m *ManagerDefault) processFlags(obj interface{}, prefix string) error {
	return m.processObject(obj, prefix, m.flagsProcessor)
}

func (m *ManagerDefault) processObject(obj interface{}, prefix string, processors ...fieldProcessor) error {
	objValue := reflect.ValueOf(obj)
	objType := reflect.TypeOf(obj)

	if objValue.Kind() == reflect.Ptr {
		if objValue.IsNil() {
			return nil // Skip processing if the pointer is nil
		}
		objValue = objValue.Elem()
		objType = objType.Elem()
	}

	// Process the object itself
	for _, processor := range processors {
		if err := processor(reflect.StructField{}, objValue, prefix); err != nil {
			return err
		}
	}

	if objValue.Kind() != reflect.Struct {
		return nil // If it's not a struct, we're done
	}

	for i := 0; i < objValue.NumField(); i++ {
		field := objValue.Field(i)
		fieldType := objType.Field(i)

		if !field.CanInterface() {
			continue
		}

		mapstructureTag := fieldType.Tag.Get("mapstructure")
		newPrefix := buildPrefix(prefix, mapstructureTag)

		// Apply all processors to this field
		for _, processor := range processors {
			if err := processor(fieldType, field, newPrefix); err != nil {
				return err
			}
		}

		// Recurse for struct fields or pointers to structs
		switch field.Kind() {
		case reflect.Struct:
			if err := m.processObject(field.Interface(), newPrefix, processors...); err != nil {
				return err
			}
		case reflect.Ptr:
			if !field.IsNil() && field.Elem().Kind() == reflect.Struct {
				if err := m.processObject(field.Interface(), newPrefix, processors...); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (m *ManagerDefault) validateProcessor(_ reflect.StructField, value reflect.Value, _ string) error {
	// Check if the value is a nil pointer
	if value.Kind() == reflect.Ptr && value.IsNil() {
		return nil // Skip validation for nil pointers
	}

	// If it's a pointer, get the element it points to
	if value.Kind() == reflect.Ptr {
		value = value.Elem()
	}

	// Now check if it implements Validator
	if validator, ok := value.Interface().(Validator); ok {
		if err := validator.Validate(); err != nil {
			return err
		}
	}

	return nil
}

func (m *ManagerDefault) defaultProcessor(_ reflect.StructField, value reflect.Value, prefix string) error {
	// Check if the value is a nil pointer
	if value.Kind() == reflect.Ptr && value.IsNil() {
		return nil // Skip defaults for nil pointers
	}

	// If it's a pointer, get the element it points to
	if value.Kind() == reflect.Ptr {
		value = value.Elem()
	}

	if setter, ok := value.Interface().(Defaults); ok {
		if err := m.applyDefaults(setter, prefix); err != nil {
			return err
		}
	}

	return nil
}

func (m *ManagerDefault) flagsProcessor(field reflect.StructField, _ reflect.Value, prefix string) error {
	if flags, ok := field.Tag.Lookup("flags"); ok {
		if flags != "" {
			m.flags[prefix] = strings.Split(flags, ",")
		}
	}

	return nil
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
	if m.root.Core.ClusterEnabled() {
		if m.root.Core.Clustered.Redis != nil {
			if m.root.Core.DB.Cache == nil {
				m.root.Core.DB.Cache = &CacheConfig{}
			}
			m.root.Core.DB.Cache.Mode = "redis"
			m.root.Core.DB.Cache.Options = m.root.Core.Clustered.Redis
		}
	}

	return nil
}

func (m *ManagerDefault) loadClusterSpace(prefix string) error {
	if m.root.Core.ClusterEnabled() && m.root.Core.Clustered.Etcd != nil {
		ctx := context.Background()
		client, err := m.root.Core.Clustered.Etcd.Client()
		if err != nil {
			return err
		}

		key := "/" + CLUSTER_CONFIG_KEY + "/" + strings.ReplaceAll(prefix, ".", "/")

		ret, err := client.Get(ctx, key)
		if err != nil {
			return err
		}

		if ret.Count > 0 {
			remoteConfig := koanf.New(".")
			err := remoteConfig.Load(NewEtcdProvider(client, key, time.Duration(m.root.Core.Clustered.Etcd.DialTimeout)*time.Second), nil)
			if err != nil {
				return err
			}

			m.lock.RLock()
			for k := range m.flags {
				if m.propertyHasFlag(k, FLAG_NOSYNC) {
					remoteConfig.Delete(k)
				}
			}

			err = m.config.Merge(remoteConfig)
			if err != nil {
				return err
			}

			m.changes = true
		}
	}

	return nil
}

func (m *ManagerDefault) saveClusterSpace(prefix string, overwrite bool) error {
	if m.root.Core.ClusterEnabled() && m.root.Core.Clustered.Etcd != nil {
		ctx := context.Background()
		client, err := m.root.Core.Clustered.Etcd.Client()
		if err != nil {
			return err
		}

		etcdKey := "/" + CLUSTER_CONFIG_KEY + "/" + strings.ReplaceAll(prefix, ".", "/")

		ret, err := client.Get(ctx, etcdKey, clientv3.WithPrefix())
		if err != nil {
			return err
		}

		if ret.Count > 0 && !overwrite {
			return nil
		}

		subConfig := m.config.Cut(prefix)

		// Create a map of existing etcd keys and values
		existing := make(map[string]string)
		for _, kv := range ret.Kvs {
			existing[string(kv.Key)] = string(kv.Value)
		}

		for k, v := range subConfig.All() {
			if lo.Contains(m.flags[prefix+"."+k], FLAG_NOSYNC) {
				continue
			}

			key := etcdKey + "/" + k
			key = strings.ReplaceAll(key, ".", "/")

			nval := fmt.Sprintf("%v", v)

			// Check if the key exists and the value is different
			if eval, exists := existing[key]; !exists || eval != nval {
				_, err = client.Put(ctx, key, nval)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (m *ManagerDefault) findProperty(key string) (reflect.Value, error) {
	var ret reflect.Value

	err := m.processObject(m.root, "", func(field reflect.StructField, value reflect.Value, prefix string) error {
		// Check if the value is a nil pointer
		if value.Kind() == reflect.Ptr && value.IsNil() {
			return nil // Skip defaults for nil pointers
		}

		// If it's a pointer, get the element it points to
		if value.Kind() == reflect.Ptr {
			value = value.Elem()
		}

		if prefix == key {
			ret = value
		}
		return nil
	})
	if err != nil {
		return reflect.New(nil), nil
	}

	if ret.Kind() == reflect.Invalid {
		return reflect.New(nil), nil
	}

	return ret, nil
}

func (m *ManagerDefault) Config() *Config {
	return m.root
}

func (m *ManagerDefault) Save() error {
	m.changes = true
	return m.maybeSave()
}

func (m *ManagerDefault) SaveChanged() error {
	if m.changes {
		return m.maybeSave()
	}

	return nil
}

func (m *ManagerDefault) ConfigFile() string {
	return findConfigFile(false, false)
}

func (m *ManagerDefault) ConfigDir() string {
	return path.Dir(m.ConfigFile())
}

func (m *ManagerDefault) LiveConfig() *jsonschema.Schema {
	r := &jsonschema.Reflector{}
	r.KeyNamer = strcase.SnakeCase
	return jsonschema.Reflect(m.root)
}

func (m *ManagerDefault) LiveUpdateProperty(key string, value any) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	// Compute the parent key in dot notation
	parts := strings.Split(key, ".")
	if len(parts) == 0 {
		return fmt.Errorf("invalid key: %s", key)
	}

	existingValue := m.config.Get(key)
	if existingValue == nil {
		return fmt.Errorf("property %s not found", key)
	}

	existingType := reflect.TypeOf(existingValue)
	newType := reflect.TypeOf(value)

	if existingType != newType {
		return fmt.Errorf("type mismatch: expected %s, got %s", existingType, newType)
	}

	if err := m.config.Set(key, value); err != nil {
		return err
	}

	property, err := m.findProperty(key)
	if err != nil {
		return err
	}

	if property.Kind() == reflect.Invalid {
		return fmt.Errorf("property %s not found or is invalid", key)
	}

	if unmarshaler, ok := property.Interface().(mapstructure.Unmarshaler); ok {
		if err := unmarshaler.DecodeMapstructure(value); err != nil {
			return fmt.Errorf("failed to unmarshal: %w", err)
		}
	} else {
		property.Set(reflect.ValueOf(value))
	}

	m.changes = true

	parent := strings.Join(parts[:len(parts)-1], ".")

	if err = m.saveClusterSpace(parent, true); err != nil {
		return fmt.Errorf("failed to save to cluster: %w", err)
	}

	return nil
}

func (m *ManagerDefault) PropertyLiveEditable(property string) bool {
	return !m.propertyHasFlag(property, FLAG_NOSYNC) && m.propertyHasFlag(property, FLAG_LIVE)
}

func (m *ManagerDefault) PropertyLiveReadable(property string) bool {
	return !m.propertyHasFlag(property, FLAG_NOSYNC)
}

func (m *ManagerDefault) PropertyLiveExists(property string) bool {
	return m.config.Exists(property)
}

func (m *ManagerDefault) propertyHasFlag(key string, flag string) bool {
	if lo.Contains(m.flags[key], flag) {
		return true
	}

	return false
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
func buildPrefix(prefix, mapstructureTag string) string {
	if mapstructureTag != "" && mapstructureTag != "-" {
		if prefix != "" {
			return prefix + "." + mapstructureTag
		}
		return mapstructureTag
	}
	return prefix
}
