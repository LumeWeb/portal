package indexd

import (
	"encoding/json"
	"testing"

	sdk "go.sia.tech/siastorage"
)

func TestSetObjectMetadata(t *testing.T) {
	obj := sdk.NewEmptyObject()

	err := SetObjectMetadata(&obj, "ipfs", "QmABC123")
	if err != nil {
		t.Fatalf("SetObjectMetadata failed: %v", err)
	}

	raw := obj.Metadata()
	if len(raw) == 0 {
		t.Fatal("expected metadata to be set, got empty")
	}

	var meta ObjectMetadata
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatalf("failed to unmarshal metadata: %v", err)
	}

	if meta.Protocol != "ipfs" {
		t.Errorf("expected protocol 'ipfs', got '%s'", meta.Protocol)
	}
	if meta.ObjectKey != "QmABC123" {
		t.Errorf("expected objectKey 'QmABC123', got '%s'", meta.ObjectKey)
	}
}

func TestSetObjectMetadata_OverwritesPreviousMetadata(t *testing.T) {
	obj := sdk.NewEmptyObject()

	// Set initial metadata.
	if err := SetObjectMetadata(&obj, "ipfs", "key1"); err != nil {
		t.Fatalf("first SetObjectMetadata failed: %v", err)
	}

	// Overwrite with new metadata.
	if err := SetObjectMetadata(&obj, "s3", "key2"); err != nil {
		t.Fatalf("second SetObjectMetadata failed: %v", err)
	}

	var meta ObjectMetadata
	if err := json.Unmarshal(obj.Metadata(), &meta); err != nil {
		t.Fatalf("failed to unmarshal metadata: %v", err)
	}

	if meta.Protocol != "s3" || meta.ObjectKey != "key2" {
		t.Errorf("expected (s3, key2), got (%s, %s)", meta.Protocol, meta.ObjectKey)
	}
}

func TestObjectMetadata_JSONRoundtrip(t *testing.T) {
	original := ObjectMetadata{
		Protocol:  "ipfs",
		ObjectKey: "bafybeihh3ejuv4hbto36ocy5kumk6lqadropxxcqambjhw5bhdqydkxzdm",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded ObjectMetadata
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded != original {
		t.Errorf("roundtrip mismatch: got %+v, want %+v", decoded, original)
	}
}
