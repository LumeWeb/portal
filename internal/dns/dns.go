package dns

import (
	"context"
	"fmt"
	"net"
	"time"

	"go.lumeweb.com/portal/core"
	"go.uber.org/zap"
)

// SetupDNSResolver configures the default DNS resolver to use a custom DNS server if specified.
// This affects all networking operations including HTTP clients, SMTP, and other network connections.
func SetupDNSResolver(dnsResolver string, logger *core.Logger) {
	if dnsResolver == "" {
		return
	}

	if logger != nil {
		logger.Info("Setting custom DNS resolver", zap.String("dns_resolver", dnsResolver))
	}

	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{
				Timeout: time.Millisecond * time.Duration(3000),
			}
			return d.DialContext(ctx, network, net.JoinHostPort(dnsResolver, "53"))
		},
	}
}

// CustomDialer returns a dial function that uses a custom DNS resolver.
// Required for libraries using net.Dialer{} directly, which bypasses net.DefaultResolver.
func CustomDialer(dnsResolver string) func(ctx context.Context, network, address string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, fmt.Errorf("failed to split host:port: %w", err)
		}

		// Create a resolver that uses the custom DNS server
		resolver := &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, netType, addr string) (net.Conn, error) {
				d := net.Dialer{
					Timeout: time.Millisecond * time.Duration(3000),
				}
				return d.DialContext(ctx, netType, net.JoinHostPort(dnsResolver, "53"))
			},
		}

		// Resolve the hostname using the custom resolver
		ips, err := resolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve %s: %w", host, err)
		}

		if len(ips) == 0 {
			return nil, fmt.Errorf("no IPs found for %s", host)
		}

		// Use the first IP address to dial
		targetAddr := net.JoinHostPort(ips[0].String(), port)

		d := net.Dialer{}
		return d.DialContext(ctx, network, targetAddr)
	}
}
