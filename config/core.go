package config

import (
	"net"
	"strings"
	z "github.com/Oudwins/zog"
	"github.com/docker/go-units"
	"go.lumeweb.com/configmanager"
	"go.lumeweb.com/portal/config/types"
	"go.sia.tech/coreutils/wallet"
)

var (
	_ configmanager.ConfigSchemaProvider = (*CoreConfig)(nil)
	_ Defaults                           = (*CoreConfig)(nil)
)

type CoreConfig struct {
	DB              DatabaseConfig       `config:"db"`
	Domain          string               `config:"domain"`
	Secure          bool                 `config:"secure"`
	PortalName      string               `config:"portal_name"`
	ExternalPort    uint                 `config:"external_port"`
	Identity        types.Identity       `config:"identity"`
	Log             LogConfig            `config:"log"`
	Port            uint                 `config:"port"`
	PostUploadLimit uint64               `config:"post_upload_limit"`
	Storage         StorageConfig        `config:"storage"`
	Mail            MailConfig           `config:"mail"`
	Clustered       *ClusterConfig       `config:"clustered"`
	NodeID          types.UUID           `config:"node_id"`
	Cron            CronConfig           `config:"cron"`
	Account         AccountConfig        `config:"account"`
	Observability   ObservabilityConfig  `config:"observability"`
	DNSResolver     string               `config:"dns_resolver"`
	OAuth           OAuthConfig          `config:"oauth"`
}

func (c CoreConfig) Schema() z.ZogSchema {
	return z.Struct(z.Shape{
		"Domain": z.String().
			Required(z.Message("core.domain is required")),
		"PortalName": z.String().
			Required(z.Message("core.portal_name is required")),
		"Port": z.Uint().
			Required(z.Message("core.port is required")).
			GT(0, z.Message("core.port must be greater than 0")),
		"PostUploadLimit": ZogUInt64(),
		"Secure":          z.Bool(),
	}).TestFunc(func(data any, ctx z.Ctx) bool {
		c, ok := data.(*CoreConfig)
		if !ok {
			return true
		}

		if c.Clustered != nil && c.Clustered.Enabled && c.Clustered.Etcd == nil {
			ctx.AddIssue(ctx.Issue().SetMessage("etcd configuration is required when cluster is enabled"))
			return false
		}
		return true
	})
}

func (c CoreConfig) Defaults() map[string]any {
	return map[string]interface{}{
		"PostUploadLimit": units.MiB * 100,
		"NodeID":          types.NewUUID(),
		"Identity":        wallet.NewSeedPhrase(),
		"Domain":          "",
		"PortalName":      "",
		"Port":            8080,
		"Secure":          true,
		"DNSResolver":     "",
	}
}

func (c CoreConfig) ClusterEnabled() bool {
	return c.Clustered != nil && c.Clustered.Enabled
}

// DNSResolverAddr returns the DNS resolver host and port.
// If DNSResolver contains a port, it extracts both. Otherwise, it returns the host with default port 53.
func (c CoreConfig) DNSResolverAddr() (host string, port string) {
	if c.DNSResolver == "" {
		return "", ""
	}

	h, p, err := net.SplitHostPort(c.DNSResolver)
	if err != nil {
		// SplitHostPort fails if there's no port, which is expected.
		// Check if it's a bracketed IPv6 address without port.
		// Trim brackets and validate it's a valid IP address.
		host := strings.Trim(c.DNSResolver, "[]")
		if net.ParseIP(host) != nil && host != c.DNSResolver {
			// Was bracketed and is a valid IP
			return host, "53"
		}
		// Treat the entire string as the host and use default port.
		return c.DNSResolver, "53"
	}

	if p == "" {
		return h, "53"
	}

	return h, p
}

// DNSResolverString returns the DNS resolver address in host:port format.
// Returns empty string if no DNS resolver is configured.
func (c CoreConfig) DNSResolverString() string {
	host, port := c.DNSResolverAddr()
	if host == "" {
		return ""
	}
	return net.JoinHostPort(host, port)
}
