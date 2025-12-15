package testing

import (
	"fmt"

	"github.com/stretchr/testify/mock"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/testing/mocks"
	"go.lumeweb.com/portal/db/models"
	"gorm.io/gorm"
)

// MockAuthService provides high-level helper functions that embed MockAuthService.
// It automatically sets up mock expectations and provides convenient methods for common auth operations.
type MockAuthService struct {
	*mocks.MockAuthService
	userSvc *MockUserService
	ctx     TestContext
}

// NewMockAuthService creates a new MockAuthService that embeds auth mock and has private user mock.
func NewMockAuthService(ctx TestContext) *MockAuthService {
	mockAuth := &MockAuthService{
		MockAuthService: mocks.NewMockAuthService(ctx.T()),
		ctx:             ctx,
	}

	// Register a startup function to get the user service
	ctx.OnStartup(func(coreCtx core.Context) error {
		mockAuth.userSvc = core.GetService[*MockUserService](coreCtx, core.USER_SERVICE)
		return nil
	})

	return mockAuth
}

// CreateAndLoginUser creates a test user and returns JWT token with automatic mock setup.
func (m *MockAuthService) CreateAndLoginUser(email, password string) (string, *models.User, error) {
	// Create test user object
	user := &models.User{
		Model:        gorm.Model{ID: 1},
		Email:        email,
		PasswordHash: m.hashPassword(password),
		Verified:     true,
	}

	// Setup mock expectations for login
	m.setupLoginExpectations(user, "127.0.0.1", false)

	// Mock the login response
	token := m.generateTestToken(user.ID)
	m.EXPECT().LoginPassword(email, password, "127.0.0.1", false).Return(token, user, nil)

	return token, user, nil
}

// CreateAndLoginUserWithRemember creates a test user and returns long-lived JWT token with automatic mock setup.
func (m *MockAuthService) CreateAndLoginUserWithRemember(email, password string) (string, *models.User, error) {
	// Create test user object
	user := &models.User{
		Model:        gorm.Model{ID: 1},
		Email:        email,
		PasswordHash: m.hashPassword(password),
		Verified:     true,
	}

	// Setup mock expectations for login
	m.setupLoginExpectations(user, "127.0.0.1", true)

	// Mock the login response
	token := m.generateTestToken(user.ID)
	m.EXPECT().LoginPassword(email, password, "127.0.0.1", true).Return(token, user, nil)

	return token, user, nil
}

// LoginUser logs in an existing user and returns JWT token with automatic mock setup.
func (m *MockAuthService) LoginUser(email, password string) (string, *models.User, error) {
	// Create test user object (would normally be found via user service)
	user := &models.User{
		Model:        gorm.Model{ID: 1},
		Email:        email,
		PasswordHash: m.hashPassword(password),
		Verified:     true,
	}

	// Setup mock expectations for login
	m.setupLoginExpectations(user, "127.0.0.1", false)

	// Mock the login response
	token := m.generateTestToken(user.ID)
	m.EXPECT().LoginPassword(email, password, "127.0.0.1", false).Return(token, user, nil)

	return token, user, nil
}

// LoginUserWithIP logs in an existing user with custom IP and returns JWT token with automatic mock setup.
func (m *MockAuthService) LoginUserWithIP(email, password, ip string) (string, *models.User, error) {
	// Create test user object
	user := &models.User{
		Model:        gorm.Model{ID: 1},
		Email:        email,
		PasswordHash: m.hashPassword(password),
		Verified:     true,
	}

	// Setup mock expectations for login
	m.setupLoginExpectations(user, ip, false)

	// Mock the login response
	token := m.generateTestToken(user.ID)
	m.EXPECT().LoginPassword(email, password, ip, false).Return(token, user, nil)

	return token, user, nil
}

// LoginUserByID logs in by user ID and returns JWT token with automatic mock setup.
func (m *MockAuthService) LoginUserByID(userID uint) (string, error) {
	// Create test user object
	user := &models.User{
		Model:        gorm.Model{ID: userID},
		Email:        fmt.Sprintf("user%d@example.com", userID),
		PasswordHash: m.hashPassword("password"),
		Verified:     true,
	}

	// Setup mock expectations for login
	m.setupLoginExpectations(user, "127.0.0.1", false)

	// Mock the login response
	token := m.generateTestToken(user.ID)
	m.EXPECT().LoginID(userID, "127.0.0.1", false).Return(token, nil)

	return token, nil
}

// LoginUserWithOTP logs in using OTP and returns JWT token with automatic mock setup.
func (m *MockAuthService) LoginUserWithOTP(userID uint, otpCode string) (string, error) {
	// Setup mock expectations for OTP login
	m.setupOTPLoginExpectations(userID)

	// Mock the OTP login response
	token := m.generateTestToken(userID)
	m.EXPECT().LoginOTP(userID, otpCode, false).Return(token, nil)

	return token, nil
}

// ValidatePassword validates password for a user with automatic mock setup.
func (m *MockAuthService) ValidatePassword(user *models.User, password string) bool {
	// Mock the password validation
	isValid := m.validatePasswordHash(user.PasswordHash, password)
	m.EXPECT().ValidLoginByUserObj(user, password).Return(isValid)

	return isValid
}

// SetupLoginExpectations manually sets up login expectations for a user.
func (m *MockAuthService) SetupLoginExpectations(user *models.User, ip string, rememberMe bool) {
	m.setupLoginExpectations(user, ip, rememberMe)
}

// SetupOTPLoginExpectations manually sets up OTP login expectations.
func (m *MockAuthService) SetupOTPLoginExpectations(userID uint, otpCode string) {
	m.setupOTPLoginExpectations(userID)
}

// setupLoginExpectations sets up mock expectations for login operations.
func (m *MockAuthService) setupLoginExpectations(user *models.User, ip string, rememberMe bool) {
	// Expect pending deletion check
	m.userSvc.MockUserService.EXPECT().IsAccountPendingDeletion(user.ID).Return(false, nil)

	// Expect account info update - use mock.Anything for time to avoid flaky tests
	m.userSvc.MockUserService.EXPECT().UpdateAccountInfo(user.ID, mock.Anything).Return(nil)
}

// setupOTPLoginExpectations sets up mock expectations for OTP login operations.
func (m *MockAuthService) setupOTPLoginExpectations(userID uint) {
	// Expect pending deletion check
	m.userSvc.MockUserService.EXPECT().IsAccountPendingDeletion(userID).Return(false, nil)

	// Expect account info update - use mock.Anything for time to avoid flaky tests
	m.userSvc.MockUserService.EXPECT().UpdateAccountInfo(userID, mock.Anything).Return(nil)
}

// hashPassword creates a simple hash for testing (in real scenario would use bcrypt)
func (m *MockAuthService) hashPassword(password string) string {
	// Simple hash for testing - in real implementation would use bcrypt
	return fmt.Sprintf("hashed_%s", password)
}

// validatePasswordHash validates a simple hash for testing
func (m *MockAuthService) validatePasswordHash(hash, password string) bool {
	return hash == fmt.Sprintf("hashed_%s", password)
}

// generateTestToken generates a simple JWT token for testing
func (m *MockAuthService) generateTestToken(userID uint) string {
	return fmt.Sprintf("test_token_%d", userID)
}

// GetUserService returns the embedded user service for advanced usage.
func (m *MockAuthService) GetUserService() *MockUserService {
	return m.userSvc
}
