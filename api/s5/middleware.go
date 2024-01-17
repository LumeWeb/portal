package s5

import (
	"context"
	"fmt"
	"git.lumeweb.com/LumeWeb/portal/interfaces"
	"github.com/golang-jwt/jwt/v5"
	"go.sia.tech/jape"
	"net/http"
	"strconv"
	"strings"
)

const (
	AuthUserIDKey = "userID"
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

				publicKey := portal.Identity().Public()

				return publicKey, nil
			})

			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			if claim, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
				subject, err := claim.GetSubject()
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}

				userID, err := strconv.ParseUint(subject, 10, 64)
				if err != nil {
					http.Error(w, "Invalid User ID", http.StatusBadRequest)
					return
				}

				exists, _ := portal.Accounts().AccountExists(userID)

				if !exists {
					http.Error(w, "Invalid User ID", http.StatusBadRequest)
					return
				}

				ctx := context.WithValue(r.Context(), "userId", userID)
				r = r.WithContext(ctx)

				h.ServeHTTP(w, r)
			} else {
				http.Error(w, "Invalid JWT", http.StatusUnauthorized)
			}
		})
	})(handler)
}
