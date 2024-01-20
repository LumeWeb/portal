package middleware

import (
	"context"
	"fmt"
	"git.lumeweb.com/LumeWeb/portal/interfaces"
	"github.com/golang-jwt/jwt/v5"
	"go.sia.tech/jape"
	"net/http"
	"strings"
)

const (
	S5AuthUserIDKey  = "userID"
	S5AuthCookieName = "s5-auth-token"
	S5AuthQueryParam = "auth_token"
)

func findAuthToken(r *http.Request) string {
	authHeader := parseAuthTokenHeader(r.Header)

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

func parseAuthTokenHeader(headers http.Header) string {
	authHeader := headers.Get("Authorization")
	if authHeader == "" {
		return ""
	}

	authHeader = strings.TrimPrefix(authHeader, "Bearer ")

	return authHeader
}

func AuthMiddleware(handler jape.Handler, portal interfaces.Portal) jape.Handler {
	return jape.Adapt(func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authToken := findAuthToken(r)

			if authToken == "" {
				http.Error(w, "Invalid JWT", http.StatusUnauthorized)
				return
			}

			token, err := jwt.Parse(authToken, func(token *jwt.Token) (interface{}, error) {
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

				exists, _ := portal.Accounts().AccountExists(userID)

				if !exists {
					http.Error(w, "Invalid User ID", http.StatusBadRequest)
					return
				}

				ctx := context.WithValue(r.Context(), S5AuthUserIDKey, userID)
				r = r.WithContext(ctx)

				h.ServeHTTP(w, r)
			} else {
				http.Error(w, "Invalid JWT", http.StatusUnauthorized)
			}
		})
	})(handler)
}

type tusJwtResponseWriter struct {
	http.ResponseWriter
}

func (w *tusJwtResponseWriter) WriteHeader(statusCode int) {
	// Check if this is the specific route and status
	if statusCode == http.StatusCreated {
		location := w.Header().Get("Location")
		if location != "" {
			w.
				authHeader := w.Header().Get("Authorization")
		}
	}
	w.ResponseWriter.WriteHeader(statusCode)
}

func BuildS5TusApi(portal interfaces.Portal) jape.Handler {

	// Wrapper function for AuthMiddleware to fit the MiddlewareFunc signature
	authMiddlewareFunc := func(h jape.Handler) jape.Handler {
		return AuthMiddleware(h, portal)
	}

	// Create a jape.Handler for your tusHandler
	tusJapeHandler := func(c jape.Context) {
		tusHandler := portal.Storage().Tus()
		tusHandler.ServeHTTP(c.ResponseWriter, c.Request)
	}

	protocolMiddleware := jape.Adapt(func(h http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), "protocol", "s5")
			h.ServeHTTP(w, r.WithContext(ctx))
		})
	})

	stripPrefix := func(h jape.Handler) jape.Handler {
		return jape.Adapt(func(h http.Handler) http.Handler {
			return http.StripPrefix("/s5/upload/tus", h)
		})(h)
	}

	injectJwt := func(h jape.Handler) jape.Handler {
		return jape.Adapt(func(h http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				res := w
				if r.Method == http.MethodPost && r.URL.Path == "/s5/upload/tus" {
					res = &tusJwtResponseWriter{ResponseWriter: w}
				}

				h.ServeHTTP(res, r)
			})
		})(h)
	}

	// Apply the middlewares to the tusJapeHandler
	tusHandler := ApplyMiddlewares(tusJapeHandler, injectJwt, stripPrefix, authMiddlewareFunc, protocolMiddleware)

	return tusHandler
}
