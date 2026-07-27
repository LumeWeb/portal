package service_tests

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/service"
	"go.lumeweb.com/queryutil"
	"go.lumeweb.com/queryutil/filter"
	"golang.org/x/crypto/bcrypt"
)

func TestUserService_EmailExists(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		userService := core.GetService[core.UserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, userService)

		// Create a test user
		email := "test@example.com"
		password := "password"
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		require.NoError(tb, err)

		user := &models.User{
			Email:        email,
			PasswordHash: string(hashedPassword),
		}
		err = ctx.DB().Create(user).Error
		require.NoError(tb, err)

		// Test email exists
		exists, retrievedUser, err := userService.EmailExists(context.Background(), email)
		require.NoError(tb, err)
		assert.True(tb, exists)
		assert.Equal(tb, user.ID, retrievedUser.ID)

		// Test email does not exist
		exists, _, err = userService.EmailExists(context.Background(), "nonexistent@example.com")
		require.NoError(tb, err)
		assert.False(tb, exists)

	}, coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService))
}

func TestUserService_AccountExists(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		userService := core.GetService[core.UserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, userService)

		// Create a test user
		email := "test@example.com"
		password := "password"
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		require.NoError(tb, err)

		user := &models.User{
			Email:        email,
			PasswordHash: string(hashedPassword),
		}
		err = ctx.DB().Create(user).Error
		require.NoError(tb, err)

		// Test account exists
		exists, retrievedUser, err := userService.AccountExists(context.Background(), user.ID)
		require.NoError(tb, err)
		assert.True(tb, exists)
		assert.Equal(tb, user.ID, retrievedUser.ID)

		// Test account does not exist
		exists, _, err = userService.AccountExists(context.Background(), 999)
		require.NoError(tb, err)
		assert.False(tb, exists)

	}, coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService))
}

func TestUserService_HashPassword(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		userService := core.GetService[core.UserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, userService)

		// Test hash password
		password := "password"
		hashedPassword, err := userService.HashPassword(password)
		require.NoError(tb, err)
		assert.NotEmpty(tb, hashedPassword)

		// Verify password
		err = bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
		assert.NoError(tb, err)

	}, coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService))
}

func TestUserService_CreateAccount(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		userService := core.GetService[core.UserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, userService)

		// Test create account
		email := "test@example.com"
		password := "password"
		user, err := userService.CreateAccount(context.Background(), email, password, false)
		require.NoError(tb, err)
		assert.NotEmpty(tb, user.PasswordHash)
		assert.Equal(tb, email, user.Email)

		// Verify account exists
		exists, _, err := userService.EmailExists(context.Background(), email)
		require.NoError(tb, err)
		assert.True(tb, exists)

		// Test create account with existing email
		_, err = userService.CreateAccount(context.Background(), email, password, false)
		assert.Error(tb, err)
		assert.Equal(tb, core.AsAccountError(err).Key, core.ErrKeyEmailAlreadyExists)

	}, coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService))
}

func TestUserService_UpdateAccountName(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		userService := core.GetService[core.UserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, userService)

		// Create a test user
		email := "test@example.com"
		password := "password"
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		require.NoError(tb, err)

		user := &models.User{
			Email:        email,
			PasswordHash: string(hashedPassword),
		}
		err = ctx.DB().Create(user).Error
		require.NoError(tb, err)

		// Test update account name
		firstName := "John"
		lastName := "Doe"
		err = userService.UpdateAccountName(context.Background(), user.ID, firstName, lastName)
		require.NoError(tb, err)

		// Verify account name
		_, updatedUser, err := userService.AccountExists(context.Background(), user.ID)
		require.NoError(tb, err)
		assert.Equal(tb, firstName, updatedUser.FirstName)
		assert.Equal(tb, lastName, updatedUser.LastName)

	}, coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService))
}

