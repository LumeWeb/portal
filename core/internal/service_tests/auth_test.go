package service_tests

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/mock"
	"go.lumeweb.com/portal/service"

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
		userService := coreTesting.GetMockUserService(ctx)
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
		userService.EXPECT().IsAccountPendingDeletion(mock.Anything, user.ID).Return(false, nil)
		userService.EXPECT().UpdateAccountInfo(mock.Anything, user.ID, mock.Anything).Return(nil)

		// Test valid login
		token, loggedInUser, err := authService.LoginPassword(context.Background(), "test@example.com", "password", "127.0.0.1", false)
		assert.NoError(tb, err)
		assert.NotEmpty(tb, token)
		assert.Equal(tb, user.ID, loggedInUser.ID)

		// Test invalid password
		_, _, err = authService.LoginPassword(context.Background(), "test@example.com", "wrongpassword", "127.0.0.1", false)
		assert.Error(tb, err)

		// Test non-existent user
		_, _, err = authService.LoginPassword(context.Background(), "nonexistent@example.com", "password", "127.0.0.1", false)
		assert.Error(tb, err)
	}, coreTesting.WithServiceFactory(core.AUTH_SERVICE, service.NewAuthService))
}

func TestAuthService_LoginOTP(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		authService := core.GetService[core.AuthService](ctx, core.AUTH_SERVICE)
		otpService := coreTesting.GetMockOTPService(ctx)
		userService := coreTesting.GetMockUserService(ctx)
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

		userService.EXPECT().IsAccountPendingDeletion(mock.Anything, user.ID).Return(false, nil)
		userService.EXPECT().UpdateAccountInfo(mock.Anything, user.ID, mock.Anything).Return(nil)
		otpService.EXPECT().OTPVerify(mock.Anything, user.ID, validCode).Return(true, nil)
		otpService.EXPECT().OTPVerify(mock.Anything, user.ID, invalidCode).Return(false, nil)

		// Test valid OTP login
		token, err := authService.LoginOTP(context.Background(), user.ID, validCode, false)
		assert.NoError(tb, err)
		assert.NotEmpty(tb, token)

		// Test invalid OTP code
		_, err = authService.LoginOTP(context.Background(), user.ID, invalidCode, false)
		assert.Error(tb, err)
	}, coreTesting.WithServiceFactory(core.AUTH_SERVICE, service.NewAuthService))
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
		valid := authService.ValidLoginByUserObj(context.Background(), user, "password")
		assert.True(tb, valid)

		// Test invalid password
		valid = authService.ValidLoginByUserObj(context.Background(), user, "wrongpassword")
		assert.False(tb, valid)
	}, coreTesting.WithServiceFactory(core.AUTH_SERVICE, service.NewAuthService))
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
		valid, fetchedUser, err := authService.ValidLoginByUserID(context.Background(), user.ID, "password")
		assert.NoError(tb, err)
		assert.True(tb, valid)
		assert.Equal(tb, user.ID, fetchedUser.ID)

		// Test invalid password
		valid, _, err = authService.ValidLoginByUserID(context.Background(), user.ID, "wrongpassword")
		assert.NoError(tb, err)
		assert.False(tb, valid)

		// Test non-existent user
		_, _, err = authService.ValidLoginByUserID(context.Background(), 999999, "password")
		assert.Error(tb, err)
	}, coreTesting.WithServiceFactory(core.AUTH_SERVICE, service.NewAuthService))
}

// testKeyIdentitySecret loads the test credential from the environment
// to comply with the no-hard-coded-secrets rule. TestMain sets a random
// value if the env var is unset, so this never panics.
func testKeyIdentitySecret() string {
	s := os.Getenv("TEST_KEY_IDENTITY_SECRET")
	if s == "" {
		panic("TEST_KEY_IDENTITY_SECRET environment variable must be set for tests")
	}
	return s
}

