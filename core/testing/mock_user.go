package testing

import (
	"context"
	"fmt"

	"github.com/stretchr/testify/mock"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/testing/mocks"
	"go.lumeweb.com/portal/db/models"
	"gorm.io/gorm"
)

// MockUserService provides high-level helper functions that embeds mocks.MockUserService.
// It automatically sets up mock expectations and provides convenient methods for common user operations.
type MockUserService struct {
	*mocks.MockUserService
	componentConfig  config.Manager
	componentContext core.Context
	componentLogger  *core.Logger
	componentDB       *gorm.DB
	ctx              TestContext
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

	// Mock of CreateAccount response
	m.EXPECT().CreateAccount(mock.Anything, email, password, true).Return(user, nil)

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

	// Mock of CreateAccount response
	m.EXPECT().CreateAccount(mock.Anything, email, password, false).Return(user, nil)

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

	// Mock of CreateAccount response
	m.EXPECT().CreateAccount(mock.Anything, email, password, true).Return(user, nil)

	// Mock of UpdateAccountInfo response for role change
	updateData := map[string]any{"role": "admin"}
	m.EXPECT().UpdateAccountInfo(mock.Anything, user.ID, updateData).Return(nil)

	return user, nil
}

// UpdateUserName updates user's first and last name with automatic mock setup.
func (m *MockUserService) UpdateUserName(userID uint, firstName, lastName string) error {
	// Mock of UpdateAccountName response
	m.EXPECT().UpdateAccountName(mock.Anything, userID, firstName, lastName).Return(nil)

	return nil
}

// UpdateUserEmail updates user's email with automatic mock setup.
func (m *MockUserService) UpdateUserEmail(userID uint, newEmail, password string) error {
	// Mock of UpdateAccountEmail response
	m.EXPECT().UpdateAccountEmail(mock.Anything, userID, newEmail, password).Return(nil)

	return nil
}

// UpdateUserPassword updates user's password with automatic mock setup.
func (m *MockUserService) UpdateUserPassword(userID uint, currentPassword, newPassword string) error {
	// Mock of UpdateAccountPassword response
	m.EXPECT().UpdateAccountPassword(mock.Anything, userID, currentPassword, newPassword).Return(nil)

	return nil
}

// DeleteUser deletes a user account with automatic mock setup.
func (m *MockUserService) DeleteUser(userID uint) error {
	// Mock of DeleteAccount response
	m.EXPECT().DeleteAccount(mock.Anything, userID).Return(nil)

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

	// Mock of AccountExists response
	m.EXPECT().AccountExists(mock.Anything, userID).Return(true, user, nil)

	return true, user, nil
}

// UserDoesNotExist checks if user does not exist by ID with automatic mock setup.
func (m *MockUserService) UserDoesNotExist(userID uint) (bool, *models.User, error) {
	// Mock of AccountExists response for not found
	m.EXPECT().AccountExists(mock.Anything, userID).Return(false, nil, nil)

	return false, nil, nil
}

// EmailDoesNotExist checks if email does not exist with automatic mock setup.
func (m *MockUserService) EmailDoesNotExist(email string) (bool, *models.User, error) {
	// Mock of EmailExists response for not found
	m.EXPECT().EmailExists(mock.Anything, email).Return(false, nil, nil)

	return false, nil, nil
}

// IsUserVerified checks if user is verified with automatic mock setup.
func (m *MockUserService) IsUserVerified(userID uint) (bool, error) {
	// Mock of IsAccountVerified response
	m.EXPECT().IsAccountVerified(mock.Anything, userID).Return(true, nil)

	return true, nil
}

// IsUserUnverified checks if user is not verified with automatic mock setup.
func (m *MockUserService) IsUserUnverified(userID uint) (bool, error) {
	// Mock of IsAccountVerified response
	m.EXPECT().IsAccountVerified(mock.Anything, userID).Return(false, nil)

	return false, nil
}

// VerifyUserEmail verifies user's email with automatic mock setup.
func (m *MockUserService) VerifyUserEmail(email string) error {
	// Mock of VerifyEmail response
	m.EXPECT().VerifyEmail(mock.Anything, email, "test_verification_token").Return(nil)

	return nil
}

// VerifyUserEmailFails simulates email verification failure with automatic mock setup.
func (m *MockUserService) VerifyUserEmailFails(email string) error {
	// Mock of VerifyEmail response with error
	m.EXPECT().VerifyEmail(mock.Anything, email, "test_verification_token").Return(fmt.Errorf("verification failed"))

	return fmt.Errorf("verification failed")
}

// AddKeyIdentityForUser adds a key identity to user account with automatic mock setup.
func (m *MockUserService) AddKeyIdentityForUser(user *models.User, keyType string, key string) error {
	// Mock of AddKeyIdentity response
	m.EXPECT().AddKeyIdentity(mock.Anything, *user, keyType, key, mock.Anything).Return(nil)

	return nil
}

