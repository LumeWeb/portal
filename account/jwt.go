package account

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/samber/lo"

	"go.sia.tech/jape"

	"git.lumeweb.com/LumeWeb/portal/api/router"

	apiRegistry "git.lumeweb.com/LumeWeb/portal/api/registry"

	"github.com/golang-jwt/jwt/v5"
)

const AUTH_COOKIE_NAME = "auth_token"

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

func SetAuthCookie(jc jape.Context, jwt string, apiName string) {
	for name, api := range apiRegistry.GetAllAPIs() {
		routeableApi, ok := api.(router.RoutableAPI)
		if !ok {
			continue
		}

		if len(apiName) > 0 && apiName != name {
			continue
		}

		http.SetCookie(jc.ResponseWriter, &http.Cookie{
			Name:     routeableApi.AuthTokenName(),
			Value:    jwt,
			Expires:  time.Now().Add(24 * time.Hour),
			Secure:   true,
			HttpOnly: true,
			Path:     "/",
			Domain:   routeableApi.Domain(),
		})
	}
}

func EchoAuthCookie(jc jape.Context, apiName string) {
	for name, api := range apiRegistry.GetAllAPIs() {
		routeableApi, ok := api.(router.RoutableAPI)
		if !ok {
			continue
		}

		if len(apiName) > 0 && apiName != name {
			continue
		}

		cookies := lo.Filter(jc.Request.Cookies(), func(item *http.Cookie, _ int) bool {
			return item.Name == routeableApi.AuthTokenName()
		})

		if len(cookies) == 0 {
			continue
		}

		http.SetCookie(jc.ResponseWriter, &http.Cookie{
			Name:     cookies[0].Name,
			Value:    cookies[0].Value,
			Expires:  cookies[0].Expires,
			Secure:   true,
			HttpOnly: true,
			Path:     "/",
			Domain:   cookies[0].Domain,
		})
	}
}

func ClearAuthCookie(jc jape.Context, apiName string) {
	for name, api := range apiRegistry.GetAllAPIs() {
		routeableApi, ok := api.(router.RoutableAPI)
		if !ok {
			continue
		}

		if len(apiName) > 0 && apiName != name {
			continue
		}

		jc.ResponseWriter.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		jc.ResponseWriter.Header().Set("Pragma", "no-cache")
		jc.ResponseWriter.Header().Set("Expires", "0")

		http.SetCookie(jc.ResponseWriter, &http.Cookie{
			Name:     routeableApi.AuthTokenName(),
			Value:    "",
			Expires:  time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC),
			Secure:   true,
			HttpOnly: true,
			Path:     "/",
			Domain:   routeableApi.Domain(),
		})

	}
}
