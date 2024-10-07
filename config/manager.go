package config

import (
	"context"
	"fmt"
	"github.com/go-viper/mapstructure/v2"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"github.com/samber/lo"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"
	"log"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
)

// Constants
const (
	CLUSTER_CONFIG_KEY = "config"
	FLAG_NOSYNC        = "nosync"
	CONFIG_EXTENSION   = ".yaml"

	CoreConfigFile    = "core" + CONFIG_EXTENSION
	SectionConfigFile = "default" + CONFIG_EXTENSION
	PluginsDir        = "plugins.d"
	ProtoDir          = "proto.d"
	ServiceDir        = "service.d"
	APIDir            = "api.d"

	protoSectionSpecifier   = "plugin.%s.protocol"
	apiSectionSpecifier     = "plugin.%s.api"
	serviceSectionSpecifier = "plugin.%s.service.%s"
)

var (
	configDirPaths = []string{
		"/etc/lumeweb/portal",
		"$HOME/.lumeweb/portal",
		"./portal.yaml",
		"./",
	}
)

type sectionKind int

const (
	sectionKindAPI sectionKind = iota
	sectionKindService
	sectionKindProtocol
)

type fieldProcessor func(field reflect.StructField, value reflect.Value, prefix string) error

type Config struct {
	Core   CoreConfig              `mapstructure:"core"`
	Plugin map[string]PluginEntity `mapstructure:"plugin"`
}

type ManagerDefault struct {
	config          *koanf.Koanf
	root            *Config
	changes         bool
	changedSections []string
	lock            sync.RWMutex
	flags           map[string][]string
	changeCallbacks []ConfigChangeCallback
	logger          *zap.Logger
	configFile      string
}

var _ Manager = (*ManagerDefault)(nil)

func NewManager() (*ManagerDefault, error) {
	k := koanf.New(".")

	return &ManagerDefault{
		config:          k,
		changes:         false,
		lock:            sync.RWMutex{},
		flags:           make(map[string][]string),
		changedSections: make([]string, 0),
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

	// Initialize config directory
	if err := m.initConfigLocation(); err != nil {
		return fmt.Errorf("failed to initialize config directory: %w", err)
	}

	coreCfg := koanf.New(".")
	err := coreCfg.Load(file.Provider(m.configFile), yaml.Parser())
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to load core config: %w", err)
		}

		if err := m.setDefaultsForObject(&m.root.Core, "core"); err != nil {
			return fmt.Errorf("failed to set core defaults: %w", err)
		}

		if err := m.maybeSave(); err != nil {
			return fmt.Errorf("failed to save config changes: %w", err)
		}
	} else {
		err = m.config.MergeAt(coreCfg, "core")
		if err != nil {
			return err
		}
	}

	_, err = m.configureSection("core", &m.root.Core)
	if err != nil {
		return err
	}
	return nil
}

func (m *ManagerDefault) RegisterConfigChangeCallback(callback ConfigChangeCallback) {
	m.changeCallbacks = append(m.changeCallbacks, callback)
}

func (m *ManagerDefault) ConfigureProtocol(pluginName string, cfg ProtocolConfig) error {
	if cfg == nil {
		return nil
	}

	m.lock.Lock()
	defer m.lock.Unlock()

	m.initPlugin(pluginName)

	err := m.loadSection(pluginName, "", sectionKindProtocol)
	if err != nil {
		return err
	}

	prefix := getProtoSectionSpecifier(pluginName)
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

	err := m.loadSection(pluginName, "", sectionKindAPI)
	if err != nil {
		return err
	}

	prefix := getAPISectionSpecifier(pluginName)
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

	m.lock.Lock()
	defer m.lock.Unlock()

	m.initPlugin(pluginName)

	err := m.loadSection(pluginName, serviceName, sectionKindService)
	if err != nil {
		return err
	}

	prefix := getServiceSectionSpecifier(pluginName, serviceName)
	section, err := m.configureSection(prefix, cfg)
	if err != nil {
		return err
	}

	m.root.Plugin[pluginName].Service[serviceName] = section.(ServiceConfig)

	return nil
}

func (m *ManagerDefault) GetPlugin(pluginName string) *PluginEntity {
	if m.root.Plugin == nil {
		return nil
	}

	m.lock.RLock()
	defer m.lock.RUnlock()

	plugin, exists := m.root.Plugin[pluginName]

	if exists {
		return &plugin
	}

	return nil
}
func (m *ManagerDefault) GetService(serviceName string) ServiceConfig {
	if m.root.Plugin == nil {
		return nil
	}

	m.lock.RLock()
	defer m.lock.RUnlock()

	for _, plugin := range m.root.Plugin {
		if service, exists := plugin.Service[serviceName]; exists {
			return service
		}
	}

	return nil
}