func TestUserService_UpdateAccountEmail(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		userService := core.GetService[core.UserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, userService)

		// Create a test user
		email := "test@example.com"
		password := "password"
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		require.NoError(tb, err)

		user := &models.User{
			Email:        email,
			PasswordHash: string(hashedPassword),
		}
		err = ctx.DB().Create(user).Error
		require.NoError(tb, err)

		// Test update account email
		newEmail := "new@example.com"
		err = userService.UpdateAccountEmail(context.Background(), user.ID, newEmail, password)
		require.NoError(tb, err)

		// Verify account email
		_, updatedUser, err := userService.AccountExists(context.Background(), user.ID)
		require.NoError(tb, err)
		assert.Equal(tb, newEmail, updatedUser.Email)

		// Test update account email with existing email
		_, err = userService.CreateAccount(context.Background(), "existing@example.com", password, false)
		require.NoError(tb, err)

		err = userService.UpdateAccountEmail(context.Background(), user.ID, "existing@example.com", password)
		assert.Error(tb, err)
		assert.Equal(tb, core.AsAccountError(err).Key, core.ErrKeyEmailAlreadyExists)

		// Test update account email with invalid password
		err = userService.UpdateAccountEmail(context.Background(), user.ID, "invalid@example.com", "wrongpassword")
		assert.Error(tb, err)
		assert.Equal(tb, core.AsAccountError(err).Key, core.ErrKeyInvalidLogin)

	}, coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService),
		coreTesting.WithServiceFactory(core.AUTH_SERVICE, service.NewAuthService))
}

func TestUserService_UpdateAccountPassword(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		userService := core.GetService[core.UserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, userService)

		// Create a test user
		email := "test@example.com"
		password := "password"
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		require.NoError(tb, err)

		user := &models.User{
			Email:        email,
			PasswordHash: string(hashedPassword),
		}
		err = ctx.DB().Create(user).Error
		require.NoError(tb, err)

		// Test update account password
		newPassword := "newpassword"
		err = userService.UpdateAccountPassword(context.Background(), user.ID, password, newPassword)
		require.NoError(tb, err)

		// Verify account password
		_, updatedUser, err := userService.AccountExists(context.Background(), user.ID)
		require.NoError(tb, err)
		err = bcrypt.CompareHashAndPassword([]byte(updatedUser.PasswordHash), []byte(newPassword))
		assert.NoError(tb, err)

		// Test update account password with invalid password
		err = userService.UpdateAccountPassword(context.Background(), user.ID, "wrongpassword", "newpassword")
		assert.Error(tb, err)
		assert.Equal(tb, core.AsAccountError(err).Key, core.ErrKeyInvalidPassword)

	}, coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService),
		coreTesting.WithServiceFactory(core.AUTH_SERVICE, service.NewAuthService))
}

func TestUserService_DeleteAccount(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		userService := core.GetService[core.UserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, userService)

		// Create a test user
		email := "test@example.com"
		password := "password"
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		require.NoError(tb, err)

		user := &models.User{
			Email:        email,
			PasswordHash: string(hashedPassword),
		}
		err = ctx.DB().Create(user).Error
		require.NoError(tb, err)

		// Test delete account
		err = userService.DeleteAccount(context.Background(), user.ID)
		require.NoError(tb, err)

		// Verify account does not exist
		exists, _, err := userService.AccountExists(context.Background(), user.ID)
		require.NoError(tb, err)
		assert.False(tb, exists)

	}, coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService))
}

func TestUserService_RequestAccountDeletion(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		userService := core.GetService[core.UserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, userService)

		// Create a test user
		email := "test@example.com"
		password := "password"
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		require.NoError(tb, err)

		user := &models.User{
			Email:        email,
			PasswordHash: string(hashedPassword),
		}
		err = ctx.DB().Create(user).Error
		require.NoError(tb, err)

		// Test request account deletion
		err = userService.RequestAccountDeletion(context.Background(), user.ID, "127.0.0.1")
		require.NoError(tb, err)

		// Verify account deletion requested
		pending, err := userService.IsAccountPendingDeletion(context.Background(), user.ID)
		require.NoError(tb, err)
		assert.True(tb, pending)

		// Test request account deletion again
		err = userService.RequestAccountDeletion(context.Background(), user.ID, "127.0.0.1")
		assert.Error(tb, err)
		assert.Equal(tb, core.AsAccountError(err).Key, core.ErrKeyAccountDeletionRequestAlreadyExists)

	}, coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService))
}

