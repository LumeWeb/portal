package middleware

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"git.lumeweb.com/LumeWeb/portal/account"
	"github.com/golang-jwt/jwt/v5"
	"net/http"
	"strings"
)

const (
	S5AuthUserIDKey  = "userID"
	S5AuthCookieName = "s5-auth-token"
	S5AuthQueryParam = "auth_token"
)

func FindAuthToken(r *http.Request) string {
	authHeader := ParseAuthTokenHeader(r.Header)

	if authHeader != "" {
		return authHeader
	}

	for _, cookie := range r.Cookies() {
		if cookie.Name == S5AuthCookieName {
			return cookie.Value
		}
	}

	return r.FormValue(S5AuthQueryParam)
}

func ParseAuthTokenHeader(headers http.Header) string {
	authHeader := headers.Get("Authorization")
	if authHeader == "" {
		return ""
	}

	authHeader = strings.TrimPrefix(authHeader, "Bearer ")

	return authHeader
}

func AuthMiddleware(identity ed25519.PrivateKey, accounts *account.AccountServiceDefault) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authToken := FindAuthToken(r)

			if authToken == "" {
				http.Error(w, "Invalid JWT", http.StatusUnauthorized)
				return
			}

			token, err := jwt.Parse(authToken, func(token *jwt.Token) (interface{}, error) {
				if _, ok := token.Method.(*jwt.SigningMethodEd25519); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
				}

				publicKey := identity.Public()

				return publicKey, nil
			})

			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			if claim, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
				subject, ok := claim["sub"]

				if !ok {
					http.Error(w, "Invalid User ID", http.StatusBadRequest)
					return
				}

				var userID uint64

				switch v := subject.(type) {
				case uint64:
					userID = v
				case float64:
					userID = uint64(v)
				default:
					// Handle the case where userID is of an unexpected type
					http.Error(w, "Invalid User ID", http.StatusBadRequest)
					return
				}

				exists, _ := accounts.AccountExists(userID)

				if !exists {
					http.Error(w, "Invalid User ID", http.StatusBadRequest)
					return
				}

				ctx := context.WithValue(r.Context(), S5AuthUserIDKey, userID)
				r = r.WithContext(ctx)

				next.ServeHTTP(w, r)
			} else {
				http.Error(w, "Invalid JWT", http.StatusUnauthorized)
			}
		})
	}
}
