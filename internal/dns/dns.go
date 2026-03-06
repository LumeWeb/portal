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

	logger.Info("Setting custom DNS resolver", zap.String("dns_resolver", dnsResolver))

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