func TestUserService_IsAccountPendingDeletion(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		userService := core.GetService[core.UserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, userService)

		// Create a test user
		email := "test@example.com"
		password := "password"
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		require.NoError(tb, err)

		user := &models.User{
			Email:        email,
			PasswordHash: string(hashedPassword),
		}
		err = ctx.DB().Create(user).Error
		require.NoError(tb, err)

		// Test is account pending deletion
		pending, err := userService.IsAccountPendingDeletion(context.Background(), user.ID)
		require.NoError(tb, err)
		assert.False(tb, pending)

		// Request account deletion
		err = userService.RequestAccountDeletion(context.Background(), user.ID, "127.0.0.1")
		require.NoError(tb, err)

		// Test is account pending deletion
		pending, err = userService.IsAccountPendingDeletion(context.Background(), user.ID)
		require.NoError(tb, err)
		assert.True(tb, pending)

	}, coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService))
}

func TestUserService_AddKeyIdentity_NilMetadataDefaultsToEmptyJSON(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		userService := core.GetService[core.UserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, userService)

		// Register a test handler
		coreTesting.WithKeyIdentityHandler("ethereum", &testKeyIdentityHandler{})(ctx)

		// Create a test user
		hashedCredential, err := bcrypt.GenerateFromPassword([]byte(testKeyIdentitySecret()), bcrypt.DefaultCost)
		require.NoError(tb, err)
		user := &models.User{
			Email:        "keyidentity@example.com",
			PasswordHash: string(hashedCredential),
		}
		err = ctx.DB().Create(user).Error
		require.NoError(tb, err)

		// Add key identity with nil metadata
		err = userService.AddKeyIdentity(context.Background(), user.ID, "ethereum", "0x1234567890abcdef1234567890abcdef12345678", nil)
		require.NoError(tb, err)

		// Verify the stored metadata is {} not NULL
		var ki models.KeyIdentity
		err = ctx.DB().Where("user_id = ? AND type = ?", user.ID, "ethereum").First(&ki).Error
		require.NoError(tb, err)
		assert.NotNil(tb, ki.Metadata)
		assert.JSONEq(tb, `{}`, string(ki.Metadata))
	}, coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService))
}

func TestUserService_AddKeyIdentity_HandlerValidatesMetadata(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		userService := core.GetService[core.UserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, userService)

		// Register a handler that rejects invalid metadata
		coreTesting.WithKeyIdentityHandler("ethereum", &testKeyIdentityHandler{})(ctx)

		hashedCredential, err := bcrypt.GenerateFromPassword([]byte(testKeyIdentitySecret()), bcrypt.DefaultCost)
		require.NoError(tb, err)
		user := &models.User{
			Email:        "keyidentity2@example.com",
			PasswordHash: string(hashedCredential),
		}
		err = ctx.DB().Create(user).Error
		require.NoError(tb, err)

		// Add key identity with valid metadata
		err = userService.AddKeyIdentity(context.Background(), user.ID, "ethereum", "0xabcdef1234567890abcdef1234567890abcdef12", json.RawMessage(`{"chain_id":"eip155:1"}`))
		require.NoError(tb, err)

		// Verify stored metadata
		var ki models.KeyIdentity
		err = ctx.DB().Where("user_id = ? AND type = ?", user.ID, "ethereum").First(&ki).Error
		require.NoError(tb, err)
		assert.JSONEq(tb, `{"chain_id":"eip155:1"}`, string(ki.Metadata))
	}, coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService))
}

func TestUserService_AddKeyIdentity_DuplicateKey(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		userService := core.GetService[core.UserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, userService)

		coreTesting.WithKeyIdentityHandler("ethereum", &testKeyIdentityHandler{})(ctx)

		hashedCredential, err := bcrypt.GenerateFromPassword([]byte(testKeyIdentitySecret()), bcrypt.DefaultCost)
		require.NoError(tb, err)
		user := &models.User{
			Email:        "keyidentity3@example.com",
			PasswordHash: string(hashedCredential),
		}
		err = ctx.DB().Create(user).Error
		require.NoError(tb, err)

		key := "0x9999999999999999999999999999999999999999"
		err = userService.AddKeyIdentity(context.Background(), user.ID, "ethereum", key, nil)
		require.NoError(tb, err)

		// Adding same key again should fail with ErrKeyKeyIdentityExists
		err = userService.AddKeyIdentity(context.Background(), user.ID, "ethereum", key, nil)
		assert.Error(tb, err)
		coreErr, ok := err.(*core.Error)
		require.True(tb, ok, "expected *core.Error")
		assert.Equal(tb, core.ErrKeyKeyIdentityExists, coreErr.Key)
	}, coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService))
}

