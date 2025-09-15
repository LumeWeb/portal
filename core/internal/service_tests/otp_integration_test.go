package service_tests

import (
	"testing"

	"time"

	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/service"
	"golang.org/x/crypto/bcrypt"
)

func TestOTPService_Integration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		otpService := core.GetService[core.OTPService](ctx, core.OTP_SERVICE)
		require.NotNil(tb, otpService)

		userService := core.GetService[core.UserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, userService)

		// 1. Create a test user
		password := "password"
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		require.NoError(tb, err)

		user := &models.User{
			Email:        "test@example.com",
			PasswordHash: string(hashedPassword),
		}
		err = ctx.DB().Create(user).Error
		require.NoError(tb, err)

		// 2. Generate OTP secret
		secret, err := otpService.OTPGenerate(user.ID)
		require.NoError(tb, err)
		assert.NotEmpty(tb, secret)

		// 3. Verify OTP secret was stored
		updatedUser, _, err := userService.AccountExists(user.ID)
		require.NoError(tb, err)
		assert.True(tb, updatedUser)

		// 4. Generate OTP code
		code, err := totp.GenerateCode(secret, time.Now())
		require.NoError(tb, err)

		// 5. Verify OTP code
		valid, err := otpService.OTPVerify(user.ID, code)
		assert.NoError(tb, err)
		assert.True(tb, valid)

		// 6. Enable OTP
		err = otpService.OTPEnable(user.ID, code)
		require.NoError(tb, err)

		// 7. Verify OTP is enabled
		enabledUser, _, err := userService.AccountExists(user.ID)
		require.NoError(tb, err)
		assert.True(tb, enabledUser)

		// 8. Disable OTP
		err = otpService.OTPDisable(user.ID)
		require.NoError(tb, err)

		// 9. Verify OTP is disabled
		disabledUser, _, err := userService.AccountExists(user.ID)
		require.NoError(tb, err)
		assert.True(tb, disabledUser)

		// 10. Test invalid OTP code
		valid, err = otpService.OTPVerify(user.ID, "000000")
		assert.NoError(tb, err)
		assert.False(tb, valid)

		// 11. Test invalid user
		_, err = otpService.OTPGenerate(999999)
		assert.Error(tb, err)

		_, err = otpService.OTPVerify(999999, "123456")
		assert.Error(tb, err)

		err = otpService.OTPEnable(999999, "123456")
		assert.Error(tb, err)

		err = otpService.OTPDisable(999999)
		assert.Error(tb, err)

	},
		coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService),
		coreTesting.WithServiceFactory(core.AUTH_SERVICE, service.NewAuthService),
		coreTesting.WithServiceFactory(core.OTP_SERVICE, service.NewOTPService),
		coreTesting.WithServiceFactory(core.CONTENT_SCANNER_SERVICE, service.NewContentScannerService),
	)
}
