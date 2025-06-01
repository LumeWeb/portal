package types

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

var (
	testUUID = "05d1b68e-3eae-11f0-9fe2-0242ac120002"
)

func TestBinaryUUID_MarshalUnmarshalJSON(t *testing.T) {
	expected := ParseUUID(testUUID)

	// Marshal to JSON
	jsonData, err := json.Marshal(expected)
	assert.NoError(t, err)
	assert.Equal(t, `"`+testUUID+`"`, string(jsonData))

	// Unmarshal back
	var actual BinaryUUID
	err = json.Unmarshal(jsonData, &actual)
	assert.NoError(t, err)
	assert.Equal(t, expected, actual)

	// Test invalid UUID
	invalidJSON := []byte(`"not-a-uuid"`)
	err = json.Unmarshal(invalidJSON, &actual)
	assert.Error(t, err)
}

func TestParseUUID(t *testing.T) {
	t.Run("valid UUID", func(t *testing.T) {
		uuid := ParseUUID(testUUID)
		assert.Equal(t, testUUID, uuid.String())
	})

	t.Run("empty string produces Nil UUID without panic", func(t *testing.T) {
		assert.NotPanics(t, func() {
			empty := ParseUUID("")
			assert.True(t, empty.IsNil())
		})
	})

	t.Run("invalid UUID panics", func(t *testing.T) {
		assert.Panics(t, func() {
			ParseUUID("invalid-uuid-string")
		})
	})
}

func TestBinaryUUID_String(t *testing.T) {
	uuid := ParseUUID(testUUID)
	assert.Equal(t, testUUID, uuid.String())
}

func TestBinaryUUID_IsNil(t *testing.T) {
	// Test Nil UUID
	nilUUID := BinaryUUID{}
	assert.True(t, nilUUID.IsNil())

	// Test non-Nil UUID
	nonNil := ParseUUID("6ba7b810-9dad-11d1-80b4-00c04fd430c8")
	assert.False(t, nonNil.IsNil())
}

func TestNewBinUUID(t *testing.T) {
	// Generate new UUIDs
	uuid1 := NewBinUUID()
	uuid2 := NewBinUUID()

	// Should not be nil
	assert.False(t, uuid1.IsNil())
	assert.False(t, uuid2.IsNil())

	// Should be valid UUIDs
	assert.NotEqual(t, uuid1.String(), uuid2.String())
	assert.Len(t, uuid1.String(), 36)
	assert.Len(t, uuid2.String(), 36)

	// Should be v4 UUIDs (check format xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx)
	assert.Equal(t, byte('4'), uuid1.String()[14])
	assert.Equal(t, byte('4'), uuid2.String()[14])
}
