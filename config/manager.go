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
	yamlCore "gopkg.in/yaml.v3"
	"log"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

// Constants
const (
	CLUSTER_CONFIG_KEY = "config"
	FLAG_SYNC          = "sync"
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

	mapStructureTag = "config"
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

type FieldProcessor func(parent *reflect.StructField, field reflect.StructField, value reflect.Value, prefix string) error

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
	updateChan      chan ConfigUpdate
}
type ConfigUpdate struct {
	Key   string
	Value any
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
		changeCallbacks: make([]ConfigChangeCallback, 0),
		updateChan:      make(chan ConfigUpdate, 100),
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

	err = m.maybeConfigureCluster()
	if err != nil {
		return err
	}

	_, err = m.configureSection("core", &m.root.Core)
	if err != nil {
		return err
	}

	err = m.initLiveUpdates()
	if err != nil {
		return err
	}

	err = m.saveCoreConfig()
	if err != nil {
		return err
	}

	return nil
}

func (m *ManagerDefault) initLiveUpdates() error {
	go m.handleConfigChanges()

	if m.root.Core.ClusterEnabled() && m.root.Core.Clustered.Etcd != nil {
		return m.initClusterWatcher()
	}

	return nil
}

func (m *ManagerDefault) initClusterWatcher() error {
	client, err := m.root.Core.Clustered.Etcd.Client()
	if err != nil {
		return err
	}

	watchKey := "/" + CLUSTER_CONFIG_KEY + "/"
	watchChan := client.Watch(context.Background(), watchKey, clientv3.WithPrefix())

	go func() {
		for watchResp := range watchChan {
			for _, event := range watchResp.Events {
				key := strings.TrimPrefix(string(event.Kv.Key), watchKey)
				key = strings.ReplaceAll(key, "/", ".")
				m.updateChan <- ConfigUpdate{Key: key, Value: event.Kv.Value}
			}
		}
	}()

	return nil
}

func (m *ManagerDefault) handleConfigChanges() {
	for update := range m.updateChan {
		if m.shouldSyncKey(update.Key) {
			m.lock.Lock()
			err := m.config.Set(update.Key, update.Value)
			if err != nil {
				m.logger.Error("Failed to update local config", zap.Error(err))
				continue
			}

			if m.root.Core.ClusterEnabled() && m.root.Core.Clustered.Etcd != nil {
				err = m.saveClusterSpace(update.Key, true)
				if err != nil {
					m.logger.Error("Failed to save to cluster space", zap.Error(err))
				}
			}

			err = m.reconfigureSection(update.Key)
			if err != nil {
				return
			}

			m.lock.Unlock()
			m.notifyConfigChangeCallbacks(update.Key, update.Value)
		}
	}
}

func (m *ManagerDefault) reconfigureSection(key string) error {
	switch {
	case strings.HasPrefix(key, "core"):
		_, err := m.configureSection("core", &m.root.Core)
		if err != nil {
			return err
		}
	case strings.HasPrefix(key, "plugin"):
		parts := strings.Split(key, ".")
		if len(parts) > 1 {
			pluginName := parts[1]
			if len(parts) > 2 {
				switch parts[2] {
				case "protocol":
					_, err := m.configureSection(GetProtoSectionSpecifier(pluginName), m.root.Plugin[pluginName].Protocol)
					if err != nil {
						return err
					}
				case "api":
					_, err := m.configureSection(GetAPISectionSpecifier(pluginName), m.root.Plugin[pluginName].API)
					if err != nil {
						return err
					}
				case "service":
					serviceName := parts[3]
					_, err := m.configureSection(GetServiceSectionSpecifier(pluginName, serviceName), m.root.Plugin[pluginName].Service[serviceName])
					if err != nil {
						return err
					}
				}
			}
		}
	}

	return nil
}

