package service_tests

import (
	mock "github.com/stretchr/testify/mock"
	"go.lumeweb.com/portal/service"

	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	coreMocks "go.lumeweb.com/portal/core/testing/mocks"
	"go.lumeweb.com/portal/db/models"
	"golang.org/x/crypto/bcrypt"
)

func TestOTPService_OTPGeneration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		otpService := core.GetService[core.OTPService](ctx, core.OTP_SERVICE)
		require.NotNil(tb, otpService)

		userService := core.GetService[*coreMocks.MockUserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, userService)

		// Create test user
		password := "password"
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		require.NoError(tb, err)

		user := &models.User{
			Email:        "test@example.com",
			PasswordHash: string(hashedPassword),
		}
		require.NoError(tb, ctx.DB().Create(user).Error)

		// Setup mock expectations for OTP generation
		userService.EXPECT().AccountExists(user.ID).Return(true, user, nil).Once()
		userService.EXPECT().UpdateAccountInfo(user.ID, mock.Anything).Return(nil).Once()

		// Generate OTP secret
		secret, err := otpService.OTPGenerate(nil, user.ID)
		require.NoError(tb, err)
		assert.NotEmpty(tb, secret)
		assert.Len(tb, secret, 32) // TOTP secrets are typically 32 chars
	}, coreTesting.WithServiceFactory(core.OTP_SERVICE, service.NewOTPService))
}

func TestOTPService_OTPVerification(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		otpService := core.GetService[core.OTPService](ctx, core.OTP_SERVICE)
		require.NotNil(tb, otpService)

		// Create test user
		password := "password"
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		require.NoError(tb, err)

		user := &models.User{
			Email:        "test@example.com",
			PasswordHash: string(hashedPassword),
		}
		require.NoError(tb, ctx.DB().Create(user).Error)

		userService := core.GetService[*coreMocks.MockUserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, userService)

		// Setup mock expectations for OTP verification
		userService.EXPECT().AccountExists(user.ID).Return(true, user, nil).Times(3)
		userService.EXPECT().UpdateAccountInfo(user.ID, mock.Anything).Return(nil).Once()

		secret, err := otpService.OTPGenerate(nil, user.ID)
		require.NoError(tb, err)

		user.OTPSecret = secret

		code, err := totp.GenerateCode(secret, time.Now())
		require.NoError(tb, err)

		// Valid code
		valid, err := otpService.OTPVerify(nil, user.ID, code)
		assert.NoError(tb, err)
		assert.True(tb, valid)

		// Invalid code
		valid, err = otpService.OTPVerify(nil, user.ID, "000000")
		assert.NoError(tb, err)
		assert.False(tb, valid)
	}, coreTesting.WithServiceFactory(core.OTP_SERVICE, service.NewOTPService))
}

func TestOTPService_OTPLifecycle(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		otpService := core.GetService[core.OTPService](ctx, core.OTP_SERVICE)
		userService := core.GetService[*coreMocks.MockUserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, otpService)

		// Create test user
		password := "password"
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		require.NoError(tb, err)

		user := &models.User{
			Email:        "test@example.com",
			PasswordHash: string(hashedPassword),
		}
		require.NoError(tb, ctx.DB().Create(user).Error)

		// Setup mock expectations for OTP lifecycle
		userService.EXPECT().AccountExists(user.ID).Return(true, user, nil)
		userService.EXPECT().UpdateAccountInfo(user.ID, mock.Anything).Return(nil)

		// Generate OTP secret
		secret, err := otpService.OTPGenerate(nil, user.ID)
		require.NoError(tb, err)

		user.OTPSecret = secret

		code, err := totp.GenerateCode(secret, time.Now())
		require.NoError(tb, err)

		// Enable OTP
		require.NoError(tb, otpService.OTPEnable(nil, user.ID, code))

		// Disable OTP
		require.NoError(tb, otpService.OTPDisable(nil, user.ID))

	}, coreTesting.WithServiceFactory(core.OTP_SERVICE, service.NewOTPService))
}

func TestOTPService_ErrorCases(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		otpService := core.GetService[core.OTPService](ctx, core.OTP_SERVICE)
		require.NotNil(tb, otpService)

		userService := core.GetService[*coreMocks.MockUserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, userService)

		// Setup mock expectations for invalid user cases
		invalidUserID := uint(999999)
		userService.EXPECT().AccountExists(invalidUserID).Return(false, nil, nil).Times(4)

		// Invalid user
		_, err := otpService.OTPGenerate(nil, invalidUserID)
		assert.Error(tb, err)

		_, err = otpService.OTPVerify(nil, invalidUserID, "123456")
		assert.Error(t, err)

		err = otpService.OTPEnable(nil, invalidUserID, "123456")
		assert.Error(t, err)

		err = otpService.OTPDisable(nil, invalidUserID)
		assert.Error(t, err)
	}, coreTesting.WithServiceFactory(core.OTP_SERVICE, service.NewOTPService))
}
