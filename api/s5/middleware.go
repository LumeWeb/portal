package s5

import (
	"fmt"
	"git.lumeweb.com/LumeWeb/portal/interfaces"
	"github.com/golang-jwt/jwt/v5"
	"go.sia.tech/jape"
	"net/http"
	"strings"
)

func AuthMiddleware(handler jape.Handler, portal interfaces.Portal) jape.Handler {
	return jape.Adapt(func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "Authorization header is required", http.StatusBadRequest)
				return
			}

			tokenString := strings.TrimPrefix(authHeader, "Bearer ")

			token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
				if _, ok := token.Method.(*jwt.SigningMethodEd25519); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
				}

				publicKey := portal.Identity()

				return publicKey, nil
			})

			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			if _, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
				h.ServeHTTP(w, r)
			} else {
				http.Error(w, "Invalid JWT", http.StatusUnauthorized)
			}
		})
	})(handler)
}
