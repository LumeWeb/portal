package core

import (
	"context"
	"time"

	"go.lumeweb.com/oauth"
)

const OAUTH_PROVIDER_SERVICE = "oauth_provider"

// OAuthProtectedResource describes a resource server (e.g., an MCP server)
// that this authorization server is permitted to issue tokens for. Plugins
// register their resources during boot so the AS can validate RFC 8707
// resource parameters and serve RFC 9728 protected-resource metadata.
type OAuthProtectedResource struct {
	// ResourceURL is the canonical URI identifying the protected resource
	// (RFC 8707 §2). Tokens issued for this resource carry it as the audience.
	ResourceURL string
	// Scopes are the scope values this resource supports (RFC 9728
	// scopes_supported).
	Scopes []string
	// DisplayName is a human-readable name for the resource (RFC 9728
	// resource_name).
	DisplayName string
}

// OAuthProviderService provides OAuth 2.1 authorization-server logic. It is a
// thin adapter over the go.lumeweb.com/oauth library, wired into the portal's
// service infrastructure (Component, logging, tracing, GORM, config). The API
// layer wraps HTTP handlers around these methods.
type OAuthProviderService interface {
	Service

	// RegisterClient handles Dynamic Client Registration (RFC 7591 §3.1).
	// Accepts redirect_uris (REQUIRED), token_endpoint_auth_method ("none"
	// only), client_name, grant_types, response_types. Returns the persisted
	// client with a server-generated client_id.
	RegisterClient(ctx context.Context, reg oauth.ClientRegistration) (*oauth.Client, error)

	// ValidateAuthorizeRequest validates a parsed authorization request per
	// RFC 6749 §4.1.1 + RFC 7636 §4.3 (PKCE) + RFC 8707 (resource).
	ValidateAuthorizeRequest(ctx context.Context, req oauth.AuthorizeRequest) error

	// IssueAuthorizationCode creates a short-lived, single-use authorization
	// code bound to client_id, redirect_uri, code_challenge, resource, and
	// user_id.
	IssueAuthorizationCode(ctx context.Context, req oauth.AuthorizeRequest, userID uint) (string, error)

	// ExchangeCode validates the PKCE code_verifier (RFC 7636 §4.6),
	// atomically consumes the authorization code, and issues access + refresh
	// tokens (RFC 6749 §5.1).
	ExchangeCode(ctx context.Context, req oauth.TokenRequest) (*oauth.TokenResponse, error)

	// RefreshToken validates a refresh token per RFC 9700 §4.13 (rotation +
	// reuse detection) and issues a new access token + rotated refresh token.
	RefreshToken(ctx context.Context, req oauth.TokenRequest) (*oauth.TokenResponse, error)

	// ValidateAccessToken checks whether a bearer token (RFC 6750) is one this
	// server issued and has not expired (with clock-skew grace). Returns the
	// associated user_id and expiry if valid.
	ValidateAccessToken(ctx context.Context, token string) (userID uint, expiry time.Time, ok bool)

	// RevokeToken revokes an access token or an entire refresh token chain
	// (RFC 7009 §2.1).
	RevokeToken(ctx context.Context, token string) error

	// Metadata returns the RFC 8414 authorization server metadata document.
	Metadata(ctx context.Context) (*oauth.ASMetadata, error)

	// RegisterResource registers a protected resource (e.g., an MCP server)
	// that this AS is authorized to issue tokens for. Plugins call this
	// during their boot/init phase. Duplicate registrations for the same
	// ResourceURL are ignored.
	RegisterResource(ctx context.Context, reg OAuthProtectedResource) error

	// UnregisterResource removes a previously registered resource.
	UnregisterResource(ctx context.Context, resourceURL string) error

	// GetResource returns the registration for a resource URL, or nil if
	// no resource is registered with that URL.
	GetResource(ctx context.Context, resourceURL string) (*OAuthProtectedResource, error)

	// ProtectedResourceMetadata returns RFC 9728 metadata for the given
	// resource URL. The resource server (typically an MCP plugin) serves
	// this document at its own /.well-known/oauth-protected-resource path.
	ProtectedResourceMetadata(ctx context.Context, resourceURL string) (*oauth.ProtectedResourceMetadata, error)

	// Reap deletes expired auth codes, access tokens, refresh tokens, and
	// stale clients. Called periodically to bound table growth.
	Reap(ctx context.Context) error
}
