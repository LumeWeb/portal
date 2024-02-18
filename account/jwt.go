package account

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type JWTPurpose string
type VerifyTokenFunc func(claim *jwt.RegisteredClaims) error

var (
	nopVerifyFunc VerifyTokenFunc = func(claim *jwt.RegisteredClaims) error {
		return nil
	}

	ErrJWTUnexpectedClaimsType = errors.New("unexpected claims type")
	ErrJWTUnexpectedIssuer     = errors.New("unexpected issuer")
	ErrJWTInvalid              = errors.New("invalid JWT")
)

const (
	JWTPurposeLogin JWTPurpose = "login"
	JWTPurpose2FA   JWTPurpose = "2fa"
)

func JWTGenerateToken(domain string, privateKey ed25519.PrivateKey, userID uint, purpose JWTPurpose) (string, error) {
	return JWTGenerateTokenWithDuration(domain, privateKey, userID, time.Hour*24, purpose)
}

func JWTGenerateTokenWithDuration(domain string, privateKey ed25519.PrivateKey, userID uint, duration time.Duration, purpose JWTPurpose) (string, error) {

	// Define the claims
	claims := jwt.RegisteredClaims{
		Issuer:    domain,
		Subject:   strconv.Itoa(int(userID)),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(duration)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		Audience:  []string{string(purpose)},
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

func JWTVerifyToken(token string, domain string, privateKey ed25519.PrivateKey, verifyFunc VerifyTokenFunc) (*jwt.RegisteredClaims, error) {
	validatedToken, err := jwt.ParseWithClaims(token, &jwt.RegisteredClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodEd25519); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}

		publicKey := privateKey.Public()

		return publicKey, nil
	})

	if err != nil {
		return nil, err
	}

	if verifyFunc == nil {
		verifyFunc = nopVerifyFunc
	}

	claim, ok := validatedToken.Claims.(*jwt.RegisteredClaims)

	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrJWTUnexpectedClaimsType, validatedToken.Claims)
	}

	if domain != claim.Issuer {
		return nil, fmt.Errorf("%w: %s", ErrJWTUnexpectedIssuer, claim.Issuer)
	}

	err = verifyFunc(claim)

	return nil, err
}