func TestUserService_AddKeyIdentity_NoHandlerReturnsAccountError(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		userService := core.GetService[core.UserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, userService)

		// Create a test user — no handler registered for "ethereum"
		hashedCredential, err := bcrypt.GenerateFromPassword([]byte(testKeyIdentitySecret()), bcrypt.DefaultCost)
		require.NoError(tb, err)
		user := &models.User{
			Email:        "nohandler-add@example.com",
			PasswordHash: string(hashedCredential),
		}
		err = ctx.DB().Create(user).Error
		require.NoError(tb, err)

		err = userService.AddKeyIdentity(context.Background(), user.ID, "ethereum", "0x1234567890abcdef1234567890abcdef12345678", nil)
		assert.Error(tb, err)
		coreErr, ok := err.(*core.Error)
		require.True(tb, ok, "expected *core.Error")
		assert.Equal(tb, core.ErrKeyAddKeyIdentityFailed, coreErr.Key)
	}, coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService))
}

func TestUserService_KeyIdentityExists_NoHandlerReturnsAccountError(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		userService := core.GetService[core.UserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, userService)

		// No handler registered for "ethereum"
		_, _, err := userService.KeyIdentityExists(context.Background(), "ethereum", "0x1234567890abcdef1234567890abcdef12345678")
		assert.Error(tb, err)
		coreErr, ok := err.(*core.Error)
		require.True(tb, ok, "expected *core.Error")
		assert.Equal(tb, core.ErrKeyInvalidLogin, coreErr.Key)
	}, coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService))
}

func TestUserService_RemoveKeyIdentity_Success(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		userService := core.GetService[core.UserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, userService)

		coreTesting.WithKeyIdentityHandler("ethereum", &testKeyIdentityHandler{})(ctx)

		hashedCredential, err := bcrypt.GenerateFromPassword([]byte(testKeyIdentitySecret()), bcrypt.DefaultCost)
		require.NoError(tb, err)
		user := &models.User{
			Email:        "remove-key@example.com",
			PasswordHash: string(hashedCredential),
		}
		err = ctx.DB().Create(user).Error
		require.NoError(tb, err)

		key := "0xaaaa1111bbbb2222cccc3333dddd4444eeee5555"
		err = userService.AddKeyIdentity(context.Background(), user.ID, "ethereum", key, nil)
		require.NoError(tb, err)

		// Remove the key
		err = userService.RemoveKeyIdentity(context.Background(), user.ID, "ethereum", key)
		require.NoError(tb, err)

		// Verify it's gone
		var count int64
		ctx.DB().Model(&models.KeyIdentity{}).Where("user_id = ? AND type = ? AND key = ?", user.ID, "ethereum", key).Count(&count)
		assert.Equal(tb, int64(0), count)
	}, coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService))
}

func TestUserService_RemoveKeyIdentity_NotFound(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		userService := core.GetService[core.UserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, userService)

		coreTesting.WithKeyIdentityHandler("ethereum", &testKeyIdentityHandler{})(ctx)

		hashedCredential, err := bcrypt.GenerateFromPassword([]byte(testKeyIdentitySecret()), bcrypt.DefaultCost)
		require.NoError(tb, err)
		user := &models.User{
			Email:        "remove-notfound@example.com",
			PasswordHash: string(hashedCredential),
		}
		err = ctx.DB().Create(user).Error
		require.NoError(tb, err)

		// Remove a key that doesn't exist
		err = userService.RemoveKeyIdentity(context.Background(), user.ID, "ethereum", "0x9999888877776666555544443333222211110000")
		assert.Error(tb, err)
		coreErr, ok := err.(*core.Error)
		require.True(tb, ok, "expected *core.Error")
		assert.Equal(tb, core.ErrKeyKeyIdentityNotFound, coreErr.Key)
	}, coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService))
}

