package core

import (
	"context"
	"crypto/rand"
	"fmt"
	"strings"

	"go.lumeweb.com/portal/db/models"
)

const AUTH_COOKIE_NAME = "auth_token"
const AUTH_TOKEN_NAME = "auth_token"

const AUTH_SERVICE = "auth"

// AnonEmailFormat is the format string for anonymous user emails generated
// from wallet addresses or other key identities. The %s is replaced with
// the lowercased key string (e.g., ETH address).
// Example: anon_0xabc123...@local.invalid
const AnonEmailFormat = "anon_%s@local.invalid"

// AnonEmail generates an anonymous email for a key string (e.g., wallet address).
// The key is lowercased for consistency.
func AnonEmail(key string) string {
	return fmt.Sprintf(AnonEmailFormat, strings.ToLower(key))
}

func GenerateSecurityToken() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, 6)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}
	for i := 0; i < 6; i++ {
		b[i] = charset[b[i]%byte(len(charset))]
	}
	return string(b)
}

type AuthService interface {
	// LoginPassword authenticates a user with the provided email and password.
	// It returns the generated JWT token and the authenticated user if successful.
	LoginPassword(ctx context.Context, email string, password string, ip string, rememberMe bool) (string, *models.User, error)

	// LoginOTP authenticates a user with the provided user ID and OTP code.
	// It returns the generated JWT token if successful.
	LoginOTP(ctx context.Context, userId uint, code string, rememberMe bool) (string, error)

	// LoginKeyIdentity authenticates a user with a typed key identity.
	// keyType is a registry key (e.g., "ethereum").
	// The key should already be normalized via the type's handler.
	// The proof must correspond to a challenge previously issued by
	// the handler's IssueChallenge method.
	LoginKeyIdentity(ctx context.Context, keyType string, key string, proof []byte, ip string, rememberMe bool) (string, *models.User, error)

	// LoginKeyIdentityWithContext is the preferred method for key identity
	// login, as it passes core.Context to the handler for challenge
	// verification. LoginKeyIdentity delegates to this.
	LoginKeyIdentityWithContext(ctx Context, keyType string, key string, proof []byte, ip string, rememberMe bool) (string, *models.User, error)

	// LoginID authenticates a user with the provided user ID.
	// It returns the generated JWT token if successful.
	LoginID(ctx context.Context, id uint, ip string, rememberMe bool) (string, error)

	// ValidLoginByUserObj checks if the provided password is valid for the given user.
	ValidLoginByUserObj(ctx context.Context, user *models.User, password string) bool

	// ValidLoginByEmail checks if the provided email and password are valid.
	// It returns a boolean indicating success, the authenticated user, and an error if any.
	ValidLoginByEmail(ctx context.Context, email string, password string) (bool, *models.User, error)

	// ValidLoginByUserID checks if the provided user ID and password are valid.
	// It returns a boolean indicating success, the authenticated user, and an error if any.
	ValidLoginByUserID(ctx context.Context, id uint, password string) (bool, *models.User, error)

	Service
}
