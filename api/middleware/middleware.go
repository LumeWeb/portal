package middleware

import (
	"context"
	"crypto/ed25519"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"git.lumeweb.com/LumeWeb/portal/account"
	"git.lumeweb.com/LumeWeb/portal/api/registry"
	"github.com/golang-jwt/jwt/v5"
	"github.com/julienschmidt/httprouter"
	"github.com/spf13/viper"
	"go.sia.tech/jape"
)

const DEFAULT_AUTH_CONTEXT_KEY = "user_id"

type JapeMiddlewareFunc func(jape.Handler) jape.Handler
type HttpMiddlewareFunc func(http.Handler) http.Handler

type FindAuthTokenFunc func(r *http.Request) string

func AdaptMiddleware(mid func(http.Handler) http.Handler) JapeMiddlewareFunc {
	return jape.Adapt(func(h http.Handler) http.Handler {
		handler := mid(h)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handler.ServeHTTP(w, r)
		})
	})
}

// ProxyMiddleware creates a new HTTP middleware for handling X-Forwarded-For headers.
func ProxyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ips := strings.Split(xff, ", ")
			if len(ips) > 0 {
				r.RemoteAddr = ips[0]
			}
		}
		next.ServeHTTP(w, r)
	})
}

func ApplyMiddlewares(handler jape.Handler, middlewares ...interface{}) jape.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		switch middlewares[i].(type) {
		case JapeMiddlewareFunc:
			mid := middlewares[i].(JapeMiddlewareFunc)
			handler = mid(handler)
		case func(http.Handler) http.Handler:
			mid := middlewares[i].(func(http.Handler) http.Handler)
			handler = AdaptMiddleware(mid)(handler)
		case HttpMiddlewareFunc:
			mid := middlewares[i].(HttpMiddlewareFunc)
			handler = AdaptMiddleware(mid)(handler)

		default:
			panic("Invalid middleware type")
		}
	}
	return handler
}
func RegisterProtocolSubdomain(config *viper.Viper, mux *httprouter.Router, name string) {
	router := registry.GetRouter()
	domain := config.GetString("core.domain")

	(router)[name+"."+domain] = mux
}

func FindAuthToken(r *http.Request, cookieName string, queryParam string) string {
	authHeader := ParseAuthTokenHeader(r.Header)

	if authHeader != "" {
		return authHeader
	}

	if cookie, err := r.Cookie(cookieName); cookie != nil && err == nil {
		return cookie.Value
	}

	return r.FormValue(queryParam)
}

func ParseAuthTokenHeader(headers http.Header) string {
	authHeader := headers.Get("Authorization")
	if authHeader == "" {
		return ""
	}

	authHeader = strings.TrimPrefix(authHeader, "Bearer ")
	authHeader = strings.TrimPrefix(authHeader, "bearer ")

	return authHeader
}

type AuthMiddlewareOptions struct {
	Identity       ed25519.PrivateKey
	Accounts       *account.AccountServiceDefault
	FindToken      FindAuthTokenFunc
	Purpose        account.JWTPurpose
	AuthContextKey string
	Config         *viper.Viper
}

func AuthMiddleware(options AuthMiddlewareOptions) func(http.Handler) http.Handler {
	if options.AuthContextKey == "" {
		options.AuthContextKey = DEFAULT_AUTH_CONTEXT_KEY
	}
	if options.Purpose == "" {
		panic("purpose is missing")
	}

	domain := options.Config.GetString("core.domain")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authToken := options.FindToken(r)

			if authToken == "" {
				http.Error(w, "Invalid JWT", http.StatusUnauthorized)
				return
			}

			claim, err := account.JWTVerifyToken(authToken, domain, options.Identity, func(claim *jwt.RegisteredClaims) error {
				aud, _ := claim.GetAudience()

				if slices.Contains[jwt.ClaimStrings, string](aud, string(options.Purpose)) == false {
					return account.ErrJWTInvalid
				}

				return nil
			})

			if err != nil {
				http.Error(w, err.Error(), http.StatusUnauthorized)
				return
			}

			userId, err := strconv.ParseUint(claim.Subject, 10, 64)

			if err != nil {
				http.Error(w, account.ErrJWTInvalid.Error(), http.StatusBadRequest)
				return
			}

			exists, _, err := options.Accounts.AccountExists(uint(userId))

			if !exists || err != nil {
				http.Error(w, account.ErrJWTInvalid.Error(), http.StatusBadRequest)
				return
			}

			ctx := context.WithValue(r.Context(), options.AuthContextKey, uint(userId))
			r = r.WithContext(ctx)

			next.ServeHTTP(w, r)
		})
	}
}

func MergeRoutes(routes ...map[string]jape.Handler) map[string]jape.Handler {
	merged := make(map[string]jape.Handler)

	for _, route := range routes {
		for k, v := range route {
			merged[k] = v
		}
	}

	return merged
}

func GetUserFromContext(ctx context.Context, key ...string) uint {
	realKey := ""

	if len(key) > 0 {
		realKey = key[0]
	}

	if realKey == "" {
		realKey = DEFAULT_AUTH_CONTEXT_KEY
	}

	userId, ok := ctx.Value(realKey).(uint)

	if !ok {
		panic("user id stored in context is not of type uint")
	}

	return userId
}