func TestUserService_RemoveKeyIdentity_DoesNotBelongToUser(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		userService := core.GetService[core.UserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, userService)

		coreTesting.WithKeyIdentityHandler("ethereum", &testKeyIdentityHandler{})(ctx)

		// Create two users
		hashedCred, err := bcrypt.GenerateFromPassword([]byte(testKeyIdentitySecret()), bcrypt.DefaultCost)
		require.NoError(tb, err)
		user1 := &models.User{Email: "rm-user1@example.com", PasswordHash: string(hashedCred)}
		user2 := &models.User{Email: "rm-user2@example.com", PasswordHash: string(hashedCred)}
		err = ctx.DB().Create(user1).Error
		require.NoError(tb, err)
		err = ctx.DB().Create(user2).Error
		require.NoError(tb, err)

		key := "0xbbbb1111cccc2222dddd3333eeee4444ffff5555"
		err = userService.AddKeyIdentity(context.Background(), user1.ID, "ethereum", key, nil)
		require.NoError(tb, err)

		// User 2 tries to remove user 1's key — should fail (not found for user 2)
		err = userService.RemoveKeyIdentity(context.Background(), user2.ID, "ethereum", key)
		assert.Error(tb, err)
		coreErr, ok := err.(*core.Error)
		require.True(tb, ok, "expected *core.Error")
		assert.Equal(tb, core.ErrKeyKeyIdentityNotFound, coreErr.Key)

		// Verify the key still exists for user 1
		var count int64
		ctx.DB().Model(&models.KeyIdentity{}).Where("user_id = ? AND key = ?", user1.ID, key).Count(&count)
		assert.Equal(tb, int64(1), count)
	}, coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService))
}

func TestUserService_RemoveKeyIdentity_NoHandlerReturnsAccountError(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		userService := core.GetService[core.UserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, userService)

		hashedCred, err := bcrypt.GenerateFromPassword([]byte(testKeyIdentitySecret()), bcrypt.DefaultCost)
		require.NoError(tb, err)
		user := &models.User{Email: "rm-nohandler@example.com", PasswordHash: string(hashedCred)}
		err = ctx.DB().Create(user).Error
		require.NoError(tb, err)

		err = userService.RemoveKeyIdentity(context.Background(), user.ID, "ethereum", "0x1234567890abcdef1234567890abcdef12345678")
		assert.Error(tb, err)
		coreErr, ok := err.(*core.Error)
		require.True(tb, ok, "expected *core.Error")
		assert.Equal(tb, core.ErrKeyKeyIdentityNotFound, coreErr.Key)
	}, coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService))
}

func TestUserService_ListKeyIdentities_Success(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		userService := core.GetService[core.UserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, userService)

		coreTesting.WithKeyIdentityHandler("ethereum", &testKeyIdentityHandler{})(ctx)

		hashedCred, err := bcrypt.GenerateFromPassword([]byte(testKeyIdentitySecret()), bcrypt.DefaultCost)
		require.NoError(tb, err)
		user := &models.User{Email: "list-keys@example.com", PasswordHash: string(hashedCred)}
		err = ctx.DB().Create(user).Error
		require.NoError(tb, err)

		// Add multiple keys
		err = userService.AddKeyIdentity(context.Background(), user.ID, "ethereum", "0x1111111111111111111111111111111111111111", nil)
		require.NoError(tb, err)
		err = userService.AddKeyIdentity(context.Background(), user.ID, "ethereum", "0x2222222222222222222222222222222222222222", nil)
		require.NoError(tb, err)

		// List identities
		identities, _, err := userService.ListKeyIdentities(context.Background(), user.ID, nil, nil, queryutil.Pagination{})
		require.NoError(tb, err)
		assert.Len(tb, identities, 2)
	}, coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService))
}

