package service_tests

import (
	"context"
	"errors"
	"github.com/stretchr/testify/mock"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/service"

	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func TestPasswordResetService_SendPasswordReset(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		passwordResetService := core.GetService[core.PasswordResetService](ctx, core.PASSWORD_RESET_SERVICE)
		require.NotNil(tb, passwordResetService)

		passwordResetService.(*service.PasswordResetServiceDefault).SetSubdomain("sub.portal.com")

		userService := coreTesting.GetMockUserService(ctx)
		require.NotNil(tb, userService)

		mailerService := coreTesting.GetMockMailerService(ctx)
		require.NotNil(tb, mailerService)

		// Create a test user
		password := "securepassword"
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		require.NoError(tb, err)

		user := &models.User{
			Email:        "testuser@example.com",
			PasswordHash: string(hashedPassword),
			FirstName:    "Test",
		}

		// Setup mock expectations
		mailerService.EXPECT().TemplateSend(
			core.MAILER_TPL_PASSWORD_RESET,
			mock.MatchedBy(func(data interface{}) bool {
				_, ok := data.(map[string]interface{})
				return ok
			}),
			mock.MatchedBy(func(data interface{}) bool {
				_, ok := data.(map[string]interface{})
				return ok
			}),
			user.Email,
		).Return(nil).Once()

		err = ctx.DB().Create(user).Error
		require.NoError(tb, err)

		// Send password reset email
		err = passwordResetService.SendPasswordReset(context.Background(), user)
		require.NoError(tb, err)

		// Verify mock expectations
		mock.AssertExpectationsForObjects(tb, mailerService)

		// Verify that a password reset token was created in the database
		var reset models.PasswordReset
		result := ctx.DB().Where("user_id = ?", user.ID).First(&reset)
		require.NoError(tb, result.Error)
		assert.NotEmpty(tb, reset.Token)
		assert.WithinDuration(tb, time.Now().Add(time.Hour), reset.ExpiresAt, time.Minute)

		// Verify that the email was sent
		// This part depends on how you mock or verify the mailer service
		// For example, if you have a mock mailer, you can check if Send was called with the correct arguments

	}, coreTesting.WithServiceFactory(core.PASSWORD_RESET_SERVICE, service.NewPasswordResetService))
}

func TestPasswordResetService_SendPasswordReset_MissingSubdomain(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		passwordResetService := core.GetService[core.PasswordResetService](ctx, core.PASSWORD_RESET_SERVICE).(*service.PasswordResetServiceDefault)
		passwordResetService.SetSubdomain("")

		// Create test user
		password := "password"
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		require.NoError(tb, err)

		user := &models.User{
			Email:        "test@example.com",
			PasswordHash: string(hashedPassword),
		}
		err = ctx.DB().Create(user).Error
		require.NoError(tb, err)

		err = passwordResetService.SendPasswordReset(context.Background(), user)
		assert.Error(t, err)
		assert.Equal(t, "password reset service subdomain not configured", err.Error())
	}, coreTesting.WithServiceFactory(core.PASSWORD_RESET_SERVICE, service.NewPasswordResetService))
}

func TestPasswordResetService_ResetPassword(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		passwordResetService := core.GetService[core.PasswordResetService](ctx, core.PASSWORD_RESET_SERVICE)
		require.NotNil(tb, passwordResetService)

		userService := coreTesting.GetMockUserService(ctx)
		require.NotNil(tb, userService)

		// 1. Create a test user
		password := "securepassword"
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		require.NoError(tb, err)

		user := &models.User{
			Email:        "testuser@example.com",
			PasswordHash: string(hashedPassword),
			FirstName:    "Test",
		}
		err = ctx.DB().Create(user).Error
		require.NoError(tb, err)

		// Setup mock expectations
		newPassword := "newsecurepassword"
		newHashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
		require.NoError(tb, err)

		user2 := &models.User{
			Email:        "testuser@example.com",
			PasswordHash: string(newHashedPassword),
			FirstName:    "Test",
		}

		userService.EXPECT().EmailExists(mock.Anything, user.Email).Return(true, user2, nil).Times(3)
		userService.EXPECT().EmailExists(mock.Anything, "invalid@example.com").Return(false, nil, nil).Once()
		userService.EXPECT().AccountExists(mock.Anything, user.ID).Return(true, user2, nil).Once()
		userService.EXPECT().HashPassword(newPassword).Return(string(newHashedPassword), nil).Once()
		userService.EXPECT().UpdateAccountInfo(mock.Anything, user.ID, map[string]interface{}{
			"password_hash": string(newHashedPassword),
		}).Return(nil).Once()

		// 2. Create a password reset token
		token := core.GenerateSecurityToken()
		reset := models.PasswordReset{
			UserID:    user.ID,
			Token:     token,
			ExpiresAt: time.Now().Add(time.Hour),
		}
		err = ctx.DB().Create(&reset).Error
		require.NoError(tb, err)

		// 3. Reset the password
		err = passwordResetService.ResetPassword(context.Background(), user.Email, token, newPassword)
		require.NoError(tb, err)

		// 4. Verify that the password was updated in the database
		_, updatedUser, err := userService.AccountExists(context.Background(), user.ID)
		require.NoError(tb, err)
		err = bcrypt.CompareHashAndPassword([]byte(updatedUser.PasswordHash), []byte(newPassword))
		require.NoError(tb, err)

		// 5. Verify that the password reset token was deleted
		var resetCheck models.PasswordReset
		result := ctx.DB().Where("user_id = ? AND token = ?", user.ID, token).First(&resetCheck)
		assert.Error(tb, result.Error)
		assert.True(tb, errors.Is(result.Error, gorm.ErrRecordNotFound))

		// Test with invalid email
		err = passwordResetService.ResetPassword(context.Background(), "invalid@example.com", token, newPassword)
		assert.Error(tb, err)
		assert.Equal(tb, core.AsAccountError(err).Key, core.ErrKeyUserNotFound)

		// Test with invalid token
		err = passwordResetService.ResetPassword(context.Background(), user.Email, "invalidtoken", newPassword)
		assert.Error(tb, err)
		assert.Equal(tb, core.AsAccountError(err).Key, core.ErrKeyUserNotFound)

		// Test with expired token
		expiredToken := core.GenerateSecurityToken()
		expiredReset := models.PasswordReset{
			UserID:    user.ID,
			Token:     expiredToken,
			ExpiresAt: time.Now().Add(-time.Hour),
		}
		err = ctx.DB().Create(&expiredReset).Error
		require.NoError(tb, err)

		err = passwordResetService.ResetPassword(context.Background(), user.Email, expiredToken, newPassword)
		assert.Error(tb, err)
		assert.Equal(tb, core.AsAccountError(err).Key, core.ErrKeySecurityTokenExpired)

	},
		coreTesting.WithServiceFactory(core.PASSWORD_RESET_SERVICE, service.NewPasswordResetService),
	)
}

func TestPasswordResetServiceDefault_ID(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		passwordResetService := core.GetService[core.PasswordResetService](ctx, core.PASSWORD_RESET_SERVICE)
		require.NotNil(tb, passwordResetService)

		assert.Equal(tb, core.PASSWORD_RESET_SERVICE, passwordResetService.ID())
	},
		coreTesting.WithServiceFactory(core.PASSWORD_RESET_SERVICE, service.NewPasswordResetService),
	)
}
