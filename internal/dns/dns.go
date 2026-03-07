package dns

import (
	"context"
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
	if dnsResolver == "" {
		return func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{}
			return d.DialContext(ctx, network, address)
		}
	}

	return func(ctx context.Context, network, address string) (net.Conn, error) {
		d := net.Dialer{
			Timeout: time.Second * 30,
			Resolver: &net.Resolver{
				PreferGo: true,
				Dial: func(ctx context.Context, netType, addr string) (net.Conn, error) {
					dd := net.Dialer{
						Timeout: time.Millisecond * time.Duration(3000),
					}
					return dd.DialContext(ctx, netType, net.JoinHostPort(dnsResolver, "53"))
				},
			},
		}
		return d.DialContext(ctx, network, address)
	}
}
