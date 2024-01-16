package account

import (
	"crypto/ed25519"
	"github.com/golang-jwt/jwt/v5"
	"time"
)

func generateToken(privateKey ed25519.PrivateKey, userID uint) (string, error) {
	// Define the claims
	claims := jwt.MapClaims{
		"iss": "portal",
		"sub": userID,
		"exp": time.Now().Add(time.Hour * 24).Unix(),
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
