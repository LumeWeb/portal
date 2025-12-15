package testing

import (
	"fmt"

	"go.lumeweb.com/portal/core/testing/mocks"
	"go.lumeweb.com/portal/db/models"
	"gorm.io/gorm"
)

// MockUserService provides high-level helper functions that embed MockUserService.
// It automatically sets up mock expectations and provides convenient methods for common user operations.
type MockUserService struct {
	*mocks.MockUserService
	ctx TestContext
}

// NewMockUserService creates a new MockUserService that embeds user mock service.
func NewMockUserService(ctx TestContext) *MockUserService {
	return &MockUserService{
		MockUserService: mocks.NewMockUserService(ctx.T()),
		ctx:             ctx,
	}
}

// CreateTestUser creates a test user account with automatic mock setup.
func (m *MockUserService) CreateTestUser(email, password string) (*models.User, error) {
	// Create test user object
	user := &models.User{
		Model:        gorm.Model{ID: 1},
		Email:        email,
		PasswordHash: m.hashPassword(password),
		Verified:     true,
	}

	// Mock the CreateAccount response
	m.EXPECT().CreateAccount(email, password, true).Return(user, nil)

	return user, nil
}

// CreateUnverifiedUser creates an unverified test user with automatic mock setup.
func (m *MockUserService) CreateUnverifiedUser(email, password string) (*models.User, error) {
	// Create test user object
	user := &models.User{
		Model:        gorm.Model{ID: 1},
		Email:        email,
		PasswordHash: m.hashPassword(password),
		Verified:     false,
	}

	// Mock the CreateAccount response
	m.EXPECT().CreateAccount(email, password, false).Return(user, nil)

	return user, nil
}

// CreateAdminUser creates a test admin user with automatic mock setup.
func (m *MockUserService) CreateAdminUser(email, password string) (*models.User, error) {
	// Create test user object
	user := &models.User{
		Model:        gorm.Model{ID: 1},
		Email:        email,
		PasswordHash: m.hashPassword(password),
		Verified:     true,
		Role:         "admin",
	}

	// Mock the CreateAccount response
	m.EXPECT().CreateAccount(email, password, true).Return(user, nil)

	// Mock the UpdateAccountInfo response for role change
	updateData := map[string]any{"role": "admin"}
	m.EXPECT().UpdateAccountInfo(user.ID, updateData).Return(nil)

	return user, nil
}

// UpdateUserName updates user's first and last name with automatic mock setup.
func (m *MockUserService) UpdateUserName(userID uint, firstName, lastName string) error {
	// Mock the UpdateAccountName response
	m.EXPECT().UpdateAccountName(userID, firstName, lastName).Return(nil)

	return nil
}

// UpdateUserEmail updates user's email with automatic mock setup.
func (m *MockUserService) UpdateUserEmail(userID uint, newEmail, password string) error {
	// Mock the UpdateAccountEmail response
	m.EXPECT().UpdateAccountEmail(userID, newEmail, password).Return(nil)

	return nil
}

// UpdateUserPassword updates user's password with automatic mock setup.
func (m *MockUserService) UpdateUserPassword(userID uint, currentPassword, newPassword string) error {
	// Mock the UpdateAccountPassword response
	m.EXPECT().UpdateAccountPassword(userID, currentPassword, newPassword).Return(nil)

	return nil
}

// DeleteUser deletes a user account with automatic mock setup.
func (m *MockUserService) DeleteUser(userID uint) error {
	// Mock the DeleteAccount response
	m.EXPECT().DeleteAccount(userID).Return(nil)

	return nil
}

// UserExists checks if user exists by ID with automatic mock setup.
func (m *MockUserService) UserExists(userID uint) (bool, *models.User, error) {
	// Create test user object
	user := &models.User{
		Model:    gorm.Model{ID: userID},
		Email:    fmt.Sprintf("user%d@example.com", userID),
		Verified: true,
	}

	// Mock the AccountExists response
	m.EXPECT().AccountExists(userID).Return(true, user, nil)

	return true, user, nil
}

// UserDoesNotExist checks if user does not exist by ID with automatic mock setup.
func (m *MockUserService) UserDoesNotExist(userID uint) (bool, *models.User, error) {
	// Mock the AccountExists response for not found
	m.EXPECT().AccountExists(userID).Return(false, nil, nil)

	return false, nil, nil
}

