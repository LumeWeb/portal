package testing

import (
	"context"
	"fmt"
	"sync"

	"github.com/stretchr/testify/mock"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/testing/mocks"
	"go.lumeweb.com/portal/db/models"
	"gorm.io/gorm"
)

// MockAuthService provides high-level helper functions that embed MockAuthService.
// It automatically sets up mock expectations and provides convenient methods for common auth operations.
// LoginTokenRegistry holds pre-registered token data for lazy login handling.
type LoginTokenRegistry struct {
	token string
	user  *models.User
}

type MockAuthService struct {
	*mocks.MockAuthService
	userSvc        *MockUserService
	componentConfig config.Manager
	ctx             TestContext
	jwtHelper       *JWTHelper
	tokenRegistry   map[string]LoginTokenRegistry // email => token+user for lazy setup
	registryMutex   sync.RWMutex
}

// NewMockAuthService creates a new MockAuthService that embeds auth mock and has private user mock.
func NewMockAuthService(ctx TestContext) *MockAuthService {
	mockAuth := &MockAuthService{
		MockAuthService: mocks.NewMockAuthService(ctx.T()),
		ctx:             ctx,
		jwtHelper:       NewJWTHelper(ctx),
		tokenRegistry:   make(map[string]LoginTokenRegistry),
	}

	// Register a startup function to get the user service
	ctx.OnStartup(func(coreCtx core.Context) error {
		mockAuth.userSvc = core.GetService[*MockUserService](coreCtx, core.USER_SERVICE)
		return nil
	})

	return mockAuth
}

// ===== Token Registration =====

// RegisterLoginToken registers a token for a specific email. When LoginPassword is called
// for this email, it will return this token lazily along with a default user.
// Use this before making HTTP requests to ensure the mock returns your pre-generated token.
func (m *MockAuthService) RegisterLoginToken(email string, token string) {
	m.RegisterLoginTokenWithUser(email, token, nil)
}

// RegisterLoginTokenWithUser registers a token and user for a specific email. When LoginPassword is called
// for this email, it will return this token and the provided user lazily without needing prior expectation setup.
// Use this when you need to return a user with specific properties (e.g., OTPEnabled=true).
// Pass nil for user to get a default user with just ID and email set.
func (m *MockAuthService) RegisterLoginTokenWithUser(email string, token string, user *models.User) {
	m.registryMutex.Lock()
	defer m.registryMutex.Unlock()
	m.tokenRegistry[email] = LoginTokenRegistry{
		token: token,
		user:  user,
	}
}

// GetRegisteredToken returns the registered token and user for an email.
func (m *MockAuthService) GetRegisteredToken(email string) (string, *models.User, bool) {
	m.registryMutex.RLock()
	defer m.registryMutex.RUnlock()
	entry, exists := m.tokenRegistry[email]
	if !exists {
		return "", nil, false
	}
	return entry.token, entry.user, true
}

// ===== Expectation Setup Helpers =====

// ExpectLoginForUser sets up a LoginPassword expectation for a specific user with a pre-generated token.
// Use this in tests before making HTTP requests - the token will be returned when LoginPassword is called.
func (m *MockAuthService) ExpectLoginForUser(email string, password string, user *models.User, token string) {
	m.EXPECT().LoginPassword(mock.Anything, email, password, mock.Anything, mock.Anything).
		Return(token, user, nil)
}

// ExpectLoginForUserWithErr sets up a LoginPassword expectation that returns an error.
func (m *MockAuthService) ExpectLoginForUserWithErr(email string, password string, err error) {
	m.EXPECT().LoginPassword(mock.Anything, email, password, mock.Anything, mock.Anything).
		Return("", nil, err)
}

// ExpectLoginID sets up a LoginID expectation with a specific token.
func (m *MockAuthService) ExpectLoginID(userID uint, token string, err error) {
	m.EXPECT().LoginID(mock.Anything, userID, mock.Anything, mock.Anything).Return(token, err)
}

// ExpectLoginOTP sets up a LoginOTP expectation with a specific token.
func (m *MockAuthService) ExpectLoginOTP(userID uint, otpCode string, token string, err error) {
	m.EXPECT().LoginOTP(mock.Anything, userID, otpCode, mock.Anything).Return(token, err)
}

// ExpectLoginKeyIdentity sets up a LoginKeyIdentity expectation with a specific token.
func (m *MockAuthService) ExpectLoginKeyIdentity(keyType string, key string, token string, err error) {
	m.EXPECT().LoginKeyIdentity(mock.Anything, keyType, key, mock.Anything, mock.Anything, mock.Anything).Return(token, err)
}

// RegisterLoginForUser generates a token and sets up expectation - returns token for testing.
// Use this when you need the token but want one-step setup.
func (m *MockAuthService) RegisterLoginForUser(email string, password string, user *models.User) (string, error) {
	token := m.generateTestToken(user.ID)
	m.ExpectLoginForUser(email, password, user, token)
	return token, nil
}

