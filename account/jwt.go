package account

import (
	"crypto/ed25519"
	"github.com/golang-jwt/jwt/v5"
	"time"
)

type JWTPurpose string

const (
	JWTPurposeLogin JWTPurpose = "login"
)

func GenerateToken(domain string, privateKey ed25519.PrivateKey, userID uint, purpose JWTPurpose) (string, error) {
	return GenerateTokenWithDuration(domain, privateKey, userID, time.Hour*24, purpose)
}

func GenerateTokenWithDuration(domain string, privateKey ed25519.PrivateKey, userID uint, duration time.Duration, purpose JWTPurpose) (string, error) {
	// Define the claims
	claims := jwt.MapClaims{
		"iss": domain,
		"sub": userID,
		"exp": time.Now().Add(duration).Unix(),
		"iat": time.Now().Unix(),
		"aud": string(purpose),
	}

	// Create the token
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)

	// Sign the token with the Ed25519 private key
	tokenString, err := token.SignedString(privateKey)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}
