package service_tests

import (
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/service"
	"golang.org/x/crypto/bcrypt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		exists, retrievedUser, err := userService.EmailExists(nil, email)
		require.NoError(tb, err)
		assert.True(tb, exists)
		assert.Equal(tb, user.ID, retrievedUser.ID)

		// Test email does not exist
		exists, _, err = userService.EmailExists(nil, "nonexistent@example.com")
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
		exists, retrievedUser, err := userService.AccountExists(nil, user.ID)
		require.NoError(tb, err)
		assert.True(tb, exists)
		assert.Equal(tb, user.ID, retrievedUser.ID)

		// Test account does not exist
		exists, _, err = userService.AccountExists(nil, 999)
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
		user, err := userService.CreateAccount(nil, email, password, false)
		require.NoError(tb, err)
		assert.NotEmpty(tb, user.PasswordHash)
		assert.Equal(tb, email, user.Email)

		// Verify account exists
		exists, _, err := userService.EmailExists(nil, email)
		require.NoError(tb, err)
		assert.True(tb, exists)

		// Test create account with existing email
		_, err = userService.CreateAccount(nil, email, password, false)
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
		err = userService.UpdateAccountName(user.ID, nil, firstName, lastName)
		require.NoError(tb, err)

		// Verify account name
		_, updatedUser, err := userService.AccountExists(nil, user.ID)
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
		err = userService.UpdateAccountEmail(nil, user.ID, newEmail, password)
		require.NoError(tb, err)

		// Verify account email
		_, updatedUser, err := userService.AccountExists(nil, user.ID)
		require.NoError(tb, err)
		assert.Equal(tb, newEmail, updatedUser.Email)

		// Test update account email with existing email
		_, err = userService.CreateAccount(nil, "existing@example.com", password, false)
		require.NoError(tb, err)

		err = userService.UpdateAccountEmail(nil, user.ID, "existing@example.com", password)
		assert.Error(tb, err)
		assert.Equal(tb, core.AsAccountError(err).Key, core.ErrKeyEmailAlreadyExists)

		// Test update account email with invalid password
		err = userService.UpdateAccountEmail(nil, user.ID, "invalid@example.com", "wrongpassword")
		assert.Error(tb, err)
		assert.Equal(tb, core.AsAccountError(err).Key, core.ErrKeyInvalidLogin)

	}, coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService))
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
		err = userService.UpdateAccountPassword(nil, user.ID, password, newPassword)
		require.NoError(tb, err)

		// Verify account password
		_, updatedUser, err := userService.AccountExists(nil, user.ID)
		require.NoError(tb, err)
		err = bcrypt.CompareHashAndPassword([]byte(updatedUser.PasswordHash), []byte(newPassword))
		assert.NoError(tb, err)

		// Test update account password with invalid password
		err = userService.UpdateAccountPassword(nil, user.ID, "wrongpassword", "newpassword")
		assert.Error(tb, err)
		assert.Equal(tb, core.AsAccountError(err).Key, core.ErrKeyInvalidPassword)

	}, coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService))
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
		err = userService.DeleteAccount(nil, user.ID)
		require.NoError(tb, err)

		// Verify account does not exist
		exists, _, err := userService.AccountExists(nil, user.ID)
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
		err = userService.RequestAccountDeletion(nil, user.ID, "127.0.0.1")
		require.NoError(tb, err)

		// Verify account deletion requested
		pending, err := userService.IsAccountPendingDeletion(nil, user.ID)
		require.NoError(tb, err)
		assert.True(tb, pending)

		// Test request account deletion again
		err = userService.RequestAccountDeletion(nil, user.ID, "127.0.0.1")
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
		pending, err := userService.IsAccountPendingDeletion(nil, user.ID)
		require.NoError(tb, err)
		assert.False(tb, pending)

		// Request account deletion
		err = userService.RequestAccountDeletion(nil, user.ID, "127.0.0.1")
		require.NoError(tb, err)

		// Test is account pending deletion
		pending, err = userService.IsAccountPendingDeletion(nil, user.ID)
		require.NoError(tb, err)
		assert.True(tb, pending)

	}, coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService))
}