func TestUserService_ListKeyIdentities_Empty(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		userService := core.GetService[core.UserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, userService)

		hashedCred, err := bcrypt.GenerateFromPassword([]byte(testKeyIdentitySecret()), bcrypt.DefaultCost)
		require.NoError(tb, err)
		user := &models.User{Email: "list-empty@example.com", PasswordHash: string(hashedCred)}
		err = ctx.DB().Create(user).Error
		require.NoError(tb, err)

		identities, _, err := userService.ListKeyIdentities(context.Background(), user.ID, nil, nil, queryutil.Pagination{})
		require.NoError(tb, err)
		assert.Empty(tb, identities)
	}, coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService))
}

// lowercasingKeyIdentityHandler normalizes keys to lowercase to test
// normalization round-trip (add with mixed-case, remove/list with different casing).
type lowercasingKeyIdentityHandler struct{}

func (h *lowercasingKeyIdentityHandler) NormalizeKey(key string) (string, error) {
	return strings.ToLower(key), nil
}
func (h *lowercasingKeyIdentityHandler) ValidateMetadata(metadata json.RawMessage) (json.RawMessage, error) {
	return metadata, nil
}
func (h *lowercasingKeyIdentityHandler) IssueChallenge(ctx core.Context, key string, metadata json.RawMessage) ([]byte, error) {
	return []byte("challenge"), nil
}
func (h *lowercasingKeyIdentityHandler) VerifyProof(ctx core.Context, key string, metadata json.RawMessage, proof []byte) error {
	return nil
}

// TestUserService_KeyIdentity_NormalizationRoundTrip verifies that a key added
// with mixed-case can be found, listed, and removed with different casing.
// This tests the normalization contract: storage uses normalized form, all
// lookups go through NormalizeKey.
func TestUserService_KeyIdentity_NormalizationRoundTrip(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		userService := core.GetService[core.UserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, userService)

		coreTesting.WithKeyIdentityHandler("ethereum", &lowercasingKeyIdentityHandler{})(ctx)

		hashedCred, err := bcrypt.GenerateFromPassword([]byte(testKeyIdentitySecret()), bcrypt.DefaultCost)
		require.NoError(tb, err)
		user := &models.User{Email: "norm-rt@example.com", PasswordHash: string(hashedCred)}
		err = ctx.DB().Create(user).Error
		require.NoError(tb, err)

		mixedCaseKey := "0xAbCdEf0123456789AbCdEf0123456789AbCdEf01"
		expectedNormalized := "0xabcdef0123456789abcdef0123456789abcdef01"

		// Add with mixed case
		err = userService.AddKeyIdentity(context.Background(), user.ID, "ethereum", mixedCaseKey, nil)
		require.NoError(tb, err)

		// KeyIdentityExists with mixed case should find it (service normalizes)
		exists, _, err := userService.KeyIdentityExists(context.Background(), "ethereum", mixedCaseKey)
		require.NoError(tb, err)
		assert.True(tb, exists, "KeyIdentityExists should find key with mixed-case input")

		// KeyIdentityExists with different casing should also find it
		exists, _, err = userService.KeyIdentityExists(context.Background(), "ethereum", "0xABCDEF0123456789ABCDEF0123456789ABCDEF01")
		require.NoError(tb, err)
		assert.True(tb, exists, "KeyIdentityExists should find key with different casing")

		// List should show the normalized form
		identities, _, err := userService.ListKeyIdentities(context.Background(), user.ID, nil, nil, queryutil.Pagination{})
		require.NoError(tb, err)
		require.Len(tb, identities, 1)
		assert.Equal(tb, expectedNormalized, identities[0].Key, "stored key should be normalized")

		// Remove with different casing should succeed
		err = userService.RemoveKeyIdentity(context.Background(), user.ID, "ethereum", "0xABCDEF0123456789ABCDEF0123456789ABCDEF01")
		require.NoError(tb, err, "RemoveKeyIdentity should succeed with different casing")

		// Verify it's gone
		identities, _, err = userService.ListKeyIdentities(context.Background(), user.ID, nil, nil, queryutil.Pagination{})
		require.NoError(tb, err)
		assert.Empty(tb, identities)
	}, coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService))
}

