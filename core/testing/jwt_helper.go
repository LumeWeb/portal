package testing

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	gjwt "github.com/golang-jwt/jwt/v5"
	"go.lumeweb.com/portal-middleware/auth/jwt"
	"go.lumeweb.com/portal/config"
)

// JWTHelper provides high-level helper functions for JWT token testing.
// It creates and validates JWT tokens using the real JWT library but with test-friendly interfaces.
type JWTHelper struct {
	ctx    TestContext
	config config.Manager
	domain string
}

// NewJWTHelper creates a new JWTHelper.
func NewJWTHelper(ctx TestContext) *JWTHelper {
	cfg := ctx.Config()
	domain := cfg.Config().Core.Domain

	return &JWTHelper{
		ctx:    ctx,
		config: cfg,
		domain: domain,
	}
}

// CreateToken creates a JWT token for testing.
func (h *JWTHelper) CreateToken(userID uint, purpose jwt.Purpose) (string, error) {
	return jwt.CreateToken(h.config.Config().Core.Identity.PrivateKey(), h.domain, strconv.Itoa(int(userID)), purpose, time.Hour)
}

// CreateLoginToken creates a login JWT token.
func (h *JWTHelper) CreateLoginToken(userID uint) (string, error) {
	return h.CreateToken(userID, jwt.PurposeLogin)
}

// Create2FAToken creates a 2FA JWT token.
func (h *JWTHelper) Create2FAToken(userID uint) (string, error) {
	return h.CreateToken(userID, jwt.Purpose2FA)
}

// CreateAPIKeyToken creates an API key JWT token.
func (h *JWTHelper) CreateAPIKeyToken(userID uint) (string, error) {
	return h.CreateToken(userID, jwt.PurposeAPI)
}

// CreateLongLivedToken creates a long-lived JWT token.
func (h *JWTHelper) CreateLongLivedToken(userID uint, days int) (string, error) {
	duration := time.Hour * 24 * time.Duration(days)
	return jwt.CreateToken(h.config.Config().Core.Identity.PrivateKey(), h.domain, strconv.Itoa(int(userID)), jwt.PurposeLogin, duration)
}

// CreateExpiredToken creates an expired JWT token for testing.
func (h *JWTHelper) CreateExpiredToken(userID uint, purpose jwt.Purpose) (string, error) {
	duration := -time.Hour // Negative duration creates an expired token
	return jwt.CreateToken(h.config.Config().Core.Identity.PrivateKey(), h.domain, strconv.Itoa(int(userID)), purpose, duration)
}

// ValidateToken validates a JWT token.
func (h *JWTHelper) ValidateToken(token string, purpose ...jwt.Purpose) (*gjwt.RegisteredClaims, error) {
	// Default to login purpose if not provided
	expectedPurpose := jwt.PurposeLogin
	if len(purpose) > 0 {
		expectedPurpose = purpose[0]
	}
	
	claims, err := jwt.DecodeAndVerify(token, &gjwt.RegisteredClaims{}, h.domain, expectedPurpose)
	if err != nil {
		return nil, err
	}
	
	registeredClaims, ok := claims.(*gjwt.RegisteredClaims)
	if !ok {
		return nil, fmt.Errorf("unexpected claims type")
	}
	
	return registeredClaims, nil
}

// ValidateLoginToken validates a JWT token specifically for login purpose.
func (h *JWTHelper) ValidateLoginToken(token string) (*gjwt.RegisteredClaims, error) {
	return h.ValidateToken(token, jwt.PurposeLogin)
}

// ValidateAPIToken validates a JWT token specifically for API purpose.
func (h *JWTHelper) ValidateAPIToken(token string) (*gjwt.RegisteredClaims, error) {
	return h.ValidateToken(token, jwt.PurposeAPI)
}

// Validate2FAToken validates a JWT token specifically for 2FA purpose.
func (h *JWTHelper) Validate2FAToken(token string) (*gjwt.RegisteredClaims, error) {
	return h.ValidateToken(token, jwt.Purpose2FA)
}

// IsTokenValid checks if a JWT token is valid.
func (h *JWTHelper) IsTokenValid(token string) bool {
	_, err := h.ValidateToken(token)
	return err == nil
}