// EmailExists checks if email exists with automatic mock setup.
func (m *MockUserService) EmailExists(email string) (bool, *models.User, error) {
	// Create test user object
	user := &models.User{
		Model:    gorm.Model{ID: 1},
		Email:    email,
		Verified: true,
	}

	// Mock the EmailExists response
	m.EXPECT().EmailExists(email).Return(true, user, nil)

	return true, user, nil
}

// EmailDoesNotExist checks if email does not exist with automatic mock setup.
func (m *MockUserService) EmailDoesNotExist(email string) (bool, *models.User, error) {
	// Mock the EmailExists response for not found
	m.EXPECT().EmailExists(email).Return(false, nil, nil)

	return false, nil, nil
}

// IsUserVerified checks if user is verified with automatic mock setup.
func (m *MockUserService) IsUserVerified(userID uint) (bool, error) {
	// Mock the IsAccountVerified response
	m.EXPECT().IsAccountVerified(userID).Return(true, nil)

	return true, nil
}

// IsUserUnverified checks if user is not verified with automatic mock setup.
func (m *MockUserService) IsUserUnverified(userID uint) (bool, error) {
	// Mock the IsAccountVerified response
	m.EXPECT().IsAccountVerified(userID).Return(false, nil)

	return false, nil
}

// VerifyUserEmail verifies user's email with automatic mock setup.
func (m *MockUserService) VerifyUserEmail(email string) error {
	// Mock the VerifyEmail response
	m.EXPECT().VerifyEmail(email, "test_verification_token").Return(nil)

	return nil
}

// VerifyUserEmailFails simulates email verification failure with automatic mock setup.
func (m *MockUserService) VerifyUserEmailFails(email string) error {
	// Mock the VerifyEmail response with error
	m.EXPECT().VerifyEmail(email, "test_verification_token").Return(fmt.Errorf("verification failed"))

	return fmt.Errorf("verification failed")
}

// AddPublicKey adds public key to user account with automatic mock setup.
func (m *MockUserService) AddPublicKey(user *models.User, publicKey string) error {
	// Mock the AddPubkeyToAccount response
	m.EXPECT().AddPubkeyToAccount(*user, publicKey).Return(nil)

	return nil
}

// AddPublicKeyFails simulates public key addition failure with automatic mock setup.
func (m *MockUserService) AddPublicKeyFails(user *models.User, publicKey string) error {
	// Mock the AddPubkeyToAccount response with error
	m.EXPECT().AddPubkeyToAccount(*user, publicKey).Return(fmt.Errorf("key already exists"))

	return fmt.Errorf("key already exists")
}

// HashPassword hashes a password with automatic mock setup.
func (m *MockUserService) HashPassword(password string) (string, error) {
	hash := m.hashPassword(password)

	// Mock the HashPassword response
	m.EXPECT().HashPassword(password).Return(hash, nil)

	return hash, nil
}

// HashPasswordFails simulates password hashing failure with automatic mock setup.
func (m *MockUserService) HashPasswordFails(password string) (string, error) {
	// Mock the HashPassword response with error
	m.EXPECT().HashPassword(password).Return("", fmt.Errorf("hashing failed"))

	return "", fmt.Errorf("hashing failed")
}

// SetupUserExistsExpectation sets up user existence expectation.
func (m *MockUserService) SetupUserExistsExpectation(userID uint, exists bool, user *models.User, err error) {
	m.EXPECT().AccountExists(userID).Return(exists, user, err)
}

// SetupEmailExistsExpectation sets up email existence expectation.
func (m *MockUserService) SetupEmailExistsExpectation(email string, exists bool, user *models.User, err error) {
	m.EXPECT().EmailExists(email).Return(exists, user, err)
}

// SetupUpdateAccountInfoExpectation sets up account info update expectation.
func (m *MockUserService) SetupUpdateAccountInfoExpectation(userID uint, updateData map[string]any, err error) {
	m.EXPECT().UpdateAccountInfo(userID, updateData).Return(err)
}

// hashPassword creates a simple hash for testing (in real scenario would use bcrypt)
func (m *MockUserService) hashPassword(password string) string {
	// Simple hash for testing - in real implementation would use bcrypt
	return fmt.Sprintf("hashed_%s", password)
}