// TestUserService_AddKeyIdentity_ConcurrentDuplicate verifies that concurrent
// AddKeyIdentity calls with the same key don't both succeed — the DB unique
// constraint on (type, key) enforces atomicity, and the loser gets
// ErrKeyKeyIdentityExists.
func TestUserService_AddKeyIdentity_ConcurrentDuplicate(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		userService := core.GetService[core.UserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, userService)

		coreTesting.WithKeyIdentityHandler("ethereum", &testKeyIdentityHandler{})(ctx)

		hashedCred, err := bcrypt.GenerateFromPassword([]byte(testKeyIdentitySecret()), bcrypt.DefaultCost)
		require.NoError(tb, err)
		user := &models.User{Email: "concurrent@example.com", PasswordHash: string(hashedCred)}
		err = ctx.DB().Create(user).Error
		require.NoError(tb, err)

		key := "0xDeadBeefDeadBeefDeadBeefDeadBeefDeadBeef"

		var wg sync.WaitGroup
		errs := make([]error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				errs[idx] = userService.AddKeyIdentity(context.Background(), user.ID, "ethereum", key, nil)
			}(i)
		}
		wg.Wait()

		// Exactly one should succeed, one should fail with ErrKeyKeyIdentityExists
		var successes, conflicts int
		for _, e := range errs {
			if e == nil {
				successes++
			} else if coreErr, ok := e.(*core.Error); ok && coreErr.Key == core.ErrKeyKeyIdentityExists {
				conflicts++
			} else {
				tb.Fatalf("unexpected error: %v", e)
			}
		}
		assert.Equal(tb, 1, successes, "exactly one goroutine should succeed")
		assert.Equal(tb, 1, conflicts, "exactly one goroutine should get conflict error")
	}, coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService))
}

// TestUserService_ListKeyIdentities_Pagination verifies that ListKeyIdentities
// respects pagination and total count with multiple identities.
func TestUserService_ListKeyIdentities_Pagination(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		userService := core.GetService[core.UserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, userService)

		coreTesting.WithKeyIdentityHandler("ethereum", &testKeyIdentityHandler{})(ctx)

		hashedCred, err := bcrypt.GenerateFromPassword([]byte(testKeyIdentitySecret()), bcrypt.DefaultCost)
		require.NoError(tb, err)
		user := &models.User{Email: "paginate@example.com", PasswordHash: string(hashedCred)}
		err = ctx.DB().Create(user).Error
		require.NoError(tb, err)

		// Add 5 keys
		for i := 0; i < 5; i++ {
			key := fmt.Sprintf("0x%040d", i)
			err = userService.AddKeyIdentity(context.Background(), user.ID, "ethereum", key, nil)
			require.NoError(tb, err)
		}

		// Page 1: start 0, pageSize 2
		pg, err := filter.NewPagination(0, 2)
		require.NoError(tb, err)
		identities, total, err := userService.ListKeyIdentities(context.Background(), user.ID, nil, nil, pg)
		require.NoError(tb, err)
		assert.Len(tb, identities, 2, "page 1 should return 2 items")
		assert.Equal(tb, int64(5), total, "total should be 5")

		// Page 2: start 2, pageSize 2
		pg, err = filter.NewPagination(2, 2)
		require.NoError(tb, err)
		identities, total, err = userService.ListKeyIdentities(context.Background(), user.ID, nil, nil, pg)
		require.NoError(tb, err)
		assert.Len(tb, identities, 2, "page 2 should return 2 items")
		assert.Equal(tb, int64(5), total, "total should still be 5")

		// Page 3: start 4, pageSize 2 (last page, 1 item)
		pg, err = filter.NewPagination(4, 2)
		require.NoError(tb, err)
		identities, total, err = userService.ListKeyIdentities(context.Background(), user.ID, nil, nil, pg)
		require.NoError(tb, err)
		assert.Len(tb, identities, 1, "page 3 should return 1 item")
		assert.Equal(tb, int64(5), total, "total should still be 5")

		// Filter by type
		typeFilter := filter.NewLogicalFilter("type", filter.OpEq, "ethereum")
		pg, _ = filter.NewPagination(0, 10)
		identities, total, err = userService.ListKeyIdentities(context.Background(), user.ID,
			[]queryutil.CrudFilter{typeFilter},
			nil,
			pg)
		require.NoError(tb, err)
		assert.Len(tb, identities, 5, "filter by type=ethereum should return all 5")
		assert.Equal(tb, int64(5), total)
	}, coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService))
}
