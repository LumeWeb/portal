package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.lumeweb.com/oauth"
	oautgorm "go.lumeweb.com/oauth/storage/gorm"
	"go.lumeweb.com/portal/core"
	"go.uber.org/zap"
)

var ErrResourceNotRegistered = errors.New("oauth: resource not registered")

var _ core.OAuthProviderService = (*OAuthProviderServiceDefault)(nil)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID:      core.OAUTH_PROVIDER_SERVICE,
		Factory: NewOAuthProviderService,
		Depends: []string{core.HTTP_SERVICE},
	})
}

// OAuthProviderServiceDefault is the portal's OAuth 2.1 authorization-server
// service. It wraps the go.lumeweb.com/oauth authorization server with the
// portal's GORM-backed storage and derives its issuer from the HTTP service.
type OAuthProviderServiceDefault struct {
	*core.BaseComponent
	as *oauth.AuthorizationServer
}

func NewOAuthProviderService() (core.Service, []core.ContextBuilderOption, error) {
	svc := &OAuthProviderServiceDefault{}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			http := core.GetService[core.HTTPService](ctx, core.HTTP_SERVICE)

			ocfg := ctx.Config().Config().Core.OAuth
			if !ocfg.Enabled {
				return nil
			}

			issuer := ocfg.Issuer
			if issuer == "" {
				issuer = http.APISubdomain("account", true)
			}
			if issuer == "" {
				return fmt.Errorf("oauth: issuer is empty; set oauth.issuer or register the 'account' API")
			}

			cfg := oauth.Config{Issuer: issuer}

			tokenTTL, err := parseTTL("token_ttl", ocfg.TokenTTL)
			if err != nil {
				return err
			}
			cfg.TokenTTL = tokenTTL

			refreshTTL, err := parseTTL("refresh_ttl", ocfg.RefreshTTL)
			if err != nil {
				return err
			}
			cfg.RefreshTTL = refreshTTL

			codeTTL, err := parseTTL("code_ttl", ocfg.CodeTTL)
			if err != nil {
				return err
			}
			cfg.CodeTTL = codeTTL

			storage, err := oautgorm.New(ctx.DB(), cfg)
			if err != nil {
				return err
			}

			ctx.Logger().Info("OAuth provider initialized", zap.String("issuer", issuer))
			svc.as = oauth.NewAuthorizationServer(cfg, storage)
			return nil
		}),
	)

	return svc, opts, nil
}

func (s *OAuthProviderServiceDefault) ID() string {
	return core.OAUTH_PROVIDER_SERVICE
}

// parseTTL parses a time.ParseDuration string from config. Empty values are
// allowed and yield a zero duration so the oauth library applies its built-in
// defaults; non-empty but malformed values fail startup rather than silently
// zeroing the token lifetime.
func parseTTL(name, value string) (time.Duration, error) {
	if value == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("oauth: invalid %s %q: %w", name, value, err)
	}
	return d, nil
}

func (s *OAuthProviderServiceDefault) RegisterClient(ctx context.Context, reg oauth.ClientRegistration) (*oauth.Client, error) {
	ctx, span := core.TraceMethod(ctx, "OAuthProviderServiceDefault.RegisterClient")
	defer span.End()
	return s.as.RegisterClient(reg)
}

func (s *OAuthProviderServiceDefault) ValidateAuthorizeRequest(ctx context.Context, req oauth.AuthorizeRequest) error {
	_, span := core.TraceMethod(ctx, "OAuthProviderServiceDefault.ValidateAuthorizeRequest")
	defer span.End()
	return s.as.ValidateAuthorizeRequest(req)
}

func (s *OAuthProviderServiceDefault) IssueAuthorizationCode(ctx context.Context, req oauth.AuthorizeRequest, userID uint) (string, error) {
	_, span := core.TraceMethod(ctx, "OAuthProviderServiceDefault.IssueAuthorizationCode")
	defer span.End()
	return s.as.IssueAuthorizationCode(req, userID)
}

func (s *OAuthProviderServiceDefault) ExchangeCode(ctx context.Context, req oauth.TokenRequest) (*oauth.TokenResponse, error) {
	_, span := core.TraceMethod(ctx, "OAuthProviderServiceDefault.ExchangeCode")
	defer span.End()
	return s.as.ExchangeCode(req)
}

func (s *OAuthProviderServiceDefault) RefreshToken(ctx context.Context, req oauth.TokenRequest) (*oauth.TokenResponse, error) {
	_, span := core.TraceMethod(ctx, "OAuthProviderServiceDefault.RefreshToken")
	defer span.End()
	return s.as.RefreshToken(req)
}

func (s *OAuthProviderServiceDefault) ValidateAccessToken(ctx context.Context, token string) (uint, time.Time, bool) {
	_, span := core.TraceMethod(ctx, "OAuthProviderServiceDefault.ValidateAccessToken")
	defer span.End()
	return s.as.ValidateAccessToken(token)
}

func (s *OAuthProviderServiceDefault) RevokeToken(ctx context.Context, token string) error {
	_, span := core.TraceMethod(ctx, "OAuthProviderServiceDefault.RevokeToken")
	defer span.End()
	return s.as.RevokeToken(token)
}

func (s *OAuthProviderServiceDefault) Metadata(ctx context.Context) (*oauth.ASMetadata, error) {
	_, span := core.TraceMethod(ctx, "OAuthProviderServiceDefault.Metadata")
	defer span.End()
	meta := oauth.BuildASMetadata(s.as.Config())
	return &meta, nil
}

func (s *OAuthProviderServiceDefault) RegisterResource(ctx context.Context, reg core.OAuthProtectedResource) error {
	_, span := core.TraceMethod(ctx, "OAuthProviderServiceDefault.RegisterResource")
	defer span.End()
	s.as.RegisterResource(oauth.Resource{
		ResourceURL: reg.ResourceURL,
		Scopes:      reg.Scopes,
		DisplayName: reg.DisplayName,
	})
	return nil
}

func (s *OAuthProviderServiceDefault) UnregisterResource(ctx context.Context, resourceURL string) error {
	_, span := core.TraceMethod(ctx, "OAuthProviderServiceDefault.UnregisterResource")
	defer span.End()
	s.as.UnregisterResource(resourceURL)
	return nil
}

func (s *OAuthProviderServiceDefault) GetResource(ctx context.Context, resourceURL string) (*core.OAuthProtectedResource, error) {
	_, span := core.TraceMethod(ctx, "OAuthProviderServiceDefault.GetResource")
	defer span.End()
	reg, ok := s.as.GetResource(resourceURL)
	if !ok {
		return nil, nil
	}
	return &core.OAuthProtectedResource{
		ResourceURL: reg.ResourceURL,
		Scopes:      reg.Scopes,
		DisplayName: reg.DisplayName,
	}, nil
}

func (s *OAuthProviderServiceDefault) ProtectedResourceMetadata(ctx context.Context, resourceURL string) (*oauth.ProtectedResourceMetadata, error) {
	_, span := core.TraceMethod(ctx, "OAuthProviderServiceDefault.ProtectedResourceMetadata")
	defer span.End()
	reg, ok := s.as.GetResource(resourceURL)
	if !ok {
		return nil, ErrResourceNotRegistered
	}
	issuer := s.as.Config().Issuer
	meta := oauth.BuildProtectedResourceMetadataFromResource(reg, issuer)
	return &meta, nil
}

func (s *OAuthProviderServiceDefault) Reap(ctx context.Context) error {
	_, span := core.TraceMethod(ctx, "OAuthProviderServiceDefault.Reap")
	defer span.End()
	return s.as.Reap()
}
