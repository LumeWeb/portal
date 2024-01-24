package middleware

import (
	"context"
	"fmt"
	"git.lumeweb.com/LumeWeb/portal/interfaces"
	"github.com/golang-jwt/jwt/v5"
	"go.sia.tech/jape"
	"net/http"
	"net/url"
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

func AuthMiddleware(portal interfaces.Portal) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
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

				next.ServeHTTP(w, r)
			} else {
				http.Error(w, "Invalid JWT", http.StatusUnauthorized)
			}
		})
	}
}

type tusJwtResponseWriter struct {
	http.ResponseWriter
	req *http.Request
}

func (w *tusJwtResponseWriter) WriteHeader(statusCode int) {
	// Check if this is the specific route and status
	if statusCode == http.StatusCreated {
		location := w.Header().Get("Location")
		authToken := parseAuthTokenHeader(w.req.Header)

		if authToken != "" && location != "" {

			parsedUrl, _ := url.Parse(location)

			query := parsedUrl.Query()
			query.Set("auth_token", authToken)
			parsedUrl.RawQuery = query.Encode()

			w.Header().Set("Location", parsedUrl.String())
		}
	}

	w.ResponseWriter.WriteHeader(statusCode)
}

func replacePrefix(prefix string, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, prefix)
		rp := strings.TrimPrefix(r.URL.RawPath, prefix)
		if len(p) < len(r.URL.Path) && (r.URL.RawPath == "" || len(rp) < len(r.URL.RawPath)) {
			/*			if len(p) == 0 {
							p = "/"
						}
						if len(rp) == 0 {
							rp = "/"
						}*/
			r2 := new(http.Request)
			*r2 = *r
			r2.URL = new(url.URL)
			*r2.URL = *r.URL
			r2.URL.Path = p
			r2.URL.RawPath = rp
			h.ServeHTTP(w, r2)
		} else {
			http.NotFound(w, r)
		}
	})
}

func BuildS5TusApi(portal interfaces.Portal) jape.Handler {
	// Create a jape.Handler for your tusHandler
	tusJapeHandler := func(c jape.Context) {
		tusHandler := portal.Storage().Tus()
		tusHandler.ServeHTTP(c.ResponseWriter, c.Request)
	}

	protocolMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), "protocol", "s5")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	stripPrefix := func(next http.Handler) http.Handler {
		return replacePrefix("/s5/upload/tus", next)
	}

	injectJwt := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			res := w
			if r.Method == http.MethodPost && r.URL.Path == "/s5/upload/tus" {
				res = &tusJwtResponseWriter{ResponseWriter: w, req: r}
			}

			next.ServeHTTP(res, r)
		})
	}

	// Apply the middlewares to the tusJapeHandler
	tusHandler := ApplyMiddlewares(tusJapeHandler, AuthMiddleware(portal), injectJwt, protocolMiddleware, stripPrefix, proxyMiddleware)

	return tusHandler
}
