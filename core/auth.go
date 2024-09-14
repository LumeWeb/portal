package core

import (
	"crypto/rand"
	"go.lumeweb.com/portal/db/models"
)

const AUTH_SERVICE = "auth"

func GenerateSecurityToken() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
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
	LoginPassword(email string, password string, ip string, rememberMe bool) (string, *models.User, error)

	// LoginOTP authenticates a user with the provided user ID and OTP code.
	// It returns the generated JWT token if successful.
	LoginOTP(userId uint, code string) (string, error)

	// LoginPubkey authenticates a user with the provided public key.
	// It returns the generated JWT token if successful.
	LoginPubkey(pubkey string, ip string) (string, error)

	// LoginID authenticates a user with the provided user ID.
	// It returns the generated JWT token if successful.
	LoginID(id uint, ip string) (string, error)

	// ValidLoginByUserObj checks if the provided password is valid for the given user.
	ValidLoginByUserObj(user *models.User, password string) bool

	// ValidLoginByEmail checks if the provided email and password are valid.
	// It returns a boolean indicating success, the authenticated user, and an error if any.
	ValidLoginByEmail(email string, password string) (bool, *models.User, error)

	// ValidLoginByUserID checks if the provided user ID and password are valid.
	// It returns a boolean indicating success, the authenticated user, and an error if any.
	ValidLoginByUserID(id uint, password string) (bool, *models.User, error)

	Service
}
