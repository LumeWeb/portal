package core

import (
	"encoding/json"
	"errors"
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

func TestNewErrorFormatVerbFallback(t *testing.T) {
	RegisterNamespace("format")
	MustRegisterDefaultErrorMessages("format", map[ErrorType]ErrorDefinition{
		"INVALID_REQUEST": {Key: "INVALID_REQUEST", Message: "Invalid request parameter: %s"},
		"PLAIN_MESSAGE":   {Key: "PLAIN_MESSAGE", Message: "Failed to process the file."},
		"NO_FORMAT":       {Key: "NO_FORMAT", Message: "Account creation failed"},
	})
	MustRegisterErrorCodes("format", map[ErrorType]int{
		"INVALID_REQUEST": 400,
		"PLAIN_MESSAGE":   500,
		"NO_FORMAT":       400,
	})

	t.Run("bridges err into format verb", func(t *testing.T) {
		e := NewError("format", "INVALID_REQUEST", errors.New("a domain is required"))
		assert.Equal(t, "Invalid request parameter: a domain is required", e.Message)
	})

	t.Run("leaves non-verb template untouched with err", func(t *testing.T) {
		e := NewError("format", "PLAIN_MESSAGE", errors.New("some detail"))
		// %!(EXTRA...) corruption would appear here if the fallback were naive.
		assert.Equal(t, "Failed to process the file.", e.Message)
	})

	t.Run("leaves message untouched when no err and no args", func(t *testing.T) {
		e := NewError("format", "NO_FORMAT", nil)
		assert.Equal(t, "Account creation failed", e.Message)
	})

	t.Run("explicit args take precedence over err", func(t *testing.T) {
		e := NewError("format", "INVALID_REQUEST", errors.New("ignored"), "explicit value")
		assert.Equal(t, "Invalid request parameter: explicit value", e.Message)
	})

	t.Run("template with verb but nil err stays unformatted", func(t *testing.T) {
		e := NewError("format", "INVALID_REQUEST", nil)
		assert.Equal(t, "Invalid request parameter: %s", e.Message)
	})
}
