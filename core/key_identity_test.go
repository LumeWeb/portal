package core

import (
	"encoding/json"
	"testing"

	"go.lumeweb.com/portal/build"
)

// mockHandler is a minimal KeyIdentityHandler for testing.
type mockHandler struct {
	challengeFn func(ctx Context, key string, metadata json.RawMessage) ([]byte, error)
	verifyFn    func(ctx Context, key string, metadata json.RawMessage, proof []byte) error
}

func (h *mockHandler) NormalizeKey(key string) (string, error) { return key, nil }
func (h *mockHandler) ValidateMetadata(metadata json.RawMessage) (json.RawMessage, error) {
	return metadata, nil
}
func (h *mockHandler) IssueChallenge(ctx Context, key string, metadata json.RawMessage) ([]byte, error) {
	if h.challengeFn != nil {
		return h.challengeFn(ctx, key, metadata)
	}
	return []byte("challenge"), nil
}
func (h *mockHandler) VerifyProof(ctx Context, key string, metadata json.RawMessage, proof []byte) error {
	if h.verifyFn != nil {
		return h.verifyFn(ctx, key, metadata, proof)
	}
	return nil
}

func TestRegisterAndGetKeyIdentityHandler(t *testing.T) {
	ResetKeyIdentities()
	defer ResetKeyIdentities()

	keyType := "test_type"
	RegisterKeyIdentity(keyType, &mockHandler{})

	h, ok := GetKeyIdentityHandler(keyType)
	if !ok {
		t.Fatalf("expected handler for %q", keyType)
	}
	if h == nil {
		t.Fatal("handler is nil")
	}

	_, ok = GetKeyIdentityHandler("nonexistent")
	if ok {
		t.Fatal("expected no handler for nonexistent type")
	}
}

func TestListKeyIdentityTypes(t *testing.T) {
	ResetKeyIdentities()
	defer ResetKeyIdentities()

	// Empty registry returns empty slice
	if types := ListKeyIdentityTypes(); len(types) != 0 {
		t.Fatalf("expected no types, got %v", types)
	}

	RegisterKeyIdentity("zebra", &mockHandler{})
	RegisterKeyIdentity("alpha", &mockHandler{})
	RegisterKeyIdentity("middle", &mockHandler{})

	got := ListKeyIdentityTypes()
	want := []string{"alpha", "middle", "zebra"}
	if len(got) != len(want) {
		t.Fatalf("expected %d types, got %d: %v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected sorted types %v, got %v", want, got)
		}
	}
}

func TestMustGetKeyIdentityHandler_Panics(t *testing.T) {
	ResetKeyIdentities()
	defer ResetKeyIdentities()

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for unregistered handler")
		}
	}()
	MustGetKeyIdentityHandler("definitely_not_registered")
}

func TestRegisterKeyIdentityHandlersFromPlugins(t *testing.T) {
	ResetState()
	defer ResetState()

	RegisterPlugin(PluginInfo{
		ID:      "test-plugin",
		Version: build.New("test-version", "", "", "", "", "", ""),
		KeyIdentityHandlers: []KeyIdentityHandlerRegistration{
			{Type: "test_plugin_type", Handler: &mockHandler{}},
		},
	})

	RegisterKeyIdentityHandlersFromPlugins()

	h, ok := GetKeyIdentityHandler("test_plugin_type")
	if !ok || h == nil {
		t.Fatalf("expected handler registered for test_plugin_type")
	}
}

func TestRegisterKeyIdentityHandlersFromPlugins_DoesNotOverwriteExistingHandler(t *testing.T) {
	ResetState()
	defer ResetState()

	// Register a pre-existing handler
	RegisterKeyIdentity("preexisting_type", &mockHandler{})

	RegisterPlugin(PluginInfo{
		ID:      "test-plugin-2",
		Version: build.New("test-version", "", "", "", "", "", ""),
		KeyIdentityHandlers: []KeyIdentityHandlerRegistration{
			{Type: "preexisting_type", Handler: &mockHandler{}},
			{Type: "plugin_type", Handler: &mockHandler{}},
		},
	})

	RegisterKeyIdentityHandlersFromPlugins()

	// Should still exist
	_, ok := GetKeyIdentityHandler("preexisting_type")
	if !ok {
		t.Fatal("pre-existing handler should still be registered")
	}

	// Plugin type should also be registered
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
	ResetState()
	defer ResetState()

	RegisterPlugin(PluginInfo{
		ID: "test-plugin-invalid-entries",
		KeyIdentityHandlers: []KeyIdentityHandlerRegistration{
			{Type: "", Handler: &mockHandler{}},           // empty type
			{Type: "nil_handler_type", Handler: nil},       // nil handler
			{Type: "valid_plugin_type", Handler: &mockHandler{}}, // valid
		},
	})

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
