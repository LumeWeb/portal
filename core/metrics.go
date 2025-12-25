package core

import (
	"fmt"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/samber/lo"
	"go.uber.org/zap"
)

const (
	MetricsIdentifierCore         = "core"
	MetricsIdentifierPluginPrefix = "plugin."
)

var (
	coreRegistry     *prometheus.Registry
	pluginRegistries = make(map[string]*prometheus.Registry)
	metricsMu        sync.RWMutex
)

func init() {
	coreRegistry = prometheus.NewRegistry()
}

// CoreMetricsRegistry returns the global core prometheus registry
func CoreMetricsRegistry() *prometheus.Registry {
	return coreRegistry
}

// PluginMetricsRegistry returns the prometheus registry for a specific plugin
func PluginMetricsRegistry(pluginID string) *prometheus.Registry {
	metricsMu.Lock()
	defer metricsMu.Unlock()

	if _, exists := pluginRegistries[pluginID]; !exists {
		pluginRegistries[pluginID] = prometheus.NewRegistry()
	}
	return pluginRegistries[pluginID]
}

// RegisterCoreCollector registers a collector with the core registry
func RegisterCoreCollector(collector prometheus.Collector) error {
	return CoreMetricsRegistry().Register(collector)
}

// RegisterPluginCollector registers a collector with a specific plugin's registry
func RegisterPluginCollector(pluginID string, collector prometheus.Collector) error {
	if pluginID == "" {
		return fmt.Errorf("plugin ID cannot be empty")
	}
	return PluginMetricsRegistry(pluginID).Register(collector)
}

// GatherAllCoreMetrics gathers all metrics from the core registry
func GatherAllCoreMetrics() ([]*dto.MetricFamily, error) {
	return CoreMetricsRegistry().Gather()
}

// GatherPluginMetrics gathers all metrics from a specific plugin's registry
func GatherPluginMetrics(pluginID string) ([]*dto.MetricFamily, error) {
	return PluginMetricsRegistry(pluginID).Gather()
}

// GatherMetricsByID gathers metrics by identifier with namespace separation
// The identifier format can be:
// - "core" or empty for all core metrics
// - "plugin.<pluginID>" for all metrics from a specific plugin
// - "<metric_name>" for a specific metric name from all registries (namespaced by plugin)
func GatherMetricsByID(ctx Context, identifier string) ([]*dto.MetricFamily, error) {
	// Handle core metrics
	if identifier == MetricsIdentifierCore || identifier == "" {
		return coreRegistry.Gather()
	}

	// Handle plugin-specific metrics with "plugin." prefix
	prefixLen := len(MetricsIdentifierPluginPrefix)
	if len(identifier) > prefixLen && identifier[:prefixLen] == MetricsIdentifierPluginPrefix {
		pluginID := identifier[prefixLen:]
		return PluginMetricsRegistry(pluginID).Gather()
	}

	// Handle specific metric name across all registries
	return gatherMetricByName(ctx, identifier)
}

// gatherMetricByName collects a specific metric name from all registries
// It prefixes plugin metrics with the plugin ID for namespace separation
func gatherMetricByName(ctx Context, metricName string) ([]*dto.MetricFamily, error) {
	metricsMu.RLock()
	defer metricsMu.RUnlock()

	var result []*dto.MetricFamily

	// Gather from core registry
	coreMetrics, err := coreRegistry.Gather()
	if err != nil {
		return nil, fmt.Errorf("failed to gather core metrics: %w", err)
	}

	for _, mf := range coreMetrics {
		if mf.GetName() == metricName {
			result = append(result, mf)
			break
		}
	}

	// Gather from all plugin registries
	for pluginID, registry := range pluginRegistries {
		pluginMetrics, err := registry.Gather()
		if err != nil {
			if ctx != nil && ctx.Logger() != nil {
				ctx.Logger().Warn("Failed to gather plugin metrics",
					zap.String("plugin", pluginID),
					zap.Error(err))
			}
			continue
		}

		for _, mf := range pluginMetrics {
			if mf.GetName() == metricName {
				// Clone and namespace the metric family
				namespaced := &dto.MetricFamily{
					Name:   lo.ToPtr(pluginID + "_" + mf.GetName()),
					Help:   mf.Help,
					Type:   mf.Type,
					Metric: mf.Metric,
				}
				result = append(result, namespaced)
				break
			}
		}
	}

	return result, nil
}

// GetMetricIdentifiers returns all available metric identifiers
// This includes "core" and "plugin.<pluginID>" for each registered plugin
func GetMetricIdentifiers() []string {
	metricsMu.RLock()
	defer metricsMu.RUnlock()

	identifiers := []string{MetricsIdentifierCore}

	for pluginID := range pluginRegistries {
		identifiers = append(identifiers, MetricsIdentifierPluginPrefix+pluginID)
	}

	return identifiers
}