// AddKeyIdentityForUserFails simulates key identity addition failure with automatic mock setup.
func (m *MockUserService) AddKeyIdentityForUserFails(user *models.User, keyType string, key string) error {
	// Mock of AddKeyIdentity response with error
	m.EXPECT().AddKeyIdentity(mock.Anything, *user, keyType, key, mock.Anything).Return(fmt.Errorf("key already exists"))

	return fmt.Errorf("key already exists")
}

// HashPasswordFails simulates password hashing failure with automatic mock setup.
func (m *MockUserService) HashPasswordFails(password string) (string, error) {
	// Mock of HashPassword response with error
	m.EXPECT().HashPassword(password).Return("", fmt.Errorf("hashing failed"))

	return "", fmt.Errorf("hashing failed")
}

// SetupUserExistsExpectation sets up user existence expectation.
func (m *MockUserService) SetupUserExistsExpectation(userID uint, exists bool, user *models.User, err error) {
	m.EXPECT().AccountExists(mock.Anything, userID).Return(exists, user, err)
}

// SetupEmailExistsExpectation sets up email existence expectation.
func (m *MockUserService) SetupEmailExistsExpectation(email string, exists bool, user *models.User, err error) {
	m.EXPECT().EmailExists(mock.Anything, email).Return(exists, user, err)
}

// SetupUpdateAccountInfoExpectation sets up account info update expectation.
func (m *MockUserService) SetupUpdateAccountInfoExpectation(userID uint, updateData map[string]any, err error) {
	m.EXPECT().UpdateAccountInfo(mock.Anything, userID, updateData).Return(err)
}

// AccountExists checks if account exists by ID with automatic mock setup.
// If no expectation was set by the test, adds a safe default expectation.
func (m *MockUserService) AccountExists(ctx context.Context, userID uint) (bool, *models.User, error) {
	// Check if test already set up an expectation for AccountExists
	// If not, add a safe default expectation (account does not exist)
	if !HasExpectationForMethod(&m.MockUserService.Mock, "AccountExists") {
		m.EXPECT().AccountExists(mock.Anything, mock.AnythingOfType("uint")).
			Return(false, nil, nil).Maybe()
	}

	// Delegate to the underlying mock implementation
	return m.MockUserService.AccountExists(ctx, userID)
}

// EmailExists checks if email exists with automatic mock setup.
// If no expectation was set by the test, adds a safe default expectation.
func (m *MockUserService) EmailExists(ctx context.Context, email string) (bool, *models.User, error) {
	// Check if test already set up an expectation for EmailExists
	// If not, add a safe default expectation (email does not exist)
	if !HasExpectationForMethod(&m.MockUserService.Mock, "EmailExists") {
		m.EXPECT().EmailExists(mock.Anything, mock.AnythingOfType("string")).
			Return(false, nil, nil).Maybe()
	}

	// Delegate to the underlying mock implementation
	return m.MockUserService.EmailExists(ctx, email)
}

// HashPassword hashes a password with automatic mock setup.
// If no expectation was set by the test, adds a safe default expectation.
func (m *MockUserService) HashPassword(password string) (string, error) {
	// Check if test already, add a safe default expectation
	if !HasExpectationForMethod(&m.MockUserService.Mock, "HashPassword") {
		// Generate a simple hash for the default return
		hash := m.hashPassword(password)
		m.EXPECT().HashPassword(mock.AnythingOfType("string")).
			Return(hash, nil).Maybe()
	}

	// Delegate to the underlying mock implementation
	return m.MockUserService.HashPassword(password)
}

// hashPassword creates a simple hash for testing (in real scenario would use bcrypt)
func (m *MockUserService) hashPassword(password string) string {
	// Simple hash for testing - in real implementation would use bcrypt
	return fmt.Sprintf("hashed_%s", password)
}

// Config implements core.Component
func (m *MockUserService) Config() config.Manager {
	return m.componentConfig
}

// SetConfig implements core.Component
func (m *MockUserService) SetConfig(cfg config.Manager) {
	m.componentConfig = cfg
}

// Context implements core.Component
func (m *MockUserService) Context() core.Context {
	return m.componentContext
}

// SetContext implements core.Component
func (m *MockUserService) SetContext(ctx core.Context) {
	m.componentContext = ctx
}

// Logger implements core.Component
func (m *MockUserService) Logger() *core.Logger {
	return m.componentLogger
}

// SetLogger implements core.Component
func (m *MockUserService) SetLogger(logger *core.Logger) {
	m.componentLogger = logger
}

// DB implements core.Component
func (m *MockUserService) DB() *gorm.DB {
	return m.componentDB
}

// SetDB implements core.Component
func (m *MockUserService) SetDB(db *gorm.DB) {
	m.componentDB = db
}
