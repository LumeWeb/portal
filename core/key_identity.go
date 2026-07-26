package core

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

// KeyIdentityHandler defines how a specific key type (e.g., "ethereum",
// "solana") validates and normalizes its keys and metadata.
// Handlers are registered by plugins via PluginInfo.KeyIdentityHandlers
// and looked up by type string.
type KeyIdentityHandler interface {
	// NormalizeKey converts a raw key to its canonical form.
	// For Ethereum: lowercase the 0x-prefixed address.
	// For Solana: base58 validation.
	// etc.
	NormalizeKey(key string) (string, error)

	// ValidateMetadata validates and normalizes type-specific metadata.
	// For Ethereum: parse { "chain_id": "eip155:1", ... }.
	// Returns the normalized metadata, or an error if invalid.
	ValidateMetadata(metadata json.RawMessage) (json.RawMessage, error)

	// VerifyProof verifies a cryptographic proof of key ownership.
	// For Ethereum: this is the CAIP-122 signature verification flow.
	// The proof bytes are protocol-specific (EIP-191 signature, etc.).
	VerifyProof(ctx context.Context, key string, metadata json.RawMessage, proof []byte) error
}

var (
	keyIdentityRegistry   = make(map[string]KeyIdentityHandler)
	keyIdentityRegistryMu sync.RWMutex
)

// RegisterKeyIdentity registers a handler for a key type.
// Called during boot from RegisterKeyIdentityHandlersFromPlugins().
// Nil handlers are silently skipped to prevent panics in callers.
// Empty keyType is rejected with a panic to enforce the type as a required registry key.
func RegisterKeyIdentity(keyType string, handler KeyIdentityHandler) {
	if keyType == "" {
		panic("key type must not be empty")
	}
	if handler == nil {
		return
	}
	keyIdentityRegistryMu.Lock()
	defer keyIdentityRegistryMu.Unlock()
	keyIdentityRegistry[keyType] = handler
}

// ResetKeyIdentities clears the key identity registry.
// Called from ResetState to ensure test isolation.
func ResetKeyIdentities() {
	keyIdentityRegistryMu.Lock()
	defer keyIdentityRegistryMu.Unlock()
	keyIdentityRegistry = make(map[string]KeyIdentityHandler)
}

// GetKeyIdentityHandler retrieves the handler for a key type.
// Returns (handler, true) if registered, (nil, false) otherwise.
func GetKeyIdentityHandler(keyType string) (KeyIdentityHandler, bool) {
	keyIdentityRegistryMu.RLock()
	defer keyIdentityRegistryMu.RUnlock()
	h, ok := keyIdentityRegistry[keyType]
	return h, ok
}

// MustGetKeyIdentityHandler retrieves the handler for a key type, panicking
// if not registered. Use in init() chains where the handler must exist.
func MustGetKeyIdentityHandler(keyType string) KeyIdentityHandler {
	h, ok := GetKeyIdentityHandler(keyType)
	if !ok {
		panic(fmt.Sprintf("key identity handler not registered for type %q", keyType))
	}
	return h
}

// KeyIdentityHandlerRegistration pairs a key type string with its handler.
// Plugins declare these in PluginInfo.KeyIdentityHandlers.
type KeyIdentityHandlerRegistration struct {
	Type    string              // e.g. "ethereum", "solana"
	Handler KeyIdentityHandler
}

// RegisterKeyIdentityHandlersFromPlugins registers all key identity handlers
// declared by plugins. Called during boot after plugins are loaded.
// Plugin IDs are sorted to ensure deterministic registration order.
func RegisterKeyIdentityHandlersFromPlugins() {
	pluginsMu.RLock()
	defer pluginsMu.RUnlock()

	ids := make([]string, 0, len(plugins))
	for id := range plugins {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		plugin := plugins[id]
		for _, reg := range plugin.KeyIdentityHandlers {
			if reg.Type == "" || reg.Handler == nil {
				continue
			}
			RegisterKeyIdentity(reg.Type, reg.Handler)
		}
	}
}