// RegisterServiceMetrics registers a service's metrics with the appropriate registry
// Core services register with the core registry, plugin services register with the plugin's registry
func RegisterServiceMetrics(serviceID string, metrics []prometheus.Collector) error {
	if len(metrics) == 0 {
		return nil
	}

	pluginID := GetPluginForService(serviceID)

	if IsCoreService(serviceID) || pluginID == "" {
		// Register with core registry
		for _, metric := range metrics {
			if err := RegisterCoreCollector(metric); err != nil {
				return fmt.Errorf("failed to register core metric %T: %w", metric, err)
			}
		}
		return nil
	}

	// Register with plugin registry
	for _, metric := range metrics {
		if err := RegisterPluginCollector(pluginID, metric); err != nil {
			return fmt.Errorf("failed to register plugin %s metric %T: %w", pluginID, metric, err)
		}
	}
	return nil
}

// RegisterPluginMetrics registers a plugin's metrics with the plugin's registry
func RegisterPluginMetrics(pluginID string, metrics []prometheus.Collector) error {
	if len(metrics) == 0 {
		return nil
	}

	for _, metric := range metrics {
		if err := RegisterPluginCollector(pluginID, metric); err != nil {
			return fmt.Errorf("failed to register plugin %s metric %T: %w", pluginID, metric, err)
		}
	}
	return nil
}

// MetricTrack tracks duration and errors for single-return functions.
// The result type can be error; if so, errors counter is incremented.
// Uses prometheus.NewTimer for idiomatic timing with Observer interface.
func MetricTrack[T any](observer prometheus.Observer, errors prometheus.Counter, f func() T) T {
	timer := prometheus.NewTimer(observer)
	defer timer.ObserveDuration()

	result := f()

	var err error
	if e, ok := any(result).(error); ok {
		err = e
		if err != nil {
			errors.Inc()
		}
	}

	return result
}

// MetricTrackResult tracks duration and errors for functions returning (T, error).
// This handles the common pattern of multi-value returns where the first value is not an error type.
// Uses prometheus.NewTimer for idiomatic timing with Observer interface.
func MetricTrackResult[T any](observer prometheus.Observer, errors prometheus.Counter, f func() (T, error)) (T, error) {
	timer := prometheus.NewTimer(observer)
	defer timer.ObserveDuration()

	result, err := f()

	if err != nil {
		errors.Inc()
	}

	return result, err
}

// MetricTrackWithBytes tracks duration, errors, and byte counts.
// Composes MetricTrack with bytes tracking.
func MetricTrackWithBytes[T any](histogram prometheus.Histogram, bytesCounter prometheus.Counter, errors prometheus.Counter, f func() (T, uint64)) T {
	return MetricTrack(histogram, errors, func() T {
		result, size := f()
		var err error
		if e, ok := any(result).(error); ok {
			err = e
		}
		if err == nil {
			bytesCounter.Add(float64(size))
		}
		return result
	})
}

// MetricTrackGauge tracks duration, errors, and manages a gauge counter.
// Composes MetricTrack with Inc/Dec pattern.
func MetricTrackGauge[T any](gauge prometheus.Gauge, histogram prometheus.Histogram, errors prometheus.Counter, f func() T) T {
	gauge.Inc()
	defer gauge.Dec()
	return MetricTrack(histogram, errors, f)
}

// MetricTrackCache tracks duration, errors, and cache hits/misses.
// Uses prometheus.NewTimer for idiomatic timing with Observer interface.
func MetricTrackCache[T any](duration prometheus.Histogram, errors, hits, misses prometheus.Counter, f func() (T, bool, error)) (T, error) {
	timer := prometheus.NewTimer(duration)
	defer timer.ObserveDuration()

	result, hit, err := f()

	if err != nil {
		errors.Inc()
	} else if hit {
		hits.Inc()
	} else {
		misses.Inc()
	}

	return result, err
}

// MetricTrackGaugeWithBytes tracks duration, errors, gauge, and bytes.
// Composes MetricTrackGauge with MetricTrackWithBytes.
func MetricTrackGaugeWithBytes[T any](gauge prometheus.Gauge, histogram prometheus.Histogram, bytesCounter prometheus.Counter, errors prometheus.Counter, size uint64, f func() T) T {
	gauge.Inc()
	defer gauge.Dec()
	return MetricTrackWithBytes(histogram, bytesCounter, errors, func() (T, uint64) {
		return f(), size
	})
}


