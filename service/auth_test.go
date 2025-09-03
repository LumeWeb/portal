package service

import (
	"testing"

	"github.com/stretchr/testify/mock"
	coreMocks "go.lumeweb.com/portal/core/testing/mocks"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/db/models"
	"golang.org/x/crypto/bcrypt"
)

func TestAuthService_LoginPassword(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		authService := core.GetService[core.AuthService](ctx, core.AUTH_SERVICE)
		userService := core.GetService[*coreMocks.MockUserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, authService)

		// Create test user with properly hashed password
		password := "password"
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		require.NoError(tb, err)

		user := &models.User{
			Email:        "test@example.com",
			PasswordHash: string(hashedPassword),
		}
		err = ctx.DB().Create(user).Error
		require.NoError(tb, err)

		// Setup mock expectations
		userService.EXPECT().IsAccountPendingDeletion(user.ID).Return(false, nil)
		userService.EXPECT().UpdateAccountInfo(user.ID, mock.Anything).Return(nil)

		// Test valid login
		token, loggedInUser, err := authService.LoginPassword("test@example.com", "password", "127.0.0.1", false)
		assert.NoError(tb, err)
		assert.NotEmpty(tb, token)
		assert.Equal(tb, user.ID, loggedInUser.ID)

		// Test invalid password
		_, _, err = authService.LoginPassword("test@example.com", "wrongpassword", "127.0.0.1", false)
		assert.Error(tb, err)

		// Test non-existent user
		_, _, err = authService.LoginPassword("nonexistent@example.com", "password", "127.0.0.1", false)
		assert.Error(tb, err)
	}, coreTesting.WithServiceFactory(core.AUTH_SERVICE, NewAuthService))
}

func TestAuthService_LoginOTP(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		authService := core.GetService[core.AuthService](ctx, core.AUTH_SERVICE)
		otpService := core.GetService[*coreMocks.MockOTPService](ctx, core.OTP_SERVICE)
		//userService := core.GetService[*coreMocks.MockUserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, authService)
		require.NotNil(tb, otpService)

		// Create test user with OTP enabled
		user := &models.User{
			Email:        "otpuser@example.com",
			PasswordHash: "$2a$10$X8z5JZJfN5JZJfN5JZJfN.5JZJfN5JZJfN5JZJfN5JZJfN5JZJfN5J",
			OTPEnabled:   true,
		}
		err := ctx.DB().Create(user).Error
		require.NoError(tb, err)

		// Setup mock expectations for OTP verification
		validCode := "123456"
		invalidCode := "000000"

		otpService.EXPECT().OTPVerify(uint(0x1), validCode).Return(true, nil)
		otpService.EXPECT().OTPVerify(uint(0x1), invalidCode).Return(false, nil)

		// Test valid OTP login
		token, err := authService.LoginOTP(user.ID, validCode, false)
		assert.NoError(tb, err)
		assert.NotEmpty(tb, token)

		// Test invalid OTP code
		_, err = authService.LoginOTP(user.ID, invalidCode, false)
		assert.Error(tb, err)
	}, coreTesting.WithServiceFactory(core.AUTH_SERVICE, NewAuthService))
}

func TestAuthService_ValidLoginByUserObj(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		authService := core.GetService[core.AuthService](ctx, core.AUTH_SERVICE)
		require.NotNil(tb, authService)

		// Create test user with properly hashed password
		password := "password"
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		require.NoError(tb, err)

		user := &models.User{
			Email:        "validuser@example.com",
			PasswordHash: string(hashedPassword),
		}
		err = ctx.DB().Create(user).Error
		require.NoError(tb, err)

		// Test valid password
		valid := authService.ValidLoginByUserObj(user, "password")
		assert.True(tb, valid)

		// Test invalid password
		valid = authService.ValidLoginByUserObj(user, "wrongpassword")
		assert.False(tb, valid)
	}, coreTesting.WithServiceFactory(core.AUTH_SERVICE, NewAuthService))
}

