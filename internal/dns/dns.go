package dns

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-kiss/monkey"
	"github.com/miekg/dns"
	"go.lumeweb.com/portal/core"
	"go.uber.org/zap"
)

var (
	customDNSAddr atomic.Value
	dnsClient     = &dns.Client{Timeout: 5 * time.Second}
)

func queryDNS(ctx context.Context, qtype uint16, name string) (*dns.Msg, error) {
	addr, _ := customDNSAddr.Load().(string)
	if addr == "" {
		return nil, nil
	}

	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(name), qtype)
	m.RecursionDesired = true

	r, _, err := dnsClient.ExchangeContext(ctx, m, addr)
	return r, err
}

func parseDNSRecords[T any](r *dns.Msg, extract func(dns.RR) *T, isNXDOMAIN bool) []*T {
	if r == nil {
		return nil
	}

	var results []*T
	for _, ans := range r.Answer {
		if extracted := extract(ans); extracted != nil {
			results = append(results, extracted)
		}
	}

	if len(results) == 0 && isNXDOMAIN && r.Rcode == dns.RcodeNameError {
		return nil
	}

	return results
}

func lookupOrFallback[T any](r *dns.Msg, results T, fallback func(context.Context, string) (T, error), ctx context.Context, name string) (T, error) {
	var zero T

	resultsSlice := reflect.ValueOf(results)
	if resultsSlice.Kind() == reflect.Slice && resultsSlice.Len() == 0 {
		return fallback(ctx, name)
	}

	if r != nil && r.Rcode == dns.RcodeNameError {
		return zero, &net.DNSError{Name: name, Err: "no such host", IsNotFound: true}
	}

	return results, nil
}

func isNXDomain(r *dns.Msg) bool {
	return r != nil && r.Rcode == dns.RcodeNameError
}

func customLookupMX(ctx context.Context, name string) ([]*net.MX, error) {
	r, err := queryDNS(ctx, dns.TypeMX, name)
	if err != nil {
		return nil, err
	}

	mxRecords := parseDNSRecords(r, func(rr dns.RR) *net.MX {
		if mx, ok := rr.(*dns.MX); ok {
			return &net.MX{Host: mx.Mx, Pref: uint16(mx.Preference)}
		}
		return nil
	}, true)

	// Sort MX records by preference (lower preference value = higher priority)
	sort.Slice(mxRecords, func(i, j int) bool {
		return mxRecords[i].Pref < mxRecords[j].Pref
	})

	return lookupOrFallback(r, mxRecords, net.DefaultResolver.LookupMX, ctx, name)
}

func customLookupTXT(ctx context.Context, name string) ([]string, error) {
	r, err := queryDNS(ctx, dns.TypeTXT, name)
	if err != nil {
		return nil, err
	}

	if r != nil {
		var txts []string
		for _, ans := range r.Answer {
			if txt, ok := ans.(*dns.TXT); ok {
				txts = append(txts, strings.Join(txt.Txt, ""))
			}
		}

		if len(txts) > 0 {
			return txts, nil
		}
	}

	return net.DefaultResolver.LookupTXT(ctx, name)
}

func customLookupCNAME(ctx context.Context, host string) (string, error) {
	r, err := queryDNS(ctx, dns.TypeCNAME, host)
	if err != nil {
		return "", err
	}

	if r != nil {
		for _, ans := range r.Answer {
			if c, ok := ans.(*dns.CNAME); ok {
				return c.Target, nil
			}
		}
		if isNXDomain(r) {
			return "", &net.DNSError{Name: host, Err: "no such host", IsNotFound: true}
		}
	}

	return net.DefaultResolver.LookupCNAME(ctx, host)
}

func customLookupNS(ctx context.Context, name string) ([]*net.NS, error) {
	r, err := queryDNS(ctx, dns.TypeNS, name)
	if err != nil {
		return nil, err
	}

	nsRecords := parseDNSRecords(r, func(rr dns.RR) *net.NS {
		if ns, ok := rr.(*dns.NS); ok {
			return &net.NS{Host: ns.Ns}
		}
		return nil
	}, true)

	return lookupOrFallback(r, nsRecords, net.DefaultResolver.LookupNS, ctx, name)
}

