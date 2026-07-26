package core

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

// KeyIdentityHandler defines how a specific key type (e.g., "ethereum",
// "solana") validates and normalizes its keys and metadata, issues challenges,
// and verifies proofs of ownership.
//
// Handlers are registered by plugins via PluginInfo.KeyIdentityHandlers
// and looked up by type string. All methods that need runtime context
// receive a core.Context, which embeds context.Context and provides
// access to config, DB, logger, and registered services.
//
// This interface is intentionally minimal so that core can orchestrate
// a generic challenge → verify → lookup → login flow without knowing
// the cryptographic details of any specific key type.
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

	// IssueChallenge generates a challenge for proving ownership of the
	// given key. The returned bytes are sent to the client, which must
	// sign them and return the signature via VerifyProof.
	//
	// For Ethereum (CAIP-122/EIP-4361): this generates a nonce and
	// constructs the SIWE message text for the client to sign.
	// For WebAuthn: this generates the assertion challenge options.
	//
	// The challenge state (nonce, etc.) must be stored by the handler
	// using ctx (e.g., via ctx.DB() or a registered challenge store service)
	// so that VerifyProof can validate it.
	IssueChallenge(ctx Context, key string, metadata json.RawMessage) ([]byte, error)

	// VerifyProof verifies a cryptographic proof of key ownership.
	//
	// The proof bytes are protocol-specific and must correspond to a
	// challenge previously issued by IssueChallenge. The handler is
	// responsible for looking up the stored challenge state via ctx
	// and performing the cryptographic verification.
	//
	// For Ethereum: this is the EIP-191 signature recovery and address
	// comparison, with nonce/domain validation from the stored challenge.
	// For WebAuthn: this verifies the assertion signature against the
	// stored credential.
	VerifyProof(ctx Context, key string, metadata json.RawMessage, proof []byte) error
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

// Ensure context import is used (for godoc examples and future helpers).
var _ context.Context = (Context)(nil)
