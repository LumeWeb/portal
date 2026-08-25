package dns

import (
	"context"
	"net"
	"testing"

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
			originalResolver := net.DefaultResolver
			defer func() {
				net.DefaultResolver = originalResolver
			}()

			logger := core.NewLogger(nil, zap.NewNop())
			SetupDNSResolver(tt.dnsResolver, logger)

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
	ctx := context.Background()

	t.Run("invalid address format", func(t *testing.T) {
		dialer := CustomDialer("127.0.0.1")
		if dialer == nil {
			t.Fatal("Expected non-nil dialer function")
		}
		_, err := dialer(ctx, "tcp", "invalid-address")
		if err == nil {
			t.Error("Expected error for invalid address format")
		}
	})

	t.Run("empty resolver returns plain dialer", func(t *testing.T) {
		if dialer := CustomDialer(""); dialer == nil {
			t.Error("Expected non-nil dialer for empty resolver")
		}
	})
}