func TestAuthService_ValidLoginByUserID(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		authService := core.GetService[core.AuthService](ctx, core.AUTH_SERVICE)
		require.NotNil(tb, authService)

		// Create test user with properly hashed password
		password := "password"
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		require.NoError(tb, err)

		user := &models.User{
			Email:        "validid@example.com",
			PasswordHash: string(hashedPassword),
		}
		err = ctx.DB().Create(user).Error
		require.NoError(tb, err)

		// Test valid login
		valid, fetchedUser, err := authService.ValidLoginByUserID(user.ID, "password")
		assert.NoError(tb, err)
		assert.True(tb, valid)
		assert.Equal(tb, user.ID, fetchedUser.ID)

		// Test invalid password
		valid, _, err = authService.ValidLoginByUserID(user.ID, "wrongpassword")
		assert.NoError(tb, err)
		assert.False(tb, valid)

		// Test non-existent user
		_, _, err = authService.ValidLoginByUserID(999999, "password")
		assert.Error(tb, err)
	}, coreTesting.WithServiceFactory(core.AUTH_SERVICE, NewAuthService))
}

func TestAuthService_LoginPubkey(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		authService := core.GetService[core.AuthService](ctx, core.AUTH_SERVICE)
		userService := core.GetService[*coreMocks.MockUserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, authService)

		// Create test user with public key
		// Create properly hashed password for test user
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.DefaultCost)
		require.NoError(tb, err)

		user := &models.User{
			Email:        "pubkeyuser@example.com",
			PasswordHash: string(hashedPassword),
		}
		err = ctx.DB().Create(user).Error
		require.NoError(tb, err)

		pubkey := &models.PublicKey{
			UserID: user.ID,
			Key:    "test-public-key",
		}
		err = ctx.DB().Create(pubkey).Error
		require.NoError(tb, err)

		// Setup mock expectations
		userService.EXPECT().IsAccountPendingDeletion(user.ID).Return(false, nil)
		userService.EXPECT().UpdateAccountInfo(user.ID, mock.Anything).Return(nil)

		// Test valid pubkey login
		token, err := authService.LoginPubkey("test-public-key", "127.0.0.1", false)
		assert.NoError(tb, err)
		assert.NotEmpty(tb, token)

		// Test invalid pubkey
		_, err = authService.LoginPubkey("invalid-key", "127.0.0.1", false)
		assert.Error(tb, err)
	}, coreTesting.WithServiceFactory(core.AUTH_SERVICE, NewAuthService))
}

func TestAuthService_LoginID(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		authService := core.GetService[core.AuthService](ctx, core.AUTH_SERVICE)
		userService := core.GetService[*coreMocks.MockUserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, authService)

		// Create test user with properly hashed password
		password := "password"
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		require.NoError(tb, err)

		user := &models.User{
			Email:        "idlogin@example.com",
			PasswordHash: string(hashedPassword),
		}
		err = ctx.DB().Create(user).Error
		require.NoError(tb, err)

		// Setup mock expectations
		userService.EXPECT().IsAccountPendingDeletion(user.ID).Return(false, nil)
		userService.EXPECT().UpdateAccountInfo(user.ID, mock.Anything).Return(nil)

		// Test valid ID login
		token, err := authService.LoginID(user.ID, "127.0.0.1", false)
		assert.NoError(tb, err)
		assert.NotEmpty(tb, token)

		// Test invalid ID
		_, err = authService.LoginID(999999, "127.0.0.1", false)
		assert.Error(tb, err)
	}, coreTesting.WithServiceFactory(core.AUTH_SERVICE, NewAuthService))
}

func TestAuthService_ValidLoginByEmail(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		authService := core.GetService[core.AuthService](ctx, core.AUTH_SERVICE)
		require.NotNil(tb, authService)

		// Create test user with properly hashed password
		password := "password"
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		require.NoError(tb, err)

		user := &models.User{
			Email:        "emailvalid@example.com",
			PasswordHash: string(hashedPassword),
		}
		err = ctx.DB().Create(user).Error
		require.NoError(tb, err)

		// Test valid login
		valid, fetchedUser, err := authService.ValidLoginByEmail("emailvalid@example.com", "password")
		assert.NoError(tb, err)
		assert.True(tb, valid)
		assert.Equal(tb, user.ID, fetchedUser.ID)

		// Test invalid password
		valid, _, err = authService.ValidLoginByEmail("emailvalid@example.com", "wrongpassword")
		assert.NoError(tb, err)
		assert.False(tb, valid)

		// Test non-existent email
		_, _, err = authService.ValidLoginByEmail("nonexistent@example.com", "password")
		assert.Error(tb, err)
	}, coreTesting.WithServiceFactory(core.AUTH_SERVICE, NewAuthService))
}
