package core

import (
	"context"

	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/queryutil"
)

const SOCIAL_AUTH_SERVICE = "social_auth"

// SocialAuthResult is the outcome of SocialAuthService.LoginOrLink.
type SocialAuthResult struct {
	User    *models.User
	Created bool // true if a new account was created
	Linked  bool // true if a new SocialAccount link was created
	// EmailVerified reports whether the resolved account's email is verified.
	// Callers must not establish a session when it is false until the email has
	// been confirmed.
	EmailVerified bool
}

// SocialAuthService resolves external identities (Google, GitHub, etc.) to
// portal users. It owns the SocialAccount model and the link/lookup
// orchestration; the provider-specific OAuth client flow (redirect, PKCE,
// token exchange, userinfo) lives in plugins.
type SocialAuthService interface {
	Service

	// LookupByProvider finds the SocialAccount for a provider identity.
	// Returns the record if the identity is linked, or nil if not found.
	LookupByProvider(ctx context.Context, provider, providerUserID string) (*models.SocialAccount, error)

	// LoginOrLink resolves a social identity to a portal user.
	//   - SocialAccount exists         => returns the linked user (Created=false, Linked=false)
	//   - Email belongs to a user      => returns ErrSocialEmailConflict, NO auto-link
	//   - Otherwise                    => creates account + link, returns user (Created=true, Linked=true)
	// emailVerified reflects whether the provider response confirmed the email;
	// the created account is marked verified only when it is true.
	LoginOrLink(ctx context.Context, provider, providerUserID, email string, emailVerified bool) (*SocialAuthResult, error)

	// LinkAccount links a social identity to an already-authenticated user.
	// Errors if (provider, providerUserID) is already linked to another user.
	LinkAccount(ctx context.Context, userID uint, provider, providerUserID, email string) error

	// UnlinkAccount removes the (userID, provider) link. Returns
	// ErrSocialAccountNotFound if no such link exists.
	UnlinkAccount(ctx context.Context, userID uint, provider string) error

	// ListAccounts returns the social identities linked to a user, with
	// optional filtering, sorting and pagination. It returns the matching
	// accounts and the total count before pagination.
	ListAccounts(ctx context.Context, userID uint, filters []queryutil.CrudFilter, sorts []queryutil.Sort, pagination queryutil.Pagination) ([]*models.SocialAccount, int64, error)
}
