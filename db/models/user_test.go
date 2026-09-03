package models

import "testing"

func TestIsReservedDomain(t *testing.T) {
	tests := []struct {
		email    string
		expected bool
	}{
		{"anon_0xabc@local.invalid", true},
		{"anon_0xabc@mail.LOCAL.INVALID", true},
		{"user@sub.localhost", true},
		{"user@example.com", false},
		{"user@example.invalid.evil.com", false},
		{"no-at-sign", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := isReservedDomain(tt.email); got != tt.expected {
			t.Errorf("isReservedDomain(%q) = %v, want %v", tt.email, got, tt.expected)
		}
	}
}

func TestVerifyUserEmail(t *testing.T) {
	if err := verifyUserEmail(""); err != nil {
		t.Errorf("verifyUserEmail(\"\") = %v, want nil", err)
	}

	anonEmail := "anon_0xabc@local.invalid"
	if err := verifyUserEmail(anonEmail); err != nil {
		t.Errorf("reserved non-routable email %q should pass syntax-only verification, got %v", anonEmail, err)
	}

	if err := verifyUserEmail("@local.invalid"); err == nil {
		t.Error("malformed reserved-domain email should fail syntax check")
	}
}
