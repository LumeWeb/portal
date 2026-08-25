package dns

import (
	"context"
	"net"
	"time"

	"go.lumeweb.com/portal/core"
	"go.uber.org/zap"
)

func newResolver(dnsResolver string) *net.Resolver {
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: 3 * time.Second}
			return d.DialContext(ctx, network, dnsResolver)
		},
	}
}

// SetupDNSResolver configures the process-wide net.DefaultResolver to route DNS
// lookups through the provided nameserver. An empty dnsResolver leaves the
// default resolver untouched.
//
// Relying on net.DefaultResolver (rather than an injectable resolver) is
// intentional: the email verifier and other consumers read net.DefaultResolver
// on every lookup, so a single assignment here covers all of them. See
// AfterShip/email-verifier issue #186, which added the option to supply a
// custom *net.Resolver and falls back to net.DefaultResolver when unset.
func SetupDNSResolver(dnsResolver string, logger *core.Logger) {
	if dnsResolver == "" {
		return
	}

	if logger != nil {
		logger.Info("Setting custom DNS resolver", zap.String("dns_resolver", dnsResolver))
	}

	net.DefaultResolver = newResolver(dnsResolver)
}

// CustomDialer returns a dial function that resolves hostnames through the
// provided nameserver. When dnsResolver is empty it returns a plain net.Dialer.
func CustomDialer(dnsResolver string) func(ctx context.Context, network, address string) (net.Conn, error) {
	if dnsResolver == "" {
		return func(ctx context.Context, network, address string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, address)
		}
	}

	return func(ctx context.Context, network, address string) (net.Conn, error) {
		d := net.Dialer{
			Timeout:  30 * time.Second,
			Resolver: newResolver(dnsResolver),
		}
		return d.DialContext(ctx, network, address)
	}
}
