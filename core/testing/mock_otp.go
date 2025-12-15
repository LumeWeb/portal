package testing

import (
	"fmt"

	"go.lumeweb.com/portal/core/testing/mocks"
	"go.lumeweb.com/portal/db/models"
	"gorm.io/gorm"
)

// MockOTPService provides high-level helper functions that embed MockOTPService.
// It automatically sets up mock expectations and provides convenient methods for common OTP operations.
type MockOTPService struct {
	*mocks.MockOTPService
	ctx TestContext
}

// NewMockOTPService creates a new MockOTPService that embeds OTP mock service.
func NewMockOTPService(ctx TestContext) *MockOTPService {
	return &MockOTPService{
		MockOTPService: mocks.NewMockOTPService(ctx.T()),
		ctx:            ctx,
	}
}

// GenerateSecret generates an OTP secret with automatic mock setup.
func (m *MockOTPService) GenerateSecret(userID uint) (string, error) {
	secret := "test_otp_secret_12345"

	// Mock the OTPGenerate response
	m.EXPECT().OTPGenerate(userID).Return(secret, nil)

	return secret, nil
}

// GenerateSecretFails simulates OTP secret generation failure with automatic mock setup.
func (m *MockOTPService) GenerateSecretFails(userID uint) (string, error) {
	// Mock the OTPGenerate response with error
	m.EXPECT().OTPGenerate(userID).Return("", fmt.Errorf("generation failed"))

	return "", fmt.Errorf("generation failed")
}

// GenerateAuthURL generates an OTP auth URL with automatic mock setup.
func (m *MockOTPService) GenerateAuthURL(secret, accountName string) (string, error) {
	return fmt.Sprintf("otpauth://totp/%s?secret=%s&issuer=test", accountName, secret), nil
}

// VerifyOTP verifies an OTP code for a user with automatic mock setup.
func (m *MockOTPService) VerifyOTP(userID uint, code string) (bool, error) {
	// Mock the OTPVerify response
	isValid := code == "123456" // Accept test code "123456"
	m.EXPECT().OTPVerify(userID, code).Return(isValid, nil)

	return isValid, nil
}

// VerifyOTPFails simulates OTP verification failure with automatic mock setup.
func (m *MockOTPService) VerifyOTPFails(userID uint, code string) (bool, error) {
	// Mock the OTPVerify response with error
	m.EXPECT().OTPVerify(userID, code).Return(false, fmt.Errorf("verification failed"))

	return false, fmt.Errorf("verification failed")
}

// CreateOTPUser creates a user with OTP enabled with automatic mock setup.
func (m *MockOTPService) CreateOTPUser(userID uint, email, password string) (*models.User, string, string, error) {
	// Generate OTP secret and auth URL
	secret, err := m.GenerateSecret(userID)
	if err != nil {
		return nil, "", "", err
	}

	authURL, err := m.GenerateAuthURL(secret, email)
	if err != nil {
		return nil, "", "", err
	}

	// Create user with OTP
	user := &models.User{
		Model:        gorm.Model{ID: userID},
		Email:        email,
		PasswordHash: m.hashPassword(password),
		Verified:     true,
		OTPEnabled:   true,
		OTPVerified:  true,
		OTPSecret:    secret,
		OTPAuthUrl:   authURL,
	}

	return user, secret, authURL, nil
}

// SetupOTPForUser sets up OTP for an existing user with automatic mock setup.
func (m *MockOTPService) SetupOTPForUser(userID uint, accountName string) (string, string, error) {
	// Generate OTP secret and auth URL
	secret, err := m.GenerateSecret(userID)
	if err != nil {
		return "", "", err
	}

	authURL, err := m.GenerateAuthURL(secret, accountName)
	if err != nil {
		return "", "", err
	}

	return secret, authURL, nil
}