func (m *ManagerDefault) GetAPI(pluginName string) APIConfig {
	m.lock.RLock()
	defer m.lock.RUnlock()

	if m.root.Plugin == nil {
		return nil
	}

	if plugin, exists := m.root.Plugin[pluginName]; exists {
		return plugin.API
	}

	return nil
}

func (m *ManagerDefault) GetProtocol(pluginName string) ProtocolConfig {
	m.lock.RLock()
	defer m.lock.RUnlock()

	if m.root.Plugin == nil {
		return nil
	}

	if plugin, exists := m.root.Plugin[pluginName]; exists {
		return plugin.Protocol
	}

	return nil
}

func (m *ManagerDefault) initConfigLocation() error {
	if m.configFile != "" {
		return nil
	}

	configFile := findConfigFile(true, true)
	if configFile == "" {
		return fmt.Errorf("no configuration file found")
	}
	m.configFile = configFile
	return nil
}

func (m *ManagerDefault) initPlugin(name string) {
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

func (m *ManagerDefault) loadSection(pluginName string, name string, kind sectionKind) error {
	basePath := m.ConfigDir()

	config := koanf.New(".")
	var err error
	var configPath string
	var target string

	switch kind {
	case sectionKindAPI:
		configPath = path.Join(basePath, PluginsDir, pluginName, APIDir, SectionConfigFile)
		target = getAPISectionSpecifier(pluginName)
	case sectionKindService:
		configPath = path.Join(basePath, PluginsDir, pluginName, ServiceDir, name+CONFIG_EXTENSION)
		target = getServiceSectionSpecifier(pluginName, name)
	case sectionKindProtocol:
		configPath = path.Join(basePath, PluginsDir, pluginName, ProtoDir, SectionConfigFile)
		target = getProtoSectionSpecifier(pluginName)

	}

	err = config.Load(file.Provider(configPath), yaml.Parser())
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("failed to load config: %w", err)
		}

		return nil
	}

	err = m.config.MergeAt(config, target)
	if err != nil {
		return err
	}

	return nil
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
			m.changedSections = append(m.changedSections, prefix)
		}
	}

	return nil
}

