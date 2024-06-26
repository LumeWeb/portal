package core

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"github.com/golang-jwt/jwt/v5"
	"github.com/samber/lo"
	"net/http"
	"strconv"
	"time"
)

const AUTH_COOKIE_NAME = "auth_token"
const AUTH_TOKEN_NAME = "auth_token"

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
	JWTPurposeNone  JWTPurpose = ""
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

	return claim, err
}

func SetAuthCookie(w http.ResponseWriter, ctx Context, jwt string) {
	for _, api := range GetAPIs() {
		http.SetCookie(w, &http.Cookie{
			Name:     api.AuthTokenName(),
			Value:    jwt,
			MaxAge:   int((24 * time.Hour).Seconds()),
			Secure:   true,
			HttpOnly: true,
			Path:     "/",
			Domain:   ctx.Config().Config().Core.Domain,
		})
	}
}

func EchoAuthCookie(w http.ResponseWriter, r *http.Request, ctx Context) {
	for _, api := range GetAPIs() {
		cookies := lo.Filter(r.Cookies(), func(item *http.Cookie, _ int) bool {
			return item.Name == api.AuthTokenName()
		})

		if len(cookies) == 0 {
			continue
		}

		unverified, _, err := jwt.NewParser().ParseUnverified(cookies[0].Value, &jwt.RegisteredClaims{})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		exp, err := unverified.Claims.GetExpirationTime()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     cookies[0].Name,
			Value:    cookies[0].Value,
			MaxAge:   int(time.Until(exp.Time).Seconds()),
			Secure:   true,
			HttpOnly: true,
			Path:     "/",
			Domain:   ctx.Config().Config().Core.Domain,
		})
	}
}

func ClearAuthCookie(w http.ResponseWriter, ctx Context) {
	for _, api := range GetAPIs() {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")

		http.SetCookie(w, &http.Cookie{
			Name:     api.AuthTokenName(),
			Value:    "",
			Expires:  time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC),
			MaxAge:   -1,
			Secure:   true,
			HttpOnly: true,
			Path:     "/",
			Domain:   ctx.Config().Config().Core.Domain,
		})
	}
}
func SendJWT(w http.ResponseWriter, jwt string) {
	w.Header().Set("Authorization", "Bearer "+jwt)
}
