package service

import (
	"fmt"
	"strings"
	"sync"

	"go.lumeweb.com/portal/core"
	"go.uber.org/zap"
)

const ContentScanOperation = "content.scan"

// OperationFinder handles finding operations and their handlers
type OperationFinder interface {
	// FindOperationHandler locates the operation and handler for a given operation type
	FindOperationHandler(operationType string) (core.Operation, core.OperationHandler, error)

	// FindProtocolOperation searches registered protocols for the operation
	FindProtocolOperation(operationType string) (core.Operation, core.OperationHandler, error)

	// FindPluginOperation searches registered plugins for the operation
	FindPluginOperation(operationType string) (core.Operation, core.OperationHandler, error)
}

// OperationFinderDefault is the default implementation of OperationFinder
type operationCacheEntry struct {
	op      core.Operation
	handler core.OperationHandler
}

type OperationFinderDefault struct {
	ctx       core.Context
	logger    *core.Logger
	opCache   map[string]operationCacheEntry
	opCacheMu sync.RWMutex
}

// NewOperationFinder creates a new OperationFinder instance
func NewOperationFinder(ctx core.Context) OperationFinder {
	return &OperationFinderDefault{
		ctx:     ctx,
		logger:  ctx.Logger(),
		opCache: make(map[string]operationCacheEntry),
	}
}

// FindOperationHandler locates the operation and handler for a given operation type
func (of *OperationFinderDefault) FindOperationHandler(operationType string) (core.Operation, core.OperationHandler, error) {
	// Check cache first
	of.opCacheMu.RLock()
	cached, exists := of.opCache[operationType]
	of.opCacheMu.RUnlock()
	if exists {
		return cached.op, cached.handler, nil
	}

	// First try to find operation in registered protocols
	op, handler, err := of.FindProtocolOperation(operationType)
	if err == nil {
		of.opCacheMu.Lock()
		of.opCache[operationType] = operationCacheEntry{op: op, handler: handler}
		of.opCacheMu.Unlock()
		return op, handler, nil
	}

	// If not found in protocols, try plugins
	op, handler, err = of.FindPluginOperation(operationType)
	if err == nil {
		of.opCacheMu.Lock()
		of.opCache[operationType] = operationCacheEntry{op: op, handler: handler}
		of.opCacheMu.Unlock()
		return op, handler, nil
	}

	// Special case for content scan operation
	if operationType == ContentScanOperation {
		scanner := core.NewNoContentScanner()
		handler := &contentScanAdapter{scanner: scanner}
		scanOp := core.NewOperation(ContentScanOperation, core.OpTypeScan, handler)
		of.opCacheMu.Lock()
		of.opCache[operationType] = operationCacheEntry{op: scanOp, handler: handler}
		of.opCacheMu.Unlock()
		return scanOp, handler, nil
	}

	return nil, nil, fmt.Errorf("operation not found: %s", operationType)
}

// FindProtocolOperation searches registered protocols for the operation
func (of *OperationFinderDefault) FindProtocolOperation(operationType string) (core.Operation, core.OperationHandler, error) {
	parts := strings.Split(operationType, ".")
	if len(parts) < 2 {
		return nil, nil, fmt.Errorf("invalid operation type format: %s", operationType)
	}

	protocolName := parts[0]
	protocol := core.GetProtocol(protocolName)
	if protocol == nil {
		return nil, nil, fmt.Errorf("protocol not found: %s", protocolName)
	}

	for _, op := range protocol.Operations() {
		if op.Type() == operationType {
			return op, op.Handler(), nil
		}
	}

	return nil, nil, fmt.Errorf("operation not found in protocol: %s", operationType)
}

// FindPluginOperation searches registered plugins for the operation
func (of *OperationFinderDefault) FindPluginOperation(operationType string) (core.Operation, core.OperationHandler, error) {
	plugins := core.GetPlugins()
	for _, plugin := range plugins {
		if plugin.Operations != nil {
			ops, err := plugin.Operations(of.ctx)
			if err != nil {
				of.logger.Warn("Failed to get operations from plugin",
					zap.String("plugin", plugin.ID),
					zap.Error(err))
				continue
			}

			for _, op := range ops {
				if op.Type() == operationType {
					return op, op.Handler(), nil
				}
			}
		}
	}

	return nil, nil, fmt.Errorf("operation not found in plugins: %s", operationType)
}