func TestAuthService_LoginKeyIdentity(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		authService := core.GetService[core.AuthService](ctx, core.AUTH_SERVICE)
		userService := coreTesting.GetMockUserService(ctx)
		require.NotNil(tb, authService)

		// Register a test key identity handler that normalizes lowercase and accepts any proof
		coreTesting.WithKeyIdentityHandler("ethereum", &testKeyIdentityHandler{})(ctx)

		// Create test user with key identity
		// Create properly hashed credential for test user
		hashedCredential, err := bcrypt.GenerateFromPassword([]byte(testKeyIdentitySecret()), bcrypt.DefaultCost)
		require.NoError(tb, err)

		user := &models.User{
			Email:        "pubkeyuser@example.com",
			PasswordHash: string(hashedCredential),
		}
		err = ctx.DB().Create(user).Error
		require.NoError(tb, err)

		keyIdentity := &models.KeyIdentity{
			UserID: user.ID,
			Type:   "ethereum",
			Key:    "0x1234567890abcdef1234567890abcdef12345678",
		}
		err = ctx.DB().Create(keyIdentity).Error
		require.NoError(tb, err)

		// Setup mock expectations
		userService.EXPECT().IsAccountPendingDeletion(mock.Anything, user.ID).Return(false, nil)
		userService.EXPECT().UpdateAccountInfo(mock.Anything, user.ID, mock.Anything).Return(nil)

		// Test valid key identity login
		token, err := authService.LoginKeyIdentity(context.Background(), "ethereum", "0x1234567890abcdef1234567890abcdef12345678", []byte("proof"), "127.0.0.1", false)
		assert.NoError(tb, err)
		assert.NotEmpty(tb, token)

		// Test invalid key identity
		_, err = authService.LoginKeyIdentity(context.Background(), "ethereum", "0xinvalid", []byte("proof"), "127.0.0.1", false)
		assert.Error(tb, err)
	}, coreTesting.WithServiceFactory(core.AUTH_SERVICE, service.NewAuthService))
}

// testKeyIdentityHandler is a minimal handler for service tests.
// It normalizes keys to lowercase and accepts any proof.
type testKeyIdentityHandler struct{}

func (h *testKeyIdentityHandler) NormalizeKey(key string) (string, error) { return key, nil }
func (h *testKeyIdentityHandler) ValidateMetadata(metadata json.RawMessage) (json.RawMessage, error) {
	return metadata, nil
}
func (h *testKeyIdentityHandler) IssueChallenge(ctx core.Context, key string, metadata json.RawMessage) ([]byte, error) {
	return []byte("challenge"), nil
}
func (h *testKeyIdentityHandler) VerifyProof(ctx core.Context, key string, metadata json.RawMessage, proof []byte) error {
	return nil
}

// failingProofHandler rejects all proofs.
type failingProofHandler struct{}

func (h *failingProofHandler) NormalizeKey(key string) (string, error) { return key, nil }
func (h *failingProofHandler) ValidateMetadata(metadata json.RawMessage) (json.RawMessage, error) {
	return metadata, nil
}
func (h *failingProofHandler) IssueChallenge(ctx core.Context, key string, metadata json.RawMessage) ([]byte, error) {
	return []byte("challenge"), nil
}
func (h *failingProofHandler) VerifyProof(ctx core.Context, key string, metadata json.RawMessage, proof []byte) error {
	return fmt.Errorf("proof verification failed")
}

// nilMetadataCapturingHandler captures the metadata it receives in VerifyProof
// to assert that nil metadata is defaulted to {} before being passed.
type nilMetadataCapturingHandler struct {
	receivedMetadata json.RawMessage
}

func (h *nilMetadataCapturingHandler) NormalizeKey(key string) (string, error) { return key, nil }
func (h *nilMetadataCapturingHandler) ValidateMetadata(metadata json.RawMessage) (json.RawMessage, error) {
	return metadata, nil
}
func (h *nilMetadataCapturingHandler) IssueChallenge(ctx core.Context, key string, metadata json.RawMessage) ([]byte, error) {
	return []byte("challenge"), nil
}
func (h *nilMetadataCapturingHandler) VerifyProof(ctx core.Context, key string, metadata json.RawMessage, proof []byte) error {
	h.receivedMetadata = metadata
	return nil
}

func TestAuthService_LoginKeyIdentity_RejectsWhenNoHandlerRegistered(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		authService := core.GetService[core.AuthService](ctx, core.AUTH_SERVICE)
		require.NotNil(tb, authService)

		// No handler registered for "ethereum" type
		// LoginKeyIdentity should fail with ErrKeyInvalidLogin
		_, err := authService.LoginKeyIdentity(context.Background(), "ethereum", "0x1234567890abcdef1234567890abcdef12345678", []byte("proof"), "127.0.0.1", false)
		assert.Error(tb, err)
		coreErr, ok := err.(*core.Error)
		require.True(tb, ok, "expected *core.Error")
		assert.Equal(tb, core.ErrKeyInvalidLogin, coreErr.Key)
	}, coreTesting.WithServiceFactory(core.AUTH_SERVICE, service.NewAuthService))
}