// ===== High-Level Helper Methods =====

// CreateAndLoginUser creates a test user and sets up login expectation with valid JWT token.
// Returns the generated token and user for use in tests.
func (m *MockAuthService) CreateAndLoginUser(email, password string) (string, *models.User, error) {
	user := &models.User{
		Model:    gorm.Model{ID: 1},
		Email:    email,
		PasswordHash: m.hashPassword(password),
		Verified: true,
	}

	token := m.generateTestToken(user.ID)
	m.ExpectLoginForUser(email, password, user, token)

	return token, user, nil
}

// CreateAndLoginUserWithRemember creates a test user and sets up expectation with long-lived token.
func (m *MockAuthService) CreateAndLoginUserWithRemember(email, password string) (string, *models.User, error) {
	user := &models.User{
		Model:        gorm.Model{ID: 1},
		Email:        email,
		PasswordHash: m.hashPassword(password),
		Verified:     true,
	}

	token, err := m.jwtHelper.CreateLongLivedToken(user.ID, 30)
	if err != nil {
		token = m.generateTestToken(user.ID)
	}
	m.ExpectLoginForUser(email, password, user, token)

	return token, user, nil
}

// LoginUser sets up expectation for existing user login (no password check).
func (m *MockAuthService) LoginUser(email, password string) (string, *models.User, error) {
	user := &models.User{
		Model:        gorm.Model{ID: 1},
		Email:        email,
		PasswordHash: m.hashPassword(password),
		Verified:     true,
	}

	token := m.generateTestToken(user.ID)
	m.ExpectLoginForUser(email, password, user, token)

	return token, user, nil
}

// LoginUserByID sets up expectation for login by ID.
func (m *MockAuthService) LoginUserByID(userID uint) (string, error) {
	token := m.generateTestToken(userID)
	m.ExpectLoginID(userID, token, nil)
	return token, nil
}

// LoginUserWithOTP sets up expectation for OTP login.
func (m *MockAuthService) LoginUserWithOTP(userID uint, otpCode string) (string, error) {
	token, err := m.jwtHelper.Create2FAToken(userID)
	if err != nil {
		token = m.generateTestToken(userID)
	}
	m.ExpectLoginOTP(userID, otpCode, token, nil)
	return token, nil
}

// ===== Interface Implementations =====

// LoginPassword implements core.AuthService.
// Lazily handles registered tokens or generates default if no expectation set.
func (m *MockAuthService) LoginPassword(ctx context.Context, email string, password string, ip string, rememberMe bool) (string, *models.User, error) {
	// If test already set up an expectation, delegate to mock
	if HasExpectationForMethod(&m.MockAuthService.Mock, "LoginPassword") {
		return m.MockAuthService.LoginPassword(ctx, email, password, ip, rememberMe)
	}

	// Check if a token was registered for this email
	if token, user, exists := m.GetRegisteredToken(email); exists {
		// If user was provided with the registration, use it; otherwise create default
		if user == nil {
			user = &models.User{
				Model:    gorm.Model{ID: 1},
				Email:    email,
				Verified: true,
			}
		}
		return token, user, nil
	}

	// No expectation and no registered token - generate default
	user := &models.User{
		Model:    gorm.Model{ID: 1},
		Email:    email,
		Verified: true,
	}
	token := m.generateTestToken(1)
	return token, user, nil
}

// LoginID implements core.AuthService.
// Lazily generates default token if no expectation set.
func (m *MockAuthService) LoginID(ctx context.Context, id uint, ip string, rememberMe bool) (string, error) {
	if HasExpectationForMethod(&m.MockAuthService.Mock, "LoginID") {
		return m.MockAuthService.LoginID(ctx, id, ip, rememberMe)
	}
	return m.generateTestToken(id), nil
}

// LoginOTP implements core.AuthService.
// Lazily generates default token if no expectation set.
func (m *MockAuthService) LoginOTP(ctx context.Context, userId uint, code string, rememberMe bool) (string, error) {
	if HasExpectationForMethod(&m.MockAuthService.Mock, "LoginOTP") {
		return m.MockAuthService.LoginOTP(ctx, userId, code, rememberMe)
	}
	return m.generateTestToken(userId), nil
}

// LoginKeyIdentity implements core.AuthService.
// Lazily generates default token if no expectation set.
func (m *MockAuthService) LoginKeyIdentity(ctx context.Context, keyType string, key string, proof []byte, ip string, rememberMe bool) (string, error) {
	if HasExpectationForMethod(&m.MockAuthService.Mock, "LoginKeyIdentity") {
		return m.MockAuthService.LoginKeyIdentity(ctx, keyType, key, proof, ip, rememberMe)
	}
	return m.generateTestToken(1), nil
}

