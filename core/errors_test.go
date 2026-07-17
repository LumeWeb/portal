package core

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestError_MarshalJSON(t *testing.T) {
	RegisterNamespace("test")
	MustRegisterDefaultErrorMessages("test", map[ErrorType]ErrorDefinition{
		"ErrAccountCreationFailed": {Key: "ErrAccountCreationFailed", Message: "Account creation failed"},
		"FILE_UPLOAD_FAILED":        {Key: "FILE_UPLOAD_FAILED", Message: "File upload failed"},
	})
	MustRegisterErrorCodes("test", map[ErrorType]int{
		"ErrAccountCreationFailed": 400,
		"FILE_UPLOAD_FAILED":        500,
	})

	tests := []struct {
		name     string
		err      *Error
		expected map[string]any
	}{
		{
			name: "Err prefix stripped from key",
			err:  NewError("test", "ErrAccountCreationFailed", nil),
			expected: map[string]any{
				"error": map[string]any{
					"reason":  "AccountCreationFailed",
					"details": "Account creation failed",
				},
			},
		},
		{
			name: "SCREAMING_SNAKE_CASE converted to PascalCase",
			err:  NewError("test", "FILE_UPLOAD_FAILED", nil),
			expected: map[string]any{
				"error": map[string]any{
					"reason":  "FileUploadFailed",
					"details": "File upload failed",
				},
			},
		},
		{
			name: "nil pointer returns Unknown",
			err:  &Error{},
			expected: map[string]any{
				"error": map[string]any{
					"reason": "",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw, err := json.Marshal(tt.err)
			require.NoError(t, err)

			var result map[string]any
			require.NoError(t, json.Unmarshal(raw, &result))

			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestError_ImplementsMarshaler(t *testing.T) {
	var _ json.Marshaler = (*Error)(nil)
}