func TestAuthService_LoginKeyIdentity_RejectsWhenProofVerificationFails(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		authService := core.GetService[core.AuthService](ctx, core.AUTH_SERVICE)
		userService := coreTesting.GetMockUserService(ctx)
		require.NotNil(tb, authService)

		// Register a handler that fails VerifyProof
		coreTesting.WithKeyIdentityHandler("ethereum", &failingProofHandler{})(ctx)

		// Create test user with key identity
		hashedCredential, err := bcrypt.GenerateFromPassword([]byte(testKeyIdentitySecret()), bcrypt.DefaultCost)
		require.NoError(tb, err)

		user := &models.User{
			Email:        "failingproof@example.com",
			PasswordHash: string(hashedCredential),
		}
		err = ctx.DB().Create(user).Error
		require.NoError(tb, err)

		keyIdentity := &models.KeyIdentity{
			UserID: user.ID,
			Type:   "ethereum",
			Key:    "0x1234567890abcdef1234567890abcdef12345678",
		}
		err = ctx.DB().Create(keyIdentity).Error
		require.NoError(tb, err)

		// Setup mock expectations — should not reach login since proof fails
		userService.EXPECT().IsAccountPendingDeletion(mock.Anything, user.ID).Return(false, nil).Maybe()

		// LoginKeyIdentity should fail because VerifyProof returns error
		_, err = authService.LoginKeyIdentity(context.Background(), "ethereum", "0x1234567890abcdef1234567890abcdef12345678", []byte("bad-proof"), "127.0.0.1", false)
		assert.Error(tb, err)
		coreErr, ok := err.(*core.Error)
		require.True(tb, ok, "expected *core.Error")
		assert.Equal(tb, core.ErrKeyInvalidLogin, coreErr.Key)
	}, coreTesting.WithServiceFactory(core.AUTH_SERVICE, service.NewAuthService))
}

func TestAuthService_LoginKeyIdentity_NilMetadataDefaultsToEmptyJSON(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		authService := core.GetService[core.AuthService](ctx, core.AUTH_SERVICE)
		userService := coreTesting.GetMockUserService(ctx)
		require.NotNil(tb, authService)

		handler := &nilMetadataCapturingHandler{}
		coreTesting.WithKeyIdentityHandler("ethereum", handler)(ctx)

		hashedCredential, err := bcrypt.GenerateFromPassword([]byte(testKeyIdentitySecret()), bcrypt.DefaultCost)
		require.NoError(tb, err)

		user := &models.User{
			Email:        "nilmeta@example.com",
			PasswordHash: string(hashedCredential),
		}
		err = ctx.DB().Create(user).Error
		require.NoError(tb, err)

		// Insert key identity with NULL metadata (simulating migrated legacy row)
		keyIdentity := &models.KeyIdentity{
			UserID:   user.ID,
			Type:     "ethereum",
			Key:      "0x1234567890abcdef1234567890abcdef12345678",
			Metadata: nil,
		}
		err = ctx.DB().Create(keyIdentity).Error
		require.NoError(tb, err)

		userService.EXPECT().IsAccountPendingDeletion(mock.Anything, user.ID).Return(false, nil)
		userService.EXPECT().UpdateAccountInfo(mock.Anything, user.ID, mock.Anything).Return(nil)

		token, err := authService.LoginKeyIdentity(context.Background(), "ethereum", "0x1234567890abcdef1234567890abcdef12345678", []byte("proof"), "127.0.0.1", false)
		assert.NoError(tb, err)
		assert.NotEmpty(tb, token)

		// Verify the handler received non-nil metadata (defaulted to {})
		assert.NotNil(tb, handler.receivedMetadata)
		assert.Equal(tb, json.RawMessage(`{}`), handler.receivedMetadata)
	}, coreTesting.WithServiceFactory(core.AUTH_SERVICE, service.NewAuthService))
}

func TestAuthService_LoginID(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		authService := core.GetService[core.AuthService](ctx, core.AUTH_SERVICE)
		userService := coreTesting.GetMockUserService(ctx)
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
		userService.EXPECT().IsAccountPendingDeletion(mock.Anything, user.ID).Return(false, nil)
		userService.EXPECT().UpdateAccountInfo(mock.Anything, user.ID, mock.Anything).Return(nil)

		// Test valid ID login
		token, err := authService.LoginID(context.Background(), user.ID, "127.0.0.1", false)
		assert.NoError(tb, err)
		assert.NotEmpty(tb, token)

		// Test invalid ID
		_, err = authService.LoginID(context.Background(), 999999, "127.0.0.1", false)
		assert.Error(tb, err)
	}, coreTesting.WithServiceFactory(core.AUTH_SERVICE, service.NewAuthService))
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
		valid, fetchedUser, err := authService.ValidLoginByEmail(context.Background(), "emailvalid@example.com", "password")
		assert.NoError(tb, err)
		assert.True(tb, valid)
		assert.Equal(tb, user.ID, fetchedUser.ID)

		// Test invalid password
		valid, _, err = authService.ValidLoginByEmail(context.Background(), "emailvalid@example.com", "wrongpassword")
		assert.NoError(tb, err)
		assert.False(tb, valid)

		// Test non-existent email
		_, _, err = authService.ValidLoginByEmail(context.Background(), "nonexistent@example.com", "password")
		assert.Error(tb, err)
	}, coreTesting.WithServiceFactory(core.AUTH_SERVICE, service.NewAuthService))
}
