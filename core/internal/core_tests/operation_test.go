package core_tests

import (
	"go.lumeweb.com/portal/core"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatOperationName(t *testing.T) {
	tests := []struct {
		name     string
		parts    []string
		expected string
	}{
		{
			name:     "single part",
			parts:    []string{"ipfs"},
			expected: "ipfs",
		},
		{
			name:     "two parts",
			parts:    []string{"ipfs", "pin"},
			expected: "ipfs.pin",
		},
		{
			name:     "three parts",
			parts:    []string{"ipfs", "pin", "car"},
			expected: "ipfs.pin.car",
		},
		{
			name:     "no parts",
			parts:    []string{},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := core.OperationName("", tt.parts...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestOperationName(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		parts    []string
		expected string
	}{
		{
			name:     "no additional parts",
			protocol: "ipfs",
			parts:    []string{},
			expected: "ipfs",
		},
		{
			name:     "single additional part",
			protocol: "ipfs",
			parts:    []string{"pin"},
			expected: "ipfs.pin",
		},
		{
			name:     "multiple additional parts",
			protocol: "ipfs",
			parts:    []string{"pin", "car"},
			expected: "ipfs.pin.car",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := core.OperationName(tt.protocol, tt.parts...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestOperationNameHelpers(t *testing.T) {
	protocol := "ipfs"

	tests := []struct {
		name     string
		fn       func(string) string
		expected string
	}{
		{
			name:     "StoreOperationName",
			fn:       core.StoreOperationName,
			expected: "ipfs.store",
		},
		{
			name:     "RetrieveOperationName",
			fn:       core.RetrieveOperationName,
			expected: "ipfs.retrieve",
		},
		{
			name:     "PublishOperationName",
			fn:       core.PublishOperationName,
			expected: "ipfs.publish",
		},
		{
			name:     "UploadOperationName",
			fn:       core.UploadOperationName,
			expected: "ipfs.upload",
		},
		{
			name:     "TUSUploadOperationName",
			fn:       core.TUSUploadOperationName,
			expected: "ipfs.tus.upload",
		},
		{
			name:     "PostUploadOperationName",
			fn:       core.PostUploadOperationName,
			expected: "ipfs.post.upload",
		},
		{
			name:     "ScanOperationName",
			fn:       core.ScanOperationName,
			expected: "ipfs.scan",
		},
		{
			name:     "UnstoreOperationName",
			fn:       core.UnstoreOperationName,
			expected: "ipfs.unstore",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.fn(protocol)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPrefixedOperationName(t *testing.T) {
	tests := []struct {
		name      string
		protocol  string
		prefix    string
		opType    core.OperationType
		expected  string
	}{
		{
			name:     "tus upload",
			protocol: "ipfs",
			prefix:   "tus",
			opType:   core.OpTypeUpload,
			expected: "ipfs.tus.upload",
		},
		{
			name:     "post upload",
			protocol: "ipfs",
			prefix:   "post",
			opType:   core.OpTypeUpload,
			expected: "ipfs.post.upload",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := core.OperationName(tt.protocol, tt.prefix, string(tt.opType))
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestOperationNameWithParts(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		parts    []string
		expected string
	}{
		{
			name:     "no parts",
			protocol: "ipfs",
			parts:    []string{},
			expected: "ipfs",
		},
		{
			name:     "one part",
			protocol: "ipfs",
			parts:    []string{"pin"},
			expected: "ipfs.pin",
		},
		{
			name:     "multiple parts",
			protocol: "ipfs",
			parts:    []string{"pin", "car", "v1"},
			expected: "ipfs.pin.car.v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := core.OperationName(tt.protocol, tt.parts...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatOperationNameHelper(t *testing.T) {
	tests := []struct {
		name     string
		parts    []string
		expected string
	}{
		{
			name:     "single part",
			parts:    []string{"ipfs"},
			expected: "ipfs",
		},
		{
			name:     "two parts",
			parts:    []string{"ipfs", "pin"},
			expected: "ipfs.pin",
		},
		{
			name:     "three parts",
			parts:    []string{"ipfs", "pin", "car"},
			expected: "ipfs.pin.car",
		},
		{
			name:     "no parts",
			parts:    []string{},
			expected: "",
		},
		{
			name:     "four parts",
			parts:    []string{"ipfs", "pin", "car", "v1"},
			expected: "ipfs.pin.car.v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Since formatOperationName is private, we test it indirectly
			// through OperationName with empty protocol
			result := core.OperationName("", tt.parts...)
			assert.Equal(t, tt.expected, result)
		})
	}
}
