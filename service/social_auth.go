package service

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"

	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/db/models"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var _ core.SocialAuthService = (*SocialAuthServiceDefault)(nil)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID:      core.SOCIAL_AUTH_SERVICE,
		Factory: NewSocialAuthService,
		Depends: []string{core.USER_SERVICE},
	})
}

// SocialAuthServiceDefault resolves external identities (Google, GitHub, etc.)
// to portal users. It owns the SocialAccount model and the link/lookup
// orchestration; the provider-specific OAuth client flow lives in plugins.
type SocialAuthServiceDefault struct {
	*core.BaseComponent
	user core.UserService
}

func NewSocialAuthService() (core.Service, []core.ContextBuilderOption, error) {
	svc := &SocialAuthServiceDefault{}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			svc.user = core.GetService[core.UserService](ctx, core.USER_SERVICE)
			return nil
		}),
	)

	return svc, opts, nil
}

func (s SocialAuthServiceDefault) ID() string {
	return core.SOCIAL_AUTH_SERVICE
}

// userService returns the USER_SERVICE dependency, falling back to a live
// registry lookup so the service also works if it was constructed without
// a startup function (e.g. in tests).
func (s SocialAuthServiceDefault) userService() core.UserService {
	if s.user != nil {
		return s.user
	}
	return core.GetService[core.UserService](s.Context(), core.USER_SERVICE)
}

func (s SocialAuthServiceDefault) LookupByProvider(ctx context.Context, provider, providerUserID string) (*models.SocialAccount, error) {
	ctx, span := core.TraceMethod(ctx, "SocialAuthServiceDefault.LookupByProvider")
	defer span.End()

	acct := &models.SocialAccount{}
	err := s.DB().WithContext(ctx).
		Where("provider = ? AND provider_user_id = ?", provider, providerUserID).
		First(acct).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, core.NewAccountError(core.ErrKeyDatabaseOperationFailed, err)
	}
	return acct, nil
}

func (s SocialAuthServiceDefault) LoginOrLink(ctx context.Context, provider, providerUserID, email string, emailVerified bool) (*core.SocialAuthResult, error) {
	ctx, span := core.TraceMethod(ctx, "SocialAuthServiceDefault.LoginOrLink")
	defer span.End()

	// Provider-uid-first: an existing link wins, even if the email changed.
	existing, err := s.LookupByProvider(ctx, provider, providerUserID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		user, err := s.userForAccount(ctx, existing.UserID)
		if err != nil {
			return nil, err
		}
		return &core.SocialAuthResult{User: user, EmailVerified: user.Verified}, nil
	}

	// Never auto-link to a portal account whose email is already taken. The
	// account is only registered after this check, so an unverified/conflicting
	// email never provisions a user.
	if email != "" {
		exists, _, err := s.userService().EmailExists(ctx, email)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, core.NewAccountError(core.ErrKeySocialEmailConflict, nil)
		}
	}

	// Register the account only now that the SSO identity is confirmed new and
	// its email is free. Registration goes through the canonical
	// UserService.CreateAccount path (no user SQL duplicated here).
	user, err := s.createAccount(ctx, email, emailVerified)
	if err != nil {
		return nil, err
	}

	if err := s.LinkAccount(ctx, user.ID, provider, providerUserID, email); err != nil {
		// A concurrent request may have linked the same identity in the
		// meantime; if so, the linked user wins and the account we just created
		// is discarded.
		if existing, lookupErr := s.LookupByProvider(ctx, provider, providerUserID); lookupErr == nil && existing != nil {
			if linked, uErr := s.userForAccount(ctx, existing.UserID); uErr == nil {
				if delErr := s.userService().DeleteAccount(ctx, user.ID); delErr != nil {
					s.Logger().Warn("failed to delete redundant social login account",
						zap.Uint("user_id", user.ID),
						zap.Error(delErr))
				}
				return &core.SocialAuthResult{User: linked, EmailVerified: linked.Verified}, nil
			}
		}

		// The link failed on its own: delete the account we just created via
		// the canonical account lifecycle so it does not linger unlinked.
		if delErr := s.userService().DeleteAccount(ctx, user.ID); delErr != nil {
			s.Logger().Warn("failed to delete unlinked social login account",
				zap.Uint("user_id", user.ID),
				zap.Error(delErr))
		}
		return nil, err
	}

	return &core.SocialAuthResult{User: user, Created: true, Linked: true, EmailVerified: user.Verified}, nil
}

