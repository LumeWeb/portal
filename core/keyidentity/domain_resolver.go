// Package keyidentity provides canonical implementations of the
// core.KeyIdentityHandler interface for EVM (Ethereum) and Solana chains.
//
// The handlers implement the full CAIP-122 challenge/verify lifecycle:
//   - IssueChallenge generates a nonce and returns a SIWE/SIWS message
//   - VerifyProof validates the signed message, recovers the signer, and
//     compares it to the claimed key
//
// Handlers are registered by plugins via PluginInfo.KeyIdentityHandlers
// and looked up by type string through the portal core registry
// (core.GetKeyIdentityHandler).
//
// # Domain resolution
//
// Handlers need a domain string for CAIP-122 message construction (the
// "domain" field in SIWE/SIWS messages). Rather than reaching into a
// specific plugin's config, the handler accepts a DomainResolver at
// construction time. Plugins provide their own resolver that reads from
// their config.
package keyidentity

import "go.lumeweb.com/portal/core"

// DomainResolver resolves the CAIP-122 domain for challenge messages.
// Plugins provide their own implementation when constructing a handler,
// decoupling the handler from plugin-specific config types.
//
// The returned domain should be the full FQDN (e.g., "account.example.com")
// used as the relying party domain in the SIWE/SIWS message.
type DomainResolver interface {
	// ResolveDomain returns the full domain for the given context, or
	// an empty string if it cannot be determined.
	ResolveDomain(ctx core.Context) string
}

// DomainResolverFunc is a function adapter for DomainResolver.
type DomainResolverFunc func(core.Context) string

func (f DomainResolverFunc) ResolveDomain(ctx core.Context) string {
	return f(ctx)
}

// CoreDomainResolver returns a DomainResolver that reads the domain from
// portal core's config (ctx.Config().Config().Core.Domain). This is useful
// for plugins that don't have a subdomain or want to use the core domain
// directly.
func CoreDomainResolver() DomainResolver {
	return DomainResolverFunc(func(ctx core.Context) string {
		if ctx == nil {
			return ""
		}
		cfgMgr := ctx.Config()
		if cfgMgr == nil {
			return ""
		}
		cfg := cfgMgr.Config()
		if cfg == nil {
			return ""
		}
		return cfg.Core.Domain
	})
}
