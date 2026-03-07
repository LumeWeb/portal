package dns

import (
	"context"
	"net"
	"reflect"
	"sync/atomic"
	"time"

	"github.com/go-kiss/monkey"
	"github.com/miekg/dns"
	"go.lumeweb.com/portal/core"
	"go.uber.org/zap"
)

var customDNSAddr atomic.Value // stores string

func customLookupMX(ctx context.Context, name string) ([]*net.MX, error) {
	addr, _ := customDNSAddr.Load().(string)
	if addr == "" {
		return net.DefaultResolver.LookupMX(ctx, name)
	}

	c := &dns.Client{Timeout: 5 * time.Second}
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(name), dns.TypeMX)
	m.RecursionDesired = true

	r, _, err := c.Exchange(m, addr)
	if err != nil {
		return nil, err
	}

	var mxRecords []*net.MX
	for _, ans := range r.Answer {
		if mx, ok := ans.(*dns.MX); ok {
			mxRecords = append(mxRecords, &net.MX{
				Host: mx.Mx,
				Pref: uint16(mx.Preference),
			})
		}
	}

	if len(mxRecords) == 0 {
		return nil, &net.DNSError{
			Name:       name,
			Err:        "no such host",
			IsNotFound: true,
		}
	}

	return mxRecords, nil
}

func SetupDNSResolver(dnsResolver string, logger *core.Logger) {
	if dnsResolver == "" {
		return
	}

	if logger != nil {
		logger.Info("Setting custom DNS resolver", zap.String("dns_resolver", dnsResolver))
	}

	customDNSAddr.Store(dnsResolver)

	customResolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 3 * time.Second}
			return d.DialContext(ctx, network, dnsResolver)
		},
	}

	net.DefaultResolver = customResolver

	resolverType := reflect.TypeOf(customResolver)
	monkey.PatchInstanceMethod(resolverType, "LookupMX", func(r *net.Resolver, ctx context.Context, name string) ([]*net.MX, error) {
		return customLookupMX(ctx, name)
	})
}

func CustomDialer(dnsResolver string) func(ctx context.Context, network, address string) (net.Conn, error) {
	if dnsResolver == "" {
		return func(ctx context.Context, network, address string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, address)
		}
	}

	return func(ctx context.Context, network, address string) (net.Conn, error) {
		d := net.Dialer{
			Timeout: 30 * time.Second,
			Resolver: &net.Resolver{
				PreferGo: true,
				Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
					dd := net.Dialer{Timeout: 3 * time.Second}
					return dd.DialContext(ctx, network, dnsResolver)
				},
			},
		}
		return d.DialContext(ctx, network, address)
	}
}