func customLookupAddr(ctx context.Context, addr string) ([]string, error) {
	addr = strings.TrimSuffix(addr, ".")

	qname := addr
	if !strings.Contains(addr, ":") {
		parts := strings.Split(addr, ".")
		qname = ""
		for i := len(parts) - 1; i >= 0; i-- {
			qname += parts[i] + "."
		}
		qname += "in-addr.arpa"
	} else {
		// IPv6 reverse lookup: convert to ip6.arpa format
		ip := net.ParseIP(addr)
		if ip != nil {
			ip = ip.To16()
			qname = ""
			for i := len(ip) - 1; i >= 0; i-- {
				qname += fmt.Sprintf("%x.%x.", ip[i]&0xf, ip[i]>>4)
			}
			qname += "ip6.arpa"
		}
	}

	r, err := queryDNS(ctx, dns.TypePTR, qname)
	if err != nil {
		return nil, err
	}

	if r != nil {
		var names []string
		for _, ans := range r.Answer {
			if ptr, ok := ans.(*dns.PTR); ok {
				names = append(names, ptr.Ptr)
			}
		}

		if len(names) > 0 {
			return names, nil
		}

		if isNXDomain(r) {
			return nil, &net.DNSError{Name: addr, Err: "no such host", IsNotFound: true}
		}
	}

	return net.DefaultResolver.LookupAddr(ctx, addr)
}

func customLookupSRV(ctx context.Context, service, proto, name string) (string, []*net.SRV, error) {
	qname := "_" + service + "._" + proto + "." + dns.Fqdn(name)
	r, err := queryDNS(ctx, dns.TypeSRV, qname)
	if err != nil {
		return "", nil, err
	}

	srvRecords := parseDNSRecords(r, func(rr dns.RR) *net.SRV {
		if srv, ok := rr.(*dns.SRV); ok {
			return &net.SRV{
				Target:   srv.Target,
				Port:     uint16(srv.Port),
				Priority: uint16(srv.Priority),
				Weight:   uint16(srv.Weight),
			}
		}
		return nil
	}, true)

	// Sort SRV records by priority (lower = higher priority) then by weight (higher = higher priority within same priority)
	sort.Slice(srvRecords, func(i, j int) bool {
		pi := srvRecords[i].Priority
		pj := srvRecords[j].Priority
		if pi != pj {
			return pi < pj
		}
		// Higher weight first
		return srvRecords[i].Weight > srvRecords[j].Weight
	})

	if len(srvRecords) == 0 {
		if r != nil && isNXDomain(r) {
			return "", nil, &net.DNSError{Name: name, Err: "no such host", IsNotFound: true}
		}
		return net.DefaultResolver.LookupSRV(ctx, service, proto, name)
	}
	return "", srvRecords, nil
}

func customLookupHost(ctx context.Context, host string) ([]string, error) {
	rA, err := queryDNS(ctx, dns.TypeA, host)
	if err != nil {
		return nil, err
	}

	var aRecords []net.IP
	if rA != nil {
		aRecordPtrs := parseDNSRecords(rA, func(rr dns.RR) *net.IP {
			if a, ok := rr.(*dns.A); ok {
				ip := net.IP(a.A)
				return &ip
			}
			return nil
		}, false)

		for _, ptr := range aRecordPtrs {
			aRecords = append(aRecords, *ptr)
		}
	}

	isNXDomainResult := isNXDomain(rA)

	rAAAA, err := queryDNS(ctx, dns.TypeAAAA, host)
	if err == nil && rAAAA != nil {
		aaaaRecordPtrs := parseDNSRecords(rAAAA, func(rr dns.RR) *net.IP {
			if aaaa, ok := rr.(*dns.AAAA); ok {
				ip := net.IP(aaaa.AAAA)
				return &ip
			}
			return nil
		}, false)

		for _, ptr := range aaaaRecordPtrs {
			aRecords = append(aRecords, *ptr)
		}
		
		if isNXDomain(rAAAA) {
			isNXDomainResult = true
		}
	}

	if len(aRecords) == 0 {
		if isNXDomainResult {
			return nil, &net.DNSError{Name: host, Err: "no such host", IsNotFound: true}
		}
		return net.DefaultResolver.LookupHost(ctx, host)
	}

	addrs := make([]string, len(aRecords))
	for i, ip := range aRecords {
		addrs[i] = ip.String()
	}
	return addrs, nil
}

