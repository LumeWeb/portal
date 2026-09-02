package service_tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/service"
)

func socialAuthTestOpts() []coreTesting.TestContextBuilderOption {
	return []coreTesting.TestContextBuilderOption{
		coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService),
		coreTesting.WithServiceFactory(core.SOCIAL_AUTH_SERVICE, service.NewSocialAuthService),
	}
}

func getSocialAuthService(tb coreTesting.TB, ctx coreTesting.TestContext) core.SocialAuthService {
	svc := core.GetService[core.SocialAuthService](ctx, core.SOCIAL_AUTH_SERVICE)
	require.NotNil(tb, svc)

	return svc
}

func TestSocialAuth_LookupByProvider(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		svc := getSocialAuthService(tb, ctx)

		// Not found returns nil, nil
		acct, err := svc.LookupByProvider(context.Background(), "google", "uid-1")
		require.NoError(tb, err)
		assert.Nil(tb, acct)

		// Insert a link directly and look it up
		user := &models.User{Email: "linked@example.com"}
		require.NoError(tb, ctx.DB().Create(user).Error)
		link := &models.SocialAccount{UserID: user.ID, Provider: "google", ProviderUserID: "uid-1", Email: user.Email}
		require.NoError(tb, ctx.DB().Create(link).Error)

		acct, err = svc.LookupByProvider(context.Background(), "google", "uid-1")
		require.NoError(tb, err)
		require.NotNil(tb, acct)
		assert.Equal(tb, user.ID, acct.UserID)
		assert.Equal(tb, "google", acct.Provider)
		assert.Equal(tb, "uid-1", acct.ProviderUserID)
	}, socialAuthTestOpts()...)
}

func TestSocialAuth_LoginOrLink_ExistingLink(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		svc := getSocialAuthService(tb, ctx)

		user := &models.User{Email: "existing@example.com", Verified: true}
		require.NoError(tb, ctx.DB().Create(user).Error)
		link := &models.SocialAccount{UserID: user.ID, Provider: "github", ProviderUserID: "gh-1"}
		require.NoError(tb, ctx.DB().Create(link).Error)

		// Existing link wins even with a different email provided
		res, err := svc.LoginOrLink(context.Background(), "github", "gh-1", "other@example.com", false)
		require.NoError(tb, err)
		require.NotNil(tb, res)
		assert.Equal(tb, user.ID, res.User.ID)
		assert.False(tb, res.Created)
		assert.False(tb, res.Linked)
		assert.True(tb, res.EmailVerified)
	}, socialAuthTestOpts()...)
}

func TestSocialAuth_LoginOrLink_CreatesAccount(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		svc := getSocialAuthService(tb, ctx)

		res, err := svc.LoginOrLink(context.Background(), "google", "new-uid", "new@example.com", true)
		require.NoError(tb, err)
		require.NotNil(tb, res)
		assert.True(tb, res.Created)
		assert.True(tb, res.Linked)
		assert.True(tb, res.EmailVerified)
		assert.Equal(tb, "new@example.com", res.User.Email)

		// The IdP assertion auto-verifies the new account
		var stored models.User
		require.NoError(tb, ctx.DB().First(&stored, res.User.ID).Error)
		assert.True(tb, stored.Verified)

		// Link is persisted
		acct, err := svc.LookupByProvider(context.Background(), "google", "new-uid")
		require.NoError(tb, err)
		require.NotNil(tb, acct)
		assert.Equal(tb, res.User.ID, acct.UserID)

		// A second call returns the same user without creating or linking again
		res2, err := svc.LoginOrLink(context.Background(), "google", "new-uid", "new@example.com", true)
		require.NoError(tb, err)
		require.NotNil(tb, res2)
		assert.Equal(tb, res.User.ID, res2.User.ID)
		assert.False(tb, res2.Created)
		assert.False(tb, res2.Linked)
	}, socialAuthTestOpts()...)
}

func TestSocialAuth_LoginOrLink_EmailConflict(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		svc := getSocialAuthService(tb, ctx)

		existing := &models.User{Email: "taken@example.com"}
		require.NoError(tb, ctx.DB().Create(existing).Error)

		// Email already belongs to a portal user -> typed error, no auto-link
		res, err := svc.LoginOrLink(context.Background(), "google", "conflict-uid", "taken@example.com", true)
		require.Error(tb, err)
		assert.True(tb, core.IsAccountError(err))
		assert.Nil(tb, res)

		acct, err := svc.LookupByProvider(context.Background(), "google", "conflict-uid")
		require.NoError(tb, err)
		assert.Nil(tb, acct)

		var count int64
		require.NoError(tb, ctx.DB().Model(&models.User{}).Count(&count).Error)
		assert.Equal(tb, int64(1), count)
	}, socialAuthTestOpts()...)
}

func TestSocialAuth_LinkAccount(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		svc := getSocialAuthService(tb, ctx)

		user1 := &models.User{Email: "link1@example.com"}
		user2 := &models.User{Email: "link2@example.com"}
		require.NoError(tb, ctx.DB().Create(user1).Error)
		require.NoError(tb, ctx.DB().Create(user2).Error)

		// Link to user1
		require.NoError(tb, svc.LinkAccount(context.Background(), user1.ID, "x", "id-1", "link1@example.com"))

		// Idempotent for the same user
		require.NoError(tb, svc.LinkAccount(context.Background(), user1.ID, "x", "id-1", "link1@example.com"))

		// Same identity linked to a different user -> conflict
		err := svc.LinkAccount(context.Background(), user2.ID, "x", "id-1", "link2@example.com")
		require.Error(tb, err)
		assert.True(tb, core.IsAccountError(err))

		accounts, err := svc.ListAccounts(context.Background(), user1.ID)
		require.NoError(tb, err)
		assert.Len(tb, accounts, 1)
	}, socialAuthTestOpts()...)
}