func (m *ManagerDefault) pruneConfig(prefix string, cfg Defaults) error {
	validKeys := make(map[string]bool)
	err := m.FieldProcessor(cfg, prefix, func(_ *reflect.StructField, field reflect.StructField, value reflect.Value, prefix string) error {
		if field.Type == nil {
			return nil
		}

		if field.Type.Kind() == reflect.Struct {
			if _, ok := value.Interface().(yamlCore.Marshaler); !ok {
				return nil
			}
		}

		if field.Type.Kind() == reflect.Map || field.Type.Kind() == reflect.Slice {
			return nil
		}

		tag := field.Tag.Get(mapStructureTag)
		if tag != "" {
			prefix = buildPrefix(prefix, field.Tag)
			validKeys[prefix] = true
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("error processing fields: %w", err)
	}

	// Only get keys under the specified prefix
	keysToCheck := m.config.Cut(prefix).Keys()

	for _, key := range keysToCheck {
		fullKey := prefix + "." + key
		if !validKeys[fullKey] {
			m.config.Delete(fullKey)
			m.changes = true
			m.changedSections = append(m.changedSections, fullKey)
			m.logger.Debug("Pruned invalid config key", zap.String("key", fullKey))
		}
	}

	return nil
}
func (m *ManagerDefault) shouldSyncKey(key string) bool {
	return !lo.Contains(m.flags[key], FLAG_SYNC)
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

	prefix := GetProtoSectionSpecifier(pluginName)
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

	prefix := GetAPISectionSpecifier(pluginName)
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

	prefix := GetServiceSectionSpecifier(pluginName, serviceName)
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
		target = GetAPISectionSpecifier(pluginName)
	case sectionKindService:
		configPath = path.Join(basePath, PluginsDir, pluginName, ServiceDir, name+CONFIG_EXTENSION)
		target = GetServiceSectionSpecifier(pluginName, name)
	case sectionKindProtocol:
		configPath = path.Join(basePath, PluginsDir, pluginName, ProtoDir, SectionConfigFile)
		target = GetProtoSectionSpecifier(pluginName)

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

	hooks := append([]mapstructure.DecodeHookFunc{}, mapstructure.StringToTimeDurationHookFunc())

	err = m.config.UnmarshalWithConf(name, cfg, koanf.UnmarshalConf{
		Tag: mapStructureTag,
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

	err = m.pruneConfig(name, cfg)
	if err != nil {
		return nil, err
	}

	err = m.maybeSave()
	if err != nil {
		return nil, err
	}

	err = m.saveClusterSpace(name, false)
	if err != nil {
		return nil, err
	}

	return cfg, nil
}
func (m *ManagerDefault) setDefaultsForObject(obj any, prefix string) error {
	return m.FieldProcessor(obj, prefix, m.defaultProcessor)
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

func (m *ManagerDefault) setDefault(key string, value any) (bool, error) {
	if !m.config.Exists(key) || (m.config.Get(key) == nil && value != nil) {
		err := m.config.Set(key, value)
		if err != nil {
			return false, err
		}
		return true, nil
	}

	return false, nil
}

func (m *ManagerDefault) validateObject(obj any) error {
	return m.FieldProcessor(obj, "", m.validateProcessor)
}

func (m *ManagerDefault) processFlags(obj any, prefix string) error {
	return m.FieldProcessor(obj, prefix, m.flagsProcessor)
}

func (m *ManagerDefault) FieldProcessor(obj any, prefix string, processors ...FieldProcessor) error {
	return m.fieldProcessorRecursive(obj, prefix, nil, processors...)
}

func (m *ManagerDefault) fieldProcessorRecursive(obj any, prefix string, parentField *reflect.StructField, processors ...FieldProcessor) error {
	objValue := reflect.ValueOf(obj)
	objType := reflect.TypeOf(obj)

	if objValue.Kind() == reflect.Ptr {
		if objValue.IsNil() {
			return nil // Skip processing if the pointer is nil
		}
		objValue = objValue.Elem()
		objType = objType.Elem()
	}

	canYaml := false
	canDefault := false
	canValidate := false
	isStruct := objType.Kind() == reflect.Struct
	if isStruct {
		if _, ok := obj.(yamlCore.Marshaler); ok {
			canYaml = true
		}

		if _, ok := obj.(Defaults); ok {
			canDefault = true
		}

		if _, ok := obj.(Validator); ok {
			canValidate = true
		}
	}

	if !isStruct || canYaml || canDefault || canValidate {
		// Process the field
		for _, processor := range processors {
			if err := processor(parentField, createStructField(objType), objValue, prefix); err != nil {
				return err
			}
		}

		if !isStruct {
			return nil
		}

	}

	for i := 0; i < objValue.NumField(); i++ {
		field := objValue.Field(i)
		fieldType := objType.Field(i)

		if !field.CanInterface() || !fieldType.IsExported() {
			continue
		}

		// Apply all processors to this field
		for _, processor := range processors {
			if err := processor(parentField, fieldType, field, prefix); err != nil {
				return err
			}
		}

		newPrefix := buildPrefix(prefix, fieldType.Tag)

		// Recurse for struct fields or pointers to structs
		switch field.Kind() {
		case reflect.Struct:
			if err := m.fieldProcessorRecursive(field.Interface(), newPrefix, &fieldType, processors...); err != nil {
				return err
			}
		case reflect.Ptr:
			if !field.IsNil() && field.Elem().Kind() == reflect.Struct {
				if err := m.fieldProcessorRecursive(field.Interface(), newPrefix, &fieldType, processors...); err != nil {
					return err
				}
			}
		case reflect.Slice:
			if field.Len() > 0 {
				for i := 0; i < field.Len(); i++ {
					fieldPrefix := fmt.Sprintf("%s.%d", newPrefix, i)
					if err := m.fieldProcessorRecursive(field.Index(i).Interface(), fieldPrefix, &fieldType, processors...); err != nil {
						return err
					}
				}
			}

		case reflect.Map:
			for _, key := range field.MapKeys() {
				fieldPrefix := fmt.Sprintf("%s.%s", newPrefix, key.String())
				if err := m.fieldProcessorRecursive(field.MapIndex(key).Interface(), fieldPrefix, &fieldType, processors...); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (m *ManagerDefault) validateProcessor(_ *reflect.StructField, _ reflect.StructField, value reflect.Value, _ string) error {
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

func (m *ManagerDefault) defaultProcessor(_ *reflect.StructField, field reflect.StructField, value reflect.Value, prefix string) error {
	// Check if the value is a nil pointer
	if value.Kind() == reflect.Ptr && value.IsNil() {
		return nil // Skip defaults for nil pointers
	}

	// If it's a pointer, get the element it points to
	if value.Kind() == reflect.Ptr {
		value = value.Elem()
	}

	if setter, ok := value.Interface().(Defaults); ok {
		if err := m.applyDefaults(setter, buildPrefix(prefix, field.Tag)); err != nil {
			return err
		}
	}

	return nil
}

func (m *ManagerDefault) flagsProcessor(_ *reflect.StructField, field reflect.StructField, _ reflect.Value, prefix string) error {
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

		if m.hasSectionChanged(GetProtoSectionSpecifier(pluginName)) && len(protocol.Keys()) > 0 {
			if err := m.saveConfigToFile(protocol, filepath.Join(pluginPath, ProtoDir, SectionConfigFile)); err != nil {
				return err
			}
		}

		for _, svcName := range services.MapKeys("") {
			if m.hasSectionChanged(GetServiceSectionSpecifier(pluginName, svcName)) && len(services.Keys()) > 0 {
				if err := m.saveConfigToFile(services.Cut(svcName), filepath.Join(pluginPath, ServiceDir, svcName+CONFIG_EXTENSION)); err != nil {
					return err
				}
			}
		}
		if m.hasSectionChanged(GetAPISectionSpecifier(pluginName)) && len(api.Keys()) > 0 {
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

		etcdPrefix := "/" + CLUSTER_CONFIG_KEY + "/" + strings.ReplaceAll(prefix, ".", "/")

		ret, err := client.Get(ctx, etcdPrefix, clientv3.WithPrefix())
		if err != nil {
			return err
		}

		for _, kv := range ret.Kvs {
			key := strings.TrimPrefix(string(kv.Key), etcdPrefix+"/")
			key = strings.ReplaceAll(key, "/", ".")
			fullKey := prefix + "." + key

			if !m.shouldSyncKey(fullKey) {
				continue
			}

			value := string(kv.Value)

			// Attempt to parse the value as needed
			var parsedValue interface{}
			if err := m.parseValue(value, &parsedValue); err != nil {
				m.logger.Warn("Failed to parse value, using as string", zap.String("key", fullKey), zap.Error(err))
				parsedValue = value
			}

			err = m.config.Set(fullKey, parsedValue)
			if err != nil {
				return err
			}

			m.changes = true
		}
	}

	return nil
}

// parseValue attempts to parse a string value into an appropriate type
func (m *ManagerDefault) parseValue(value string, result interface{}) error {
	// Try to parse as int
	if intValue, err := strconv.Atoi(value); err == nil {
		*result.(*interface{}) = intValue
		return nil
	}

	// Try to parse as float
	if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
		*result.(*interface{}) = floatValue
		return nil
	}

	// Try to parse as bool
	if boolValue, err := strconv.ParseBool(value); err == nil {
		*result.(*interface{}) = boolValue
		return nil
	}

	// If all else fails, treat it as a string
	*result.(*interface{}) = value
	return nil
}

func (m *ManagerDefault) saveClusterSpace(prefix string, overwrite bool) error {
	if m.root.Core.ClusterEnabled() && m.root.Core.Clustered.Etcd != nil {
		ctx := context.Background()
		client, err := m.root.Core.Clustered.Etcd.Client()
		if err != nil {
			return err
		}

		etcdPrefix := "/" + CLUSTER_CONFIG_KEY + "/" + strings.ReplaceAll(prefix, ".", "/")

		if !overwrite {
			ret, err := client.Get(ctx, etcdPrefix, clientv3.WithPrefix())
			if err != nil {
				return err
			}

			if ret.Count > 0 {
				return nil
			}
		}

		subConfig := m.config.Cut(prefix)

		for k, v := range subConfig.All() {
			if !m.shouldSyncKey(prefix + "." + k) {
				continue
			}

			key := etcdPrefix + "/" + strings.ReplaceAll(k, ".", "/")
			value := fmt.Sprintf("%v", v)

			_, err = client.Put(ctx, key, value)
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

func (m *ManagerDefault) Update(key string, value any) error {
	var exists bool

	err := m.FieldProcessor(m.root, key, m.fieldExistsProcessorFactory(key, func() {
		exists = true
	}))

	if err != nil {
		return err
	}

	if !exists {
		return fmt.Errorf("key %s does not exist", key)
	}

	m.updateChan <- ConfigUpdate{Key: key, Value: value}

	return nil
}

func (m *ManagerDefault) Exists(key string) bool {
	return m.config.Exists(key)
}

func (m *ManagerDefault) Get(key string) any {
	return m.config.Get(key)
}

func (m *ManagerDefault) All() map[string]any {
	return m.config.All()
}

func (m *ManagerDefault) IsEditable(key string) bool {
	return m.shouldSyncKey(key)
}

func (m *ManagerDefault) fieldExistsProcessorFactory(key string, cb func()) FieldProcessor {
	return func(_ *reflect.StructField, field reflect.StructField, value reflect.Value, prefix string) error {
		if field.Name == key {
			cb()
			return nil
		}
		return nil
	}
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

func buildPrefix(prefix string, tag reflect.StructTag) string {
	configTag := tag.Get(mapStructureTag)

	if configTag != "" && configTag != "-" {
		if prefix != "" {
			return prefix + "." + configTag
		}
		return configTag
	}
	return prefix
}

func GetCoreSectionSpecifier(key string) string {
	return fmt.Sprintf("core.%s", key)
}

func GetProtoSectionSpecifier(pluginName string) string {
	return fmt.Sprintf(protoSectionSpecifier, pluginName)
}

func GetAPISectionSpecifier(pluginName string) string {
	return fmt.Sprintf(apiSectionSpecifier, pluginName)
}

func GetServiceSectionSpecifier(pluginName string, serviceName string) string {
	return fmt.Sprintf(serviceSectionSpecifier, pluginName, serviceName)
}

func createStructField(t reflect.Type) reflect.StructField {
	// Create a new StructField
	f := reflect.StructField{}

	// Set the Type
	f.Type = t

	// Set the Name (assuming the struct type name is the field name)
	f.Name = t.Name()

	// Anonymous is false as this is the root
	f.Anonymous = false

	// PkgPath is empty for exported fields
	if t.Name() != "" && t.Name()[0] >= 'A' && t.Name()[0] <= 'Z' {
		f.PkgPath = ""
	} else {
		f.PkgPath = t.PkgPath()
	}

	// Tag is empty as we don't have tag information at this level
	f.Tag = ""

	// Offset is 0 as this is the root
	f.Offset = 0

	// Index is empty as this is the root
	f.Index = []int{}

	return f
}
