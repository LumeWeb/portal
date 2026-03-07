package dns

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
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
	dnsResolver := "127.0.0.1"
	dialer := CustomDialer(dnsResolver)

	if dialer == nil {
		t.Fatal("Expected non-nil dialer function")
	}

	ctx := context.Background()

	t.Run("invalid address format", func(t *testing.T) {
		_, err := dialer(ctx, "tcp", "invalid-address")
		if err == nil {
			t.Error("Expected error for invalid address format")
		}
	})

	t.Run("valid address format", func(t *testing.T) {
		conn, err := dialer(ctx, "tcp", "example.com:25")
		if err != nil && conn != nil {
			conn.Close()
		}
	})
}

const mockDNSPort = "15353"
const mockMXRecord = "mock-mx-server.test.local."

func startMockDNSServer(t *testing.T) (*dns.Server, string) {
	addr := "127.0.0.1:" + mockDNSPort
	server := &dns.Server{
		Addr: addr,
		Net:  "udp",
	}

	server.Handler = dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Answer = append(m.Answer, &dns.MX{
			Hdr: dns.RR_Header{
				Name:   r.Question[0].Name,
				Rrtype: dns.TypeMX,
				Class:  dns.ClassINET,
				Ttl:    3600,
			},
			Preference: 10,
			Mx:         mockMXRecord,
		})
		w.WriteMsg(m)
	})

	go func() {
		server.ListenAndServe()
	}()

	time.Sleep(1 * time.Second)
	return server, addr
}

func verifyMockMXRecord(mxRecords []*net.MX) bool {
	for _, mx := range mxRecords {
		if mx.Host == mockMXRecord {
			return true
		}
	}
	return false
}

func runSetupDNSResolverTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	originalResolver := net.DefaultResolver
	defer func() {
		net.DefaultResolver = originalResolver
	}()

	mockServer, mockAddr := startMockDNSServer(t)
	defer mockServer.Shutdown()

	SetupDNSResolver(mockAddr, nil)

	mxRecords, err := net.LookupMX("example.com")
	if err != nil {
		t.Fatalf("net.LookupMX error: %v", err)
	}

	if !verifyMockMXRecord(mxRecords) {
		t.Error("Expected mock MX record from SetupDNSResolver")
	}
}

func TestSetupDNSResolverIntegration(t *testing.T) {
	runSetupDNSResolverTest(t)
}