func TestSocialAuth_UnlinkAccount(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		svc := getSocialAuthService(tb, ctx)

		user := &models.User{Email: "unlink@example.com"}
		require.NoError(tb, ctx.DB().Create(user).Error)
		require.NoError(tb, svc.LinkAccount(context.Background(), user.ID, "google", "g-1", "unlink@example.com"))

		require.NoError(tb, svc.UnlinkAccount(context.Background(), user.ID, "google"))

		acct, err := svc.LookupByProvider(context.Background(), "google", "g-1")
		require.NoError(tb, err)
		assert.Nil(tb, acct)

		// Unlinking a non-existent link -> not found error
		err = svc.UnlinkAccount(context.Background(), user.ID, "google")
		require.Error(tb, err)
	}, socialAuthTestOpts()...)
}

func TestSocialAuth_ListAccounts(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		svc := getSocialAuthService(tb, ctx)

		user := &models.User{Email: "multi@example.com"}
		require.NoError(tb, ctx.DB().Create(user).Error)
		require.NoError(tb, svc.LinkAccount(context.Background(), user.ID, "google", "g-1", "multi@example.com"))
		require.NoError(tb, svc.LinkAccount(context.Background(), user.ID, "github", "gh-1", "multi@example.com"))

		accounts, err := svc.ListAccounts(context.Background(), user.ID)
		require.NoError(tb, err)
		assert.Len(tb, accounts, 2)

		other := &models.User{Email: "other@example.com"}
		require.NoError(tb, ctx.DB().Create(other).Error)
		otherAccounts, err := svc.ListAccounts(context.Background(), other.ID)
		require.NoError(tb, err)
		assert.Empty(tb, otherAccounts)
	}, socialAuthTestOpts()...)
}

func TestSocialAuth_LoginOrLink_UnverifiedEmail(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		svc := getSocialAuthService(tb, ctx)

		// Pre-seed a user so the created account is not the "first user"
		// (CreateAccount auto-verifies the first account).
		require.NoError(tb, ctx.DB().Create(&models.User{Email: "seed@example.com"}).Error)

		// Provider did not confirm the email: the account is created but NOT
		// marked verified.
		res, err := svc.LoginOrLink(context.Background(), "google", "unverified-uid", "unverified@example.com", false)
		require.NoError(tb, err)
		require.NotNil(tb, res)
		assert.True(tb, res.Created)
		assert.False(tb, res.EmailVerified)

		var stored models.User
		require.NoError(tb, ctx.DB().First(&stored, res.User.ID).Error)
		assert.False(tb, stored.Verified)
	}, socialAuthTestOpts()...)
}

func TestSocialAuth_RelinkAfterUnlink(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		svc := getSocialAuthService(tb, ctx)

		user := &models.User{Email: "relink@example.com"}
		require.NoError(tb, ctx.DB().Create(user).Error)
		require.NoError(tb, svc.LinkAccount(context.Background(), user.ID, "google", "g-1", "relink@example.com"))

		// Unlink hard-deletes the row so the identity's unique key is freed.
		require.NoError(tb, svc.UnlinkAccount(context.Background(), user.ID, "google"))

		// Re-linking the same identity must succeed.
		require.NoError(tb, svc.LinkAccount(context.Background(), user.ID, "google", "g-1", "relink@example.com"))

		acct, err := svc.LookupByProvider(context.Background(), "google", "g-1")
		require.NoError(tb, err)
		require.NotNil(tb, acct)
		assert.Equal(tb, user.ID, acct.UserID)
	}, socialAuthTestOpts()...)
}

func TestSocialAuth_LoginOrLink_FirstUserNotAdmin(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		svc := getSocialAuthService(tb, ctx)
		access := coreTesting.GetMockAccessService(ctx)

		// Fresh empty DB: this is the first account, but a public social login
		// must never self-appoint as the portal administrator.
		res, err := svc.LoginOrLink(context.Background(), "google", "admin-uid", "social-admin@example.com", true)
		require.NoError(tb, err)
		require.NotNil(tb, res)
		assert.True(tb, res.Created)

		access.AssertNotCalled(tb, "AssignRoleToUser", mock.Anything, res.User.ID, core.ACCESS_ADMIN_ROLE)
	}, socialAuthTestOpts()...)
}

func TestSocialAuth_LoginOrLink_DiscardsUnlinkedAccount(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		svc := getSocialAuthService(tb, ctx)

		// A soft-deleted row keeps (provider, provider_user_id) claimed so the
		// link INSERT inside LoginOrLink collides with the unique index, forcing
		// the "created but could not be linked" path.
		other := &models.User{Email: "other@example.com"}
		require.NoError(tb, ctx.DB().Create(other).Error)
		stale := &models.SocialAccount{UserID: other.ID, Provider: "google", ProviderUserID: "g-fail"}
		require.NoError(tb, ctx.DB().Create(stale).Error)
		require.NoError(tb, ctx.DB().Delete(stale).Error) // soft delete keeps the unique key

		_, err := svc.LoginOrLink(context.Background(), "google", "g-fail", "discarded@example.com", true)
		require.Error(tb, err)

		// The account created for the failed link must be deleted (no active
		// orphan remaining).
		var count int64
		require.NoError(tb, ctx.DB().Model(&models.User{}).Where("email = ?", "discarded@example.com").Count(&count).Error)
		assert.Zero(tb, count)
	}, socialAuthTestOpts()...)
}
