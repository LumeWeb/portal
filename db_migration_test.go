package portal

import (
	"strings"
	"testing"
)

func TestSanitizeIdent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "plugin",
		},
		{
			name:     "valid identifier",
			input:    "valid_plugin_name",
			expected: "valid_plugin_name",
		},
		{
			name:     "uppercase letters",
			input:    "MyPlugin",
			expected: "myplugin",
		},
		{
			name:     "special characters",
			input:    "my-plugin@v1.0",
			expected: "my_plugin_v1_0",
		},
		{
			name:     "starts with digit",
			input:    "1plugin",
			expected: "_1plugin",
		},
		{
			name:     "starts with special character",
			input:    "-plugin",
			expected: "_plugin",
		},
		{
			name:     "mixed invalid characters",
			input:    "plugin!@#$%^&*()",
			expected: "plugin__________",
		},
		{
			name:     "very long identifier",
			input:    "a_very_long_plugin_identifier_that_exceeds_the_maximum_length_allowed_for_sql_identifiers",
			expected: "a_very_long_plugin_identifier_that_exceeds_the_maximum_length_al",
		},
		{
			name:     "exactly 64 characters",
			input:    "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefgh1234",
			expected: "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefgh1234",
		},
		{
			name:     "65 characters should be truncated",
			input:    "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklm",
			expected: "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijkl",
		},
		{
			name:     "unicode characters",
			input:    "pluginαβγ",
			expected: "plugin___",
		},
		{
			name:     "whitespace characters",
			input:    "plugin with spaces",
			expected: "plugin_with_spaces",
		},
		{
			name:     "numbers in middle",
			input:    "plugin123test",
			expected: "plugin123test",
		},
		{
			name:     "underscore preservation",
			input:    "my_plugin_v1",
			expected: "my_plugin_v1",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := sanitizeIdent(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeIdent(%q) = %q, want %q", tt.input, result, tt.expected)
			}
			if len(result) > 64 {
				t.Errorf("sanitizeIdent(%q) length = %d, exceeds 64", tt.input, len(result))
			}
		})
	}
}

func TestPluginTableName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "short plugin name",
			input:    "myplugin",
			expected: "goose_myplugin_version",
		},
		{
			name:     "plugin name with special characters",
			input:    "my-plugin@v1.0",
			expected: "goose_my_plugin_v1_0_version",
		},
		{
			name:     "long plugin name that gets truncated",
			input:    "a_very_long_plugin_identifier_that_exceeds_the_maximum_length_allowed_for_sql_identifiers_and_more",
			expected: "goose_a_very_long_plugin_identifier_that_exceeds_the_max_version",
		},
		{
			name:     "empty plugin name",
			input:    "",
			expected: "goose_plugin_version",
		},
		{
			name:     "plugin name starting with digit",
			input:    "1plugin",
			expected: "goose__1plugin_version",
		},
		{
			name:     "extremely long plugin name",
			input:    "this_is_an_extremely_long_plugin_name_that_will_definitely_exceed_the_limit_and_require_truncation",
			expected: "goose_this_is_an_extremely_long_plugin_name_that_will_de_version",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := pluginTableName(tt.input)
			if result != tt.expected {
				t.Errorf("pluginTableName(%q) = %q, want %q", tt.input, result, tt.expected)
			}

			// Verify that the result is always <= 64 characters
			if len(result) > 64 {
				t.Errorf("pluginTableName(%q) = %q, which exceeds 64 characters (length: %d)", tt.input, result, len(result))
			}

			// Stable format invariants
			if !strings.HasPrefix(result, "goose_") || !strings.HasSuffix(result, "_version") {
				t.Errorf("pluginTableName(%q) must start with 'goose_' and end with '_version', got %q", tt.input, result)
			}
		})
	}
}