func (s SocialAuthServiceDefault) LinkAccount(ctx context.Context, userID uint, provider, providerUserID, email string) error {
	ctx, span := core.TraceMethod(ctx, "SocialAuthServiceDefault.LinkAccount")
	defer span.End()

	existing, err := s.LookupByProvider(ctx, provider, providerUserID)
	if err != nil {
		return err
	}
	if existing != nil {
		if existing.UserID == userID {
			return nil
		}
		return core.NewAccountError(core.ErrKeySocialAlreadyLinked, nil)
	}

	acct := models.SocialAccount{
		UserID:         userID,
		Provider:       provider,
		ProviderUserID: providerUserID,
		Email:          email,
	}

	err = db.RetryableComponentTransaction(s, ctx, func(tx *gorm.DB) *gorm.DB {
		return tx.WithContext(ctx).Create(&acct)
	})
	if err != nil {
		if db.IsDuplicateKeyError(err) {
			return core.NewAccountError(core.ErrKeySocialAlreadyLinked, err)
		}
		return core.NewAccountError(core.ErrKeySocialAddFailed, err)
	}

	return nil
}

func (s SocialAuthServiceDefault) UnlinkAccount(ctx context.Context, userID uint, provider string) error {
	ctx, span := core.TraceMethod(ctx, "SocialAuthServiceDefault.UnlinkAccount")
	defer span.End()

	var rowsAffected int64
	err := db.RetryableComponentTransaction(s, ctx, func(tx *gorm.DB) *gorm.DB {
		// Hard delete: the UNIQUE(provider, provider_user_id) index does not
		// include deleted_at, so a soft-deleted row would keep the identity
		// claimed and block re-linking. Unscoped removes it for good.
		tx = tx.Unscoped().
			WithContext(ctx).
			Where("user_id = ? AND provider = ?", userID, provider).
			Delete(&models.SocialAccount{})
		rowsAffected = tx.RowsAffected
		return tx
	})
	if err != nil {
		return core.NewAccountError(core.ErrKeySocialRemoveFailed, err)
	}
	if rowsAffected == 0 {
		return core.NewAccountError(core.ErrKeySocialAccountNotFound, fmt.Errorf("social account not found for user %d provider %q", userID, provider))
	}
	return nil
}

func (s SocialAuthServiceDefault) ListAccounts(ctx context.Context, userID uint) ([]*models.SocialAccount, error) {
	ctx, span := core.TraceMethod(ctx, "SocialAuthServiceDefault.ListAccounts")
	defer span.End()

	var accounts []*models.SocialAccount
	err := s.DB().WithContext(ctx).
		Where("user_id = ?", userID).
		Order("provider ASC").
		Find(&accounts).Error
	if err != nil {
		return nil, core.NewAccountError(core.ErrKeyDatabaseOperationFailed, err)
	}
	return accounts, nil
}

// userForAccount loads the portal user linked to a social account.
func (s SocialAuthServiceDefault) userForAccount(ctx context.Context, userID uint) (*models.User, error) {
	exists, user, err := s.userService().AccountExists(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, core.NewAccountError(core.ErrKeySocialAccountNotFound, fmt.Errorf("linked user %d not found", userID))
	}
	return user, nil
}

// createAccount provisions a new portal user for social login. The account is
// created with a random, unguessable password (it is never
// password-authenticated). It is marked email-verified only when the provider
// response confirms the email (emailVerified); otherwise it is created
// unverified awaiting email confirmation.
func (s SocialAuthServiceDefault) createAccount(ctx context.Context, email string, emailVerified bool) (*models.User, error) {
	password, err := generateRandomPassword()
	if err != nil {
		return nil, core.NewAccountError(core.ErrKeySocialAccountCreationFailed, err)
	}

	user, err := s.userService().CreateAccount(ctx, email, password, false, core.WithBootstrapAdmin(false))
	if err != nil {
		if core.IsAccountError(err) {
			// Preserve typed account errors (e.g. ErrKeyEmailAlreadyExists).
			return nil, err
		}
		return nil, core.NewAccountError(core.ErrKeySocialAccountCreationFailed, err)
	}

	if emailVerified && !user.Verified {
		if err := s.userService().UpdateAccountInfo(ctx, user.ID, map[string]any{"verified": true}); err != nil {
			return nil, err
		}
		user.Verified = true
	}

	return user, nil
}

// generateRandomPassword returns a 32-byte cryptographically-strong random
// password used for social accounts that will never be password-authenticated.
func generateRandomPassword() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
