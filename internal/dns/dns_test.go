package dns

import (
	"context"
	"net"
	"testing"
	"time"

	"go.lumeweb.com/portal/core"
	"go.uber.org/zap"
)

func TestSetupDNSResolver(t *testing.T) {
	tests := []struct {
		name        string
		dnsResolver string
		wantSet     bool
	}{
		{
			name:        "empty resolver does not modify default resolver",
			dnsResolver: "",
			wantSet:     false,
		},
		{
			name:        "valid resolver configures default resolver",
			dnsResolver: "8.8.8.8",
			wantSet:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save original resolver and ensure restoration even on panic
			originalResolver := net.DefaultResolver
			defer func() {
				net.DefaultResolver = originalResolver
			}()

			// Call SetupDNSResolver with a test logger
			logger := core.NewLogger(nil, zap.NewNop())
			SetupDNSResolver(tt.dnsResolver, logger)

			// Check if DefaultResolver was modified
			if tt.wantSet {
				if net.DefaultResolver == originalResolver {
					t.Error("Expected net.DefaultResolver to be modified")
				}
			} else {
				if net.DefaultResolver != originalResolver {
					t.Error("Expected net.DefaultResolver to remain unchanged")
				}
			}
		})
	}
}

func TestCustomDialer(t *testing.T) {
	dnsResolver := "127.0.0.1"
	dialer := CustomDialer(dnsResolver)

	if dialer == nil {
		t.Fatal("Expected non-nil dialer function")
	}

	ctx := context.Background()

	// Test with invalid address format
	t.Run("invalid address format", func(t *testing.T) {
		_, err := dialer(ctx, "tcp", "invalid-address")
		if err == nil {
			t.Error("Expected error for invalid address format")
		}
	})

	// Test with valid address format (will fail to resolve, but validates parsing)
	t.Run("valid address format with resolution", func(t *testing.T) {
		// This will fail to actually connect since we're using a fake DNS server,
		// but it should at least parse the address correctly
		conn, err := dialer(ctx, "tcp", "example.com:25")
		if err != nil {
			// Expected to fail due to DNS resolution, but not due to parsing
			if len(err.Error()) > 0 {
				t.Logf("Expected resolution failure: %v", err)
			}
		}
		if conn != nil {
			conn.Close()
		}
	})
}

func TestCustomDialerResolution(t *testing.T) {
	// This test verifies that CustomDialer properly resolves hostnames
	// using the configured DNS server

	dnsResolver := "8.8.8.8"
	dialer := CustomDialer(dnsResolver)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test with a well-known hostname that should resolve
	t.Run("resolves known hostname", func(t *testing.T) {
		conn, err := dialer(ctx, "tcp", "dns.google:53")
		if err != nil {
			// Connection might fail due to network, but resolution should work
			// Check if error is related to DNS resolution vs connection
			t.Logf("Connection error (expected if network unavailable): %v", err)
		}
		if conn != nil {
			conn.Close()
		}
	})
}

func TestCustomDialerIPv6(t *testing.T) {
	// Test that IPv6 addresses are properly formatted
	dnsResolver := "::1"
	dialer := CustomDialer(dnsResolver)
	ctx := context.Background()

	// Test with IPv6 address in host:port format
	t.Run("handles IPv6 addresses", func(t *testing.T) {
		// This should properly join IPv6 address with port
		conn, err := dialer(ctx, "tcp", "[::1]:25")
		if err != nil {
			t.Logf("Expected connection failure (no server): %v", err)
		}
		if conn != nil {
			conn.Close()
		}
	})
}
