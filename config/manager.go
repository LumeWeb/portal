package config

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-viper/mapstructure/v2"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"github.com/samber/lo"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
	"os"
	"path"
	"path/filepath"
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

type fieldProcessor func(field reflect.StructField, value reflect.Value, prefix string) error

type Config struct {
	Core   CoreConfig              `mapstructure:"core"`
	Plugin map[string]PluginEntity `mapstructure:"plugin"`
}

type ManagerDefault struct {
	config          *koanf.Koanf
	root            *Config
	changes         bool
	lock            sync.RWMutex
	flags           map[string][]string
	changeCallbacks []ConfigChangeCallback
	logger          *zap.Logger
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

func (m *ManagerDefault) SetLogger(logger *zap.Logger) {
	m.logger = logger
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

func (m *ManagerDefault) RegisterConfigChangeCallback(callback ConfigChangeCallback) {
	m.changeCallbacks = append(m.changeCallbacks, callback)
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
		if configFile == "" {
			return fmt.Errorf("no writable configuration file location found")
		}

		err = os.MkdirAll(filepath.Dir(configFile), 0755)
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
			for k, flags := range m.flags {
				if lo.Contains(flags, FLAG_NOSYNC) {
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

		for k, v := range subConfig.All() {
			if lo.Contains(m.flags[prefix+"."+k], FLAG_NOSYNC) {
				continue
			}

			key := etcdKey + "/" + k
			key = strings.ReplaceAll(key, ".", "/")

			_, err = client.Put(ctx, key, fmt.Sprintf("%v", v))
			if err != nil {
				return err
			}
		}
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

func findHighestWritableDir(path string) (string, error) {
	dir := path
	for {
		// Try to create a temporary file in the directory
		tempFile, err := os.CreateTemp(dir, ".write_test")
		if err == nil {
			tempFile.Close()
			os.Remove(tempFile.Name())
			return dir, nil
		}

		if !os.IsPermission(err) && !os.IsNotExist(err) {
			return "", err
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// We've reached the root directory
			return "", fmt.Errorf("no writable directory found")
		}
		dir = parent
	}
}

func findConfigFile(dirCheck bool, ignoreExist bool) string {
	for _, _path := range configFilePaths {
		expandedPath := os.ExpandEnv(_path)

		// First, check if the file exists
		_, err := os.Stat(expandedPath)
		if err == nil {
			// File exists, check if we can write to it
			file, err := os.OpenFile(expandedPath, os.O_WRONLY, 0644)
			if err == nil {
				file.Close()
				return expandedPath
			}
			// Can't write to existing file, continue to next path
			continue
		}

		// File doesn't exist
		if os.IsNotExist(err) {
			if !ignoreExist {
				continue
			}

			// Check if we can create the file
			if dirCheck {
				_, err := findHighestWritableDir(filepath.Dir(expandedPath))
				if err == nil {
					// We found a writable directory in the path
					return expandedPath
				}
				continue
			}

			// If we can't find a writable directory or dirCheck is false,
			// but ignoreExist is true, we still return the path
			return expandedPath
		}

		// Some other error occurred (e.g., permission denied), skip this path
		continue
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