func (m *ManagerDefault) setDefault(key string, value interface{}) (bool, error) {
	if !m.config.Exists(key) || (m.config.Get(key) == nil && value != nil) {
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
	if !m.changes {
		return nil
	}

	m.changedSections = lo.Uniq(m.changedSections)

	if err := m.saveCoreConfig(); err != nil {
		return err
	}

	if err := m.savePluginConfigs(); err != nil {
		return err
	}

	m.changes = false
	m.changedSections = make([]string, 0)
	return nil
}

// New function to save core config
func (m *ManagerDefault) saveCoreConfig() error {
	if len(m.changedSections) > 0 {
		if !m.hasPrefixChanged("core") {
			return nil
		}
	}

	coreConfigPath := m.configFile
	coreConfig := m.config.Cut("core")
	return m.saveConfigToFile(coreConfig, coreConfigPath)
}

func (m *ManagerDefault) hasSectionChanged(section string) bool {
	if len(m.changedSections) == 0 {
		return true
	}
	for _, changed := range m.changedSections {
		if section == changed {
			return true
		}
	}

	return false
}

func (m *ManagerDefault) hasPrefixChanged(prefix string) bool {
	if len(m.changedSections) == 0 {
		return true
	}
	for _, changed := range m.changedSections {
		if strings.HasPrefix(changed, prefix+".") {
			return true
		}
	}

	return false
}

func (m *ManagerDefault) saveConfigToFile(config *koanf.Koanf, filePath string) error {
	data, err := config.Marshal(yaml.Parser())
	if err != nil {
		return fmt.Errorf("error marshaling config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("error creating directory: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}

	return nil
}

// New function to save plugin configs
func (m *ManagerDefault) savePluginConfigs() error {
	pluginsPath := filepath.Join(m.ConfigDir(), PluginsDir)
	plugins := m.config.Cut("plugin")

	for _, pluginName := range plugins.MapKeys("") {
		pluginPath := filepath.Join(pluginsPath, pluginName)

		services := plugins.Cut(pluginName).Cut("service")
		api := plugins.Cut(pluginName).Cut("api")
		protocol := plugins.Cut(pluginName).Cut("protocol")

		if m.hasSectionChanged(getProtoSectionSpecifier(pluginName)) && len(protocol.Keys()) > 0 {
			if err := m.saveConfigToFile(protocol, filepath.Join(pluginPath, ProtoDir, SectionConfigFile)); err != nil {
				return err
			}
		}

		for _, svcName := range services.MapKeys("") {
			if m.hasSectionChanged(getServiceSectionSpecifier(pluginName, svcName)) && len(services.Keys()) > 0 {
				if err := m.saveConfigToFile(services.Cut(svcName), filepath.Join(pluginPath, ServiceDir, svcName+CONFIG_EXTENSION)); err != nil {
					return err
				}
			}
		}
		if m.hasSectionChanged(getAPISectionSpecifier(pluginName)) && len(api.Keys()) > 0 {
			if err := m.saveConfigToFile(api, filepath.Join(pluginPath, APIDir, SectionConfigFile)); err != nil {
				return err
			}
		}
	}

	return nil
}

func (m *ManagerDefault) maybeConfigureCluster() error {
	if m.root.Core.ClusterEnabled() {
		if m.root.Core.Clustered.RedisEnabled() {
			if m.root.Core.DB.Cache == nil {
				m.root.Core.DB.Cache = &CacheConfig{}
			}

			if m.root.Core.DB.Cache.Mode == "redis" {
				m.root.Core.DB.Cache.Options = m.root.Core.Clustered.Redis
			}
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

		ret, err := client.Get(ctx, key, clientv3.WithPrefix())
		if err != nil {
			return err
		}

		parser := yaml.Parser()

		for _, kv := range ret.Kvs {
			configMap, err := parser.Unmarshal(kv.Value)
			if err != nil {
				return err
			}

			subKey := strings.TrimPrefix(string(kv.Key), key+"/")
			fullKey := prefix + "." + subKey

			if lo.Contains(m.flags[fullKey], FLAG_NOSYNC) {
				continue
			}

			err = m.config.Set(fullKey, configMap)
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

		if !overwrite {
			ret, err := client.Get(ctx, etcdKey, clientv3.WithPrefix())
			if err != nil {
				return err
			}

			if ret.Count > 0 {
				return nil
			}
		}

		subConfig := m.config.Cut(prefix)
		parser := yaml.Parser()

		for k, v := range subConfig.All() {
			if lo.Contains(m.flags[prefix+"."+k], FLAG_NOSYNC) {
				continue
			}

			key := etcdKey + "/" + k
			key = strings.ReplaceAll(key, ".", "/")

			data, err := parser.Marshal(map[string]interface{}{k: v})
			if err != nil {
				return err
			}

			_, err = client.Put(ctx, key, string(data))
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (m *ManagerDefault) notifyConfigChangeCallbacks(key string, value any) {
	for _, callback := range m.changeCallbacks {
		err := callback(key, value)
		if err != nil {
			m.logger.Error("failed to notify config change callback", zap.Error(err))
		}
	}
}

func (m *ManagerDefault) Config() *Config {
	return m.root
}

func (m *ManagerDefault) Save() error {
	m.changes = true
	return m.maybeSave()
}

func (m *ManagerDefault) ConfigFile() string {
	return m.configFile
}

func (m *ManagerDefault) ConfigDir() string {
	return path.Dir(m.ConfigFile())
}

func findConfigFile(dirCheck bool, ignoreExist bool) string {
	for _, _path := range configDirPaths {
		expandedPath := os.ExpandEnv(path.Join(_path, CoreConfigFile))

		// First, check if the file exists
		_, err := os.Stat(expandedPath)
		if err == nil {
			// File exists, check if we can write to it
			_file, err := os.OpenFile(expandedPath, os.O_WRONLY, 0644)
			if err == nil {
				err := _file.Close()
				if err != nil {
					log.Printf("error closing file: %v", err)
				}
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

func findHighestWritableDir(path string) (string, error) {
	dir := path
	for {
		// Try to create a temporary file in the directory
		tempFile, err := os.CreateTemp(dir, ".write_test")
		if err == nil {
			err = tempFile.Close()
			if err != nil {
				return "", err
			}
			err = os.Remove(tempFile.Name())
			if err != nil {
				return "", err
			}
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

func buildPrefix(prefix, mapstructureTag string) string {
	if mapstructureTag != "" && mapstructureTag != "-" {
		if prefix != "" {
			return prefix + "." + mapstructureTag
		}
		return mapstructureTag
	}
	return prefix
}

func getProtoSectionSpecifier(pluginName string) string {
	return fmt.Sprintf(protoSectionSpecifier, pluginName)
}

func getAPISectionSpecifier(pluginName string) string {
	return fmt.Sprintf(apiSectionSpecifier, pluginName)
}

func getServiceSectionSpecifier(pluginName string, serviceName string) string {
	return fmt.Sprintf(serviceSectionSpecifier, pluginName, serviceName)
}
