package service_tests

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/service"
	"golang.org/x/crypto/bcrypt"
)

func TestAuthService_Integration(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		authService := core.GetService[core.AuthService](ctx, core.AUTH_SERVICE)
		require.NotNil(tb, authService)

		// Register test key identity handler for "ed25519" type
		coreTesting.WithKeyIdentityHandler("ed25519", &testKeyIdentityHandler{})(ctx)

		userService := core.GetService[core.UserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, userService)

		// 1. Create a test user with password
		password := "securepassword"
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		require.NoError(tb, err)

		user := &models.User{
			Email:        "testuser@example.com",
			PasswordHash: string(hashedPassword),
		}
		err = ctx.DB().Create(user).Error
		require.NoError(tb, err)

		// 2. Create a test user with key identity
		pubKeyUser := &models.User{
			Email:        "pubkeyuser@example.com",
			PasswordHash: string(hashedPassword), // same password for simplicity
		}
		err = ctx.DB().Create(pubKeyUser).Error
		require.NoError(tb, err)

		// Generate a real ed25519 key pair
		pubKey, _, err := ed25519.GenerateKey(nil)
		require.NoError(tb, err)

		// Encode the public key to base64 for storage
		pubKeyBase64 := base64.StdEncoding.EncodeToString(pubKey)

		// Store the key identity in the database
		keyIdentity := &models.KeyIdentity{
			UserID: pubKeyUser.ID,
			Type:   "ed25519",
			Key:    pubKeyBase64,
		}
		err = ctx.DB().Create(keyIdentity).Error
		require.NoError(tb, err)

		// 3. Test LoginPassword
		token, loggedInUser, err := authService.LoginPassword(context.Background(), "testuser@example.com", password, "127.0.0.1", false)
		assert.NoError(tb, err)
		assert.NotEmpty(tb, token)
		assert.Equal(tb, user.ID, loggedInUser.ID)

		// 4. Test LoginKeyIdentity
		pubkeyToken, err := authService.LoginKeyIdentity(context.Background(), "ed25519", pubKeyBase64, []byte("proof"), "127.0.0.1", false)
		assert.NoError(tb, err)
		assert.NotEmpty(tb, pubkeyToken)

		// 5. Test LoginID
		idToken, err := authService.LoginID(context.Background(), user.ID, "127.0.0.1", false)
		assert.NoError(tb, err)
		assert.NotEmpty(tb, idToken)

		// 6. Test invalid LoginPassword
		_, _, err = authService.LoginPassword(context.Background(), "testuser@example.com", "wrongpassword", "127.0.0.1", false)
		assert.Error(tb, err)

		// 7. Test invalid LoginKeyIdentity
		_, err = authService.LoginKeyIdentity(context.Background(), "ed25519", "invalidkey", []byte("proof"), "127.0.0.1", false)
		assert.Error(tb, err)

		// 8. Test invalid LoginID
		_, err = authService.LoginID(context.Background(), 999999, "127.0.0.1", false)
		assert.Error(tb, err)

		// 9. Verify valid password by user object
		valid := authService.ValidLoginByUserObj(context.Background(), user, password)
		assert.True(tb, valid)

		// 10. Verify valid password by email
		valid, fetchedUser, err := authService.ValidLoginByEmail(context.Background(), "testuser@example.com", password)
		assert.NoError(tb, err)
		assert.True(tb, valid)
		assert.Equal(tb, user.ID, fetchedUser.ID)

		// 11. Verify valid password by user ID
		userIDStr := strconv.Itoa(int(user.ID))
		fmt.Println("User ID:", userIDStr)
		valid, fetchedUser, err = authService.ValidLoginByUserID(context.Background(), user.ID, password)
		assert.NoError(tb, err)
		assert.True(tb, valid)
		assert.Equal(tb, user.ID, fetchedUser.ID)

		// 12. Test account pending deletion
		err = userService.RequestAccountDeletion(context.Background(), user.ID, "127.0.0.1")
		require.NoError(tb, err)

		_, _, err = authService.LoginPassword(context.Background(), "testuser@example.com", password, "127.0.0.1", false)
		assert.Error(tb, err)

	},
		coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService),
		coreTesting.WithServiceFactory(core.AUTH_SERVICE, service.NewAuthService),
	)
}
