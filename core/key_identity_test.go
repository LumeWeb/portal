package core

import (
	"context"
	"encoding/json"
	"testing"
)

// mockHandler is a minimal handler for testing the registry.
type mockHandler struct{}

func (h *mockHandler) NormalizeKey(key string) (string, error) { return key, nil }
func (h *mockHandler) ValidateMetadata(metadata json.RawMessage) (json.RawMessage, error) {
	return metadata, nil
}
func (h *mockHandler) VerifyProof(ctx context.Context, key string, metadata json.RawMessage, proof []byte) error {
	return nil
}

func TestRegisterAndGetKeyIdentityHandler(t *testing.T) {
	keyType := "test_type"

	// Register
	RegisterKeyIdentity(keyType, &mockHandler{})

	// Get -- should exist
	h, ok := GetKeyIdentityHandler(keyType)
	if !ok {
		t.Fatal("expected handler to be registered")
	}
	if h == nil {
		t.Fatal("handler should not be nil")
	}

	// Get nonexistent
	_, ok = GetKeyIdentityHandler("nonexistent")
	if ok {
		t.Fatal("expected handler to not be registered for nonexistent type")
	}
}

func TestMustGetKeyIdentityHandler_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for unregistered type")
		}
	}()
	MustGetKeyIdentityHandler("definitely_not_registered")
}

func TestRegisterKeyIdentityHandlersFromPlugins(t *testing.T) {
	// Register a plugin with a key identity handler
	RegisterPlugin(PluginInfo{
		ID: "test-plugin-keyidentity",
		KeyIdentityHandlers: []KeyIdentityHandlerRegistration{
			{Type: "test_plugin_type", Handler: &mockHandler{}},
		},
	})
	defer UnregisterPlugin("test-plugin-keyidentity")

	RegisterKeyIdentityHandlersFromPlugins()

	h, ok := GetKeyIdentityHandler("test_plugin_type")
	if !ok {
		t.Fatal("expected handler to be registered from plugin")
	}
	if h == nil {
		t.Fatal("handler should not be nil")
	}
}

func TestRegisterKeyIdentityHandlersFromPlugins_DoesNotOverwriteExistingHandler(t *testing.T) {
	// Manually register a handler first
	RegisterKeyIdentity("preexisting_type", &mockHandler{})

	// Register a plugin with a different type — should not touch preexisting
	RegisterPlugin(PluginInfo{
		ID: "test-plugin-no-overwrite",
		KeyIdentityHandlers: []KeyIdentityHandlerRegistration{
			{Type: "plugin_type", Handler: &mockHandler{}},
		},
	})
	defer UnregisterPlugin("test-plugin-no-overwrite")

	RegisterKeyIdentityHandlersFromPlugins()

	// Both should exist
	_, ok := GetKeyIdentityHandler("preexisting_type")
	if !ok {
		t.Fatal("pre-existing handler should still be registered")
	}
	_, ok = GetKeyIdentityHandler("plugin_type")
	if !ok {
		t.Fatal("plugin handler should be registered")
	}
}

func TestRegisterKeyIdentity_NilHandlerRejected(t *testing.T) {
	RegisterKeyIdentity("nil_test_type", nil)

	_, ok := GetKeyIdentityHandler("nil_test_type")
	if ok {
		t.Fatal("nil handler should not be registered")
	}
}

func TestRegisterKeyIdentity_EmptyTypePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for empty key type")
		}
	}()
	RegisterKeyIdentity("", &mockHandler{})
}

func TestRegisterKeyIdentityHandlersFromPlugins_SkipsInvalidEntries(t *testing.T) {
	RegisterPlugin(PluginInfo{
		ID: "test-plugin-invalid-entries",
		KeyIdentityHandlers: []KeyIdentityHandlerRegistration{
			{Type: "", Handler: &mockHandler{}},           // empty type
			{Type: "nil_handler_type", Handler: nil},       // nil handler
			{Type: "valid_plugin_type", Handler: &mockHandler{}}, // valid
		},
	})
	defer UnregisterPlugin("test-plugin-invalid-entries")

	RegisterKeyIdentityHandlersFromPlugins()

	// Empty type should not be registered
	_, ok := GetKeyIdentityHandler("")
	if ok {
		t.Fatal("empty type should not be registered")
	}

	// Nil handler should not be registered
	_, ok = GetKeyIdentityHandler("nil_handler_type")
	if ok {
		t.Fatal("nil handler should not be registered")
	}

	// Valid entry should be registered
	_, ok = GetKeyIdentityHandler("valid_plugin_type")
	if !ok {
		t.Fatal("valid handler should be registered")
	}
}