// EnableOTPForUser enables and verifies OTP for an existing user with automatic mock setup.
func (m *MockOTPService) EnableOTPForUser(userID uint, accountName string) (string, string, error) {
	secret, authURL, err := m.SetupOTPForUser(userID, accountName)
	if err != nil {
		return "", "", err
	}

	// Verify OTP with test code
	_, err = m.VerifyOTP(userID, "123456")
	if err != nil {
		return "", "", err
	}

	return secret, authURL, nil
}

// DisableOTPForUser disables OTP for an existing user with automatic mock setup.
func (m *MockOTPService) DisableOTPForUser(userID uint) error {
	// No mock expectations needed for disable operation
	return nil
}

// TestOTPLoginWorkflow tests a complete OTP login workflow with automatic mock setup.
func (m *MockOTPService) TestOTPLoginWorkflow(email, password string) (string, *models.User, error) {
	// Create user with OTP
	user, _, _, err := m.CreateOTPUser(1, email, password)
	if err != nil {
		return "", nil, err
	}

	// Generate test OTP code
	testCode := "123456"

	// Verify OTP
	_, err = m.VerifyOTP(user.ID, testCode)
	if err != nil {
		return "", nil, err
	}

	// Generate test token (would normally be done by auth service)
	token := fmt.Sprintf("otp_token_%d", user.ID)

	return token, user, nil
}

// TestOTPSetupWorkflow tests a complete OTP setup workflow with automatic mock setup.
func (m *MockOTPService) TestOTPSetupWorkflow(email, password string) (string, string, error) {
	// Create regular user first
	user := &models.User{
		Model:        gorm.Model{ID: 1},
		Email:        email,
		PasswordHash: m.hashPassword(password),
		Verified:     true,
	}

	// Setup OTP for the user
	secret, authURL, err := m.EnableOTPForUser(user.ID, email)
	if err != nil {
		return "", "", err
	}

	return secret, authURL, nil
}

// TestOTPDisableWorkflow tests disabling OTP for a user with automatic mock setup.
func (m *MockOTPService) TestOTPDisableWorkflow(email, password string) error {
	// Create user with OTP
	user, _, _, err := m.CreateOTPUser(1, email, password)
	if err != nil {
		return err
	}

	// Disable OTP
	err = m.DisableOTPForUser(user.ID)
	if err != nil {
		return err
	}

	return nil
}

// ValidateOTPConfiguration validates that OTP is properly configured for a user.
func (m *MockOTPService) ValidateOTPConfiguration(user *models.User) bool {
	return user.OTPEnabled &&
		user.OTPVerified &&
		user.OTPSecret != "" &&
		user.OTPAuthUrl != ""
}

// GetOTPStatus returns OTP status for a user.
func (m *MockOTPService) GetOTPStatus(user *models.User) map[string]interface{} {
	return map[string]interface{}{
		"enabled":    user.OTPEnabled,
		"verified":   user.OTPVerified,
		"secret":     user.OTPSecret,
		"auth_url":   user.OTPAuthUrl,
		"configured": m.ValidateOTPConfiguration(user),
	}
}

// SetupOTPExpectations manually sets up OTP expectations for testing.
func (m *MockOTPService) SetupOTPExpectations(userID uint, validCode string, invalidCode string) {
	// Setup expectations for valid and invalid codes
	m.EXPECT().OTPVerify(userID, validCode).Return(true, nil)
	m.EXPECT().OTPVerify(userID, invalidCode).Return(false, nil)
}

// SetupOTPGenerationExpectations manually sets up OTP generation expectations.
func (m *MockOTPService) SetupOTPGenerationExpectations(userID uint, secret string, err error) {
	m.EXPECT().OTPGenerate(userID).Return(secret, err)
}

// hashPassword creates a simple hash for testing (in real scenario would use bcrypt)
func (m *MockOTPService) hashPassword(password string) string {
	// Simple hash for testing - in real implementation would use bcrypt
	return fmt.Sprintf("hashed_%s", password)
}
