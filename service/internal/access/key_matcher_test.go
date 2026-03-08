package access

import (
	"testing"
)

func TestKeyMatchEcho(t *testing.T) {
	tests := []struct {
		name     string
		key1     string
		key2     string
		expected bool
	}{
		// Basic :param tests
		{
			name:     "exact match",
			key1:     "/api/account/keys",
			key2:     "/api/account/keys",
			expected: true,
		},
		{
			name:     "single :param",
			key1:     "/api/account/keys/ec6a9c56-2d04-4450-b6e3-1d10985e275b",
			key2:     "/api/account/keys/:keyID",
			expected: true,
		},
		{
			name:     "multiple :params",
			key1:     "/api/account/123/keys/456",
			key2:     "/api/account/:accountID/keys/:keyID",
			expected: true,
		},
		{
			name:     "UUID :param",
			key1:     "/api/account/keys/ec6a9c56-2d04-4450-b6e3-1d10985e275b",
			key2:     "/api/account/keys/:keyID",
			expected: true,
		},
		{
			name:     "numeric :param",
			key1:     "/api/account/keys/12345",
			key2:     "/api/account/keys/:keyID",
			expected: true,
		},
		{
			name:     "no match - missing segment",
			key1:     "/api/account/keys",
			key2:     "/api/account/keys/:keyID",
			expected: false,
		},
		{
			name:     "no match - extra segment",
			key1:     "/api/account/keys/123/extra",
			key2:     "/api/account/keys/:keyID",
			expected: false,
		},
		{
			name:     "no match - wrong path",
			key1:     "/api/account/keys/123",
			key2:     "/api/account/users/:userID",
			expected: false,
		},

		// :param/* sub-resource tests
		{
			name:     "sub-resource with one segment",
			key1:     "/api/account/keys/123/subresource",
			key2:     "/api/account/keys/:keyID/*",
			expected: true,
		},
		{
			name:     "sub-resource with multiple segments",
			key1:     "/api/account/keys/123/sub/resource/path",
			key2:     "/api/account/keys/:keyID/*",
			expected: true,
		},
		{
			name:     "sub-resource with UUID",
			key1:     "/api/account/keys/ec6a9c56-2d04-4450-b6e3-1d10985e275b/subresource",
			key2:     "/api/account/keys/:keyID/*",
			expected: true,
		},
		{
			name:     "sub-resource no match - ends at param",
			key1:     "/api/account/keys/123",
			key2:     "/api/account/keys/:keyID/*",
			expected: false,
		},
		{
			name:     "sub-resource no match - missing param",
			key1:     "/api/account/keys/subresource",
			key2:     "/api/account/keys/:keyID/*",
			expected: false,
		},

		// Wildcard tests (standard KeyMatch2 behavior)
		{
			name:     "wildcard basic",
			key1:     "/api/account/keys",
			key2:     "/api/account/*",
			expected: true,
		},
		{
			name:     "wildcard with segment",
			key1:     "/api/account/keys/123",
			key2:     "/api/account/*",
			expected: true,
		},
		{
			name:     "wildcard no match",
			key1:     "/api/users/123",
			key2:     "/api/account/*",
			expected: false,
		},

		// Edge cases
		{
			name:     "empty strings",
			key1:     "",
			key2:     "",
			expected: true,
		},
		{
			name:     "root path",
			key1:     "/",
			key2:     "/",
			expected: true,
		},
		{
			name:     "trailing slashes - both",
			key1:     "/api/account/keys/",
			key2:     "/api/account/keys/",
			expected: true,
		},
		{
			name:     "trailing slashes - pattern only",
			key1:     "/api/account/keys",
			key2:     "/api/account/keys/",
			expected: false,
		},
		{
			name:     "trailing slashes - key only",
			key1:     "/api/account/keys/",
			key2:     "/api/account/keys",
			expected: false,
		},
		{
			name:     "complex nested path with params",
			key1:     "/api/v1/accounts/123/keys/456/actions/789",
			key2:     "/api/v1/accounts/:accountID/keys/:keyID/actions/:actionID",
			expected: true,
		},
		{
			name:     "complex nested path with sub-resources",
			key1:     "/api/v1/accounts/123/keys/456/sub/extra/path",
			key2:     "/api/v1/accounts/:accountID/keys/:keyID/*",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := KeyMatchEcho(tt.key1, tt.key2)
			if result != tt.expected {
				t.Errorf("KeyMatchEcho(%q, %q) = %v, expected %v", tt.key1, tt.key2, result, tt.expected)
			}
		})
	}
}

func TestKeyMatchEchoFunc(t *testing.T) {
	tests := []struct {
		name     string
		args     []interface{}
		expected bool
		wantErr  bool
	}{
		{
			name:     "basic match",
			args:     []interface{}{"/api/account/keys/123", "/api/account/keys/:keyID"},
			expected: true,
			wantErr:  false,
		},
		{
			name:     "no match",
			args:     []interface{}{"/api/account/keys", "/api/account/keys/:keyID"},
			expected: false,
			wantErr:  false,
		},
		{
			name:     "sub-resource match",
			args:     []interface{}{"/api/account/keys/123/sub", "/api/account/keys/:keyID/*"},
			expected: true,
			wantErr:  false,
		},
		{
			name:     "wrong number of args - one",
			args:     []interface{}{"/api/account/keys/123"},
			expected: false,
			wantErr:  false,
		},
		{
			name:     "wrong number of args - three",
			args:     []interface{}{"/api/account/keys/123", "/api/account/keys/:keyID", "extra"},
			expected: false,
			wantErr:  false,
		},
		{
			name:     "non-string args",
			args:     []interface{}{123, "/api/account/keys/:keyID"},
			expected: false,
			wantErr:  false,
		},
		{
			name:     "both non-string args",
			args:     []interface{}{123, 456},
			expected: false,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := KeyMatchEchoFunc(tt.args...)
			if (err != nil) != tt.wantErr {
				t.Errorf("KeyMatchEchoFunc() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if result != tt.expected {
				t.Errorf("KeyMatchEchoFunc() = %v, expected %v", result, tt.expected)
			}
		})
	}
}