// LoginKeyIdentityWithContext implements core.AuthService.
// Lazily generates default token if no expectation set.
func (m *MockAuthService) LoginKeyIdentityWithContext(ctx core.Context, keyType string, key string, proof []byte, ip string, rememberMe bool) (string, error) {
	if HasExpectationForMethod(&m.MockAuthService.Mock, "LoginKeyIdentityWithContext") {
		return m.MockAuthService.LoginKeyIdentityWithContext(ctx, keyType, key, proof, ip, rememberMe)
	}
	return m.generateTestToken(1), nil
}

// ValidLoginByUserObj implements core.AuthService.
func (m *MockAuthService) ValidLoginByUserObj(ctx context.Context, user *models.User, password string) bool {
	return m.validatePasswordHash(user.PasswordHash, password)
}

// ValidLoginByEmail implements core.AuthService.
func (m *MockAuthService) ValidLoginByEmail(ctx context.Context, email string, password string) (bool, *models.User, error) {
	if HasExpectationForMethod(&m.MockAuthService.Mock, "ValidLoginByEmail") {
		return m.MockAuthService.ValidLoginByEmail(ctx, email, password)
	}

	user := &models.User{
		Model:    gorm.Model{ID: 1},
		Email:    email,
		PasswordHash: m.hashPassword(password),
		Verified: true,
	}
	valid := m.validatePasswordHash(m.hashPassword(password), password)
	return valid, user, nil
}

// ValidLoginByUserID implements core.AuthService.
func (m *MockAuthService) ValidLoginByUserID(ctx context.Context, id uint, password string) (bool, *models.User, error) {
	if HasExpectationForMethod(&m.MockAuthService.Mock, "ValidLoginByUserID") {
		return m.MockAuthService.ValidLoginByUserID(ctx, id, password)
	}

	user := &models.User{
		Model:    gorm.Model{ID: id},
		PasswordHash: m.hashPassword(password),
		Verified: true,
	}
	valid := m.validatePasswordHash(m.hashPassword(password), password)
	return valid, user, nil
}

// ===== Setup Methods =====

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
	// Maybe expect pending deletion check
	m.userSvc.MockUserService.EXPECT().IsAccountPendingDeletion(mock.Anything, user.ID).Return(false, nil).Maybe()

	// Maybe expect account info update - use mock.Anything for time to avoid flaky tests
	m.userSvc.MockUserService.EXPECT().UpdateAccountInfo(mock.Anything, user.ID, mock.Anything).Return(nil).Maybe()
}

// setupOTPLoginExpectations sets up mock expectations for OTP login operations.
func (m *MockAuthService) setupOTPLoginExpectations(userID uint) {
	// Maybe expect pending deletion check
	m.userSvc.MockUserService.EXPECT().IsAccountPendingDeletion(mock.Anything, userID).Return(false, nil).Maybe()

	// Maybe expect account info update - use mock.Anything for time to avoid flaky tests
	m.userSvc.MockUserService.EXPECT().UpdateAccountInfo(mock.Anything, userID, mock.Anything).Return(nil).Maybe()
}

// ===== Private Methods =====

// hashPassword creates a simple hash for testing (in real scenario would use bcrypt)
func (m *MockAuthService) hashPassword(password string) string {
	return fmt.Sprintf("hashed_%s", password)
}

// validatePasswordHash validates a simple hash for testing
func (m *MockAuthService) validatePasswordHash(hash, password string) bool {
	return hash == fmt.Sprintf("hashed_%s", password)
}

// generateTestToken generates a real JWT token for testing
func (m *MockAuthService) generateTestToken(userID uint) string {
	token, err := m.jwtHelper.CreateLoginToken(userID)
	if err != nil {
		// Fallback to simple test token if JWT creation fails
		return fmt.Sprintf("test_token_%d", userID)
	}
	return token
}

// ===== Service Getters =====

// GetUserService returns embedded user service for advanced usage.
func (m *MockAuthService) GetUserService() *MockUserService {
	return m.userSvc
}

// ===== Component Implementation =====

// Config implements core.Component.
func (m *MockAuthService) Config() config.Manager {
	return m.componentConfig
}

// SetConfig implements core.Component.
func (m *MockAuthService) SetConfig(cfg config.Manager) {
	m.componentConfig = cfg
}

// Context implements core.Component.
func (m *MockAuthService) Context() core.Context {
	return m.ctx
}

// SetContext implements core.Component.
func (m *MockAuthService) SetContext(coreCtx core.Context) {
	if testCtx, ok := coreCtx.(TestContext); ok {
		m.ctx = testCtx
	}
}

// Logger implements core.Component.
func (m *MockAuthService) Logger() *core.Logger {
	return nil
}

// SetLogger implements core.Component.
func (m *MockAuthService) SetLogger(logger *core.Logger) {
}

// DB implements core.Component.
func (m *MockAuthService) DB() *gorm.DB {
	return nil
}

// SetDB implements core.Component.
func (m *MockAuthService) SetDB(db *gorm.DB) {
}