func customLookupIP(ctx context.Context, network, host string) ([]net.IP, error) {
	switch network {
	case "ip4":
		// IPv4 only
		r, err := queryDNS(ctx, dns.TypeA, host)
		if err != nil {
			return nil, err
		}

		ipPtrs := parseDNSRecords(r, func(rr dns.RR) *net.IP {
			if a, ok := rr.(*dns.A); ok {
				ip := net.IP(a.A)
				return &ip
			}
			return nil
		}, true)

		ips := make([]net.IP, len(ipPtrs))
		for i, ptr := range ipPtrs {
			ips[i] = *ptr
		}

		if len(ips) == 0 {
			if isNXDomain(r) {
				return nil, &net.DNSError{Name: host, Err: "no such host", IsNotFound: true}
			}
			return net.DefaultResolver.LookupIP(ctx, network, host)
		}
		return ips, nil

	case "ip6":
		// IPv6 only
		r, err := queryDNS(ctx, dns.TypeAAAA, host)
		if err != nil {
			return nil, err
		}

		ipPtrs := parseDNSRecords(r, func(rr dns.RR) *net.IP {
			if aaaa, ok := rr.(*dns.AAAA); ok {
				ip := net.IP(aaaa.AAAA)
				return &ip
			}
			return nil
		}, true)

		ips := make([]net.IP, len(ipPtrs))
		for i, ptr := range ipPtrs {
			ips[i] = *ptr
		}

		if len(ips) == 0 {
			if isNXDomain(r) {
				return nil, &net.DNSError{Name: host, Err: "no such host", IsNotFound: true}
			}
			return net.DefaultResolver.LookupIP(ctx, network, host)
		}
		return ips, nil

	case "ip":
		// Both IPv4 and IPv6
		var ips []net.IP

		// Query A records (IPv4)
		rA, err := queryDNS(ctx, dns.TypeA, host)
		if err == nil && rA != nil {
			aRecordPtrs := parseDNSRecords(rA, func(rr dns.RR) *net.IP {
				if a, ok := rr.(*dns.A); ok {
					ip := net.IP(a.A)
					return &ip
				}
				return nil
			}, false)

			for _, ptr := range aRecordPtrs {
				ips = append(ips, *ptr)
			}
		}

		// Query AAAA records (IPv6)
		rAAAA, err := queryDNS(ctx, dns.TypeAAAA, host)
		if err == nil && rAAAA != nil {
			aaaaRecordPtrs := parseDNSRecords(rAAAA, func(rr dns.RR) *net.IP {
				if aaaa, ok := rr.(*dns.AAAA); ok {
					ip := net.IP(aaaa.AAAA)
					return &ip
				}
				return nil
			}, false)

			for _, ptr := range aaaaRecordPtrs {
				ips = append(ips, *ptr)
			}
		}

		if len(ips) == 0 {
			hasNXDomain := (rA != nil && isNXDomain(rA)) || (rAAAA != nil && isNXDomain(rAAAA))
			if hasNXDomain {
				return nil, &net.DNSError{Name: host, Err: "no such host", IsNotFound: true}
			}
			return net.DefaultResolver.LookupIP(ctx, network, host)
		}
		return ips, nil

	default:
		return net.DefaultResolver.LookupIP(ctx, network, host)
	}
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
	monkey.PatchInstanceMethod(resolverType, "LookupTXT", func(r *net.Resolver, ctx context.Context, name string) ([]string, error) {
		return customLookupTXT(ctx, name)
	})
	monkey.PatchInstanceMethod(resolverType, "LookupCNAME", func(r *net.Resolver, ctx context.Context, host string) (string, error) {
		return customLookupCNAME(ctx, host)
	})
	monkey.PatchInstanceMethod(resolverType, "LookupNS", func(r *net.Resolver, ctx context.Context, name string) ([]*net.NS, error) {
		return customLookupNS(ctx, name)
	})
	monkey.PatchInstanceMethod(resolverType, "LookupAddr", func(r *net.Resolver, ctx context.Context, addr string) ([]string, error) {
		return customLookupAddr(ctx, addr)
	})
	monkey.PatchInstanceMethod(resolverType, "LookupSRV", func(r *net.Resolver, ctx context.Context, service, proto, name string) (string, []*net.SRV, error) {
		return customLookupSRV(ctx, service, proto, name)
	})
	monkey.PatchInstanceMethod(resolverType, "LookupHost", func(r *net.Resolver, ctx context.Context, host string) ([]string, error) {
		return customLookupHost(ctx, host)
	})
	monkey.PatchInstanceMethod(resolverType, "LookupIP", func(r *net.Resolver, ctx context.Context, network, host string) ([]net.IP, error) {
		return customLookupIP(ctx, network, host)
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