// IsTokenExpired checks if a JWT token is expired.
func (h *JWTHelper) IsTokenExpired(token string) bool {
	claims, err := h.ValidateToken(token)
	if err != nil {
		return errors.Is(err, gjwt.ErrTokenExpired)
	}
	if claims.ExpiresAt == nil {
		return false
	}
	return claims.ExpiresAt.Time.Before(time.Now())
}

// GetTokenUserID extracts user ID from JWT token.
func (h *JWTHelper) GetTokenUserID(token string) (uint, error) {
	claims, err := h.ValidateToken(token)
	if err != nil {
		return 0, err
	}

	userID, err := strconv.ParseUint(claims.Subject, 10, 32)
	if err != nil {
		return 0, err
	}

	return uint(userID), nil
}

// GetTokenPurpose extracts purpose from JWT token.
func (h *JWTHelper) GetTokenPurpose(token string) (jwt.Purpose, error) {
	claims, err := h.ValidateToken(token)
	if err != nil {
		return "", err
	}

	// Purpose is stored in the Audience field
	if len(claims.Audience) > 0 {
		return jwt.Purpose(claims.Audience[0]), nil
	}

	return jwt.PurposeNone, nil
}

// GetTokenExpiration extracts expiration time from JWT token.
func (h *JWTHelper) GetTokenExpiration(token string) (time.Time, error) {
	claims, err := h.ValidateToken(token)
	if err != nil {
		return time.Time{}, err
	}

	if claims.ExpiresAt == nil {
		return time.Time{}, fmt.Errorf("token has no expiration")
	}

	return claims.ExpiresAt.Time, nil
}

// CreateTokenPair creates a pair of JWT tokens (login and 2FA) for testing.
func (h *JWTHelper) CreateTokenPair(userID uint) (string, string, error) {
	loginToken, err := h.CreateLoginToken(userID)
	if err != nil {
		return "", "", err
	}

	twoFactorToken, err := h.Create2FAToken(userID)
	if err != nil {
		return "", "", err
	}

	return loginToken, twoFactorToken, nil
}

// CreateMultipleTokens creates multiple JWT tokens for the same user.
func (h *JWTHelper) CreateMultipleTokens(userID uint, count int, purpose jwt.Purpose) ([]string, error) {
	tokens := make([]string, 0, count)

	for i := 0; i < count; i++ {
		token, err := h.CreateToken(userID, purpose)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, token)
	}

	return tokens, nil
}

// CreateTokensForMultipleUsers creates JWT tokens for multiple users.
func (h *JWTHelper) CreateTokensForMultipleUsers(userIDs []uint, purpose jwt.Purpose) (map[uint]string, error) {
	tokens := make(map[uint]string)

	for _, userID := range userIDs {
		token, err := h.CreateToken(userID, purpose)
		if err != nil {
			return nil, err
		}
		tokens[userID] = token
	}

	return tokens, nil
}

// SimulateTokenRefresh simulates a token refresh by creating a new token.
func (h *JWTHelper) SimulateTokenRefresh(oldToken string) (string, error) {
	userID, err := h.GetTokenUserID(oldToken)
	if err != nil {
		return "", err
	}

	purpose, err := h.GetTokenPurpose(oldToken)
	if err != nil {
		return "", err
	}

	return h.CreateToken(userID, purpose)
}

// GenerateTestToken generates a simple test token (not a real JWT).
// Useful for tests that don't need actual JWT validation.
func (h *JWTHelper) GenerateTestToken(userID uint) string {
	return fmt.Sprintf("test_token_%d", userID)
}

// GenerateExpiredTestToken generates a simple expired test token.
// Useful for tests that need to simulate expired tokens.
func (h *JWTHelper) GenerateExpiredTestToken(userID uint) string {
	return fmt.Sprintf("expired_token_%d", userID)
}

// GenerateInvalidTestToken generates an invalid test token.
// Useful for tests that need to simulate invalid tokens.
func (h *JWTHelper) GenerateInvalidTestToken() string {
	return "invalid_test_token"
}