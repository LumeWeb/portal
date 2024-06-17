package middleware

import (
	"context"
	"errors"
	"github.com/golang-jwt/jwt/v5"
	"go.lumeweb.com/portal/core"
	"net/http"
	"strconv"
	"strings"
)

type AuthTokenContextKeyType string
type UserIdContextKeyType string

type FindAuthTokenFunc func(r *http.Request) string

func FindAuthToken(r *http.Request, cookieName string, queryParam string) string {
	authHeader := ParseAuthTokenHeader(r.Header)

	if authHeader != "" {
		return authHeader
	}

	if cookie, err := r.Cookie(cookieName); cookie != nil && err == nil {
		return cookie.Value
	}

	if cookie, err := r.Cookie(core.AUTH_COOKIE_NAME); cookie != nil && err == nil {
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
	Context        core.Context
	FindToken      FindAuthTokenFunc
	Purpose        core.JWTPurpose
	AuthContextKey string
	EmptyAllowed   bool
	ExpiredAllowed bool
}

func AuthMiddleware(options AuthMiddlewareOptions) func(http.Handler) http.Handler {
	config := options.Context.Config()

	if options.AuthContextKey == "" {
		options.AuthContextKey = string(DEFAULT_USER_ID_CONTEXT_KEY)
	}

	if options.FindToken == nil {
		options.FindToken = func(r *http.Request) string {
			return FindAuthToken(r, core.AUTH_COOKIE_NAME, core.AUTH_TOKEN_NAME)
		}
	}

	domain := config.Config().Core.Domain

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authToken := options.FindToken(r)

			if authToken == "" {
				if !options.EmptyAllowed {
					http.Error(w, "Invalid JWT", http.StatusUnauthorized)
					return
				}
				next.ServeHTTP(w, r)
				return
			}

			var audList *jwt.ClaimStrings

			claim, err := core.JWTVerifyToken(authToken, domain, options.Context.Config().Config().Core.Identity.PrivateKey(), func(claim *jwt.RegisteredClaims) error {
				aud, _ := claim.GetAudience()

				audList = &aud

				if options.Purpose != core.JWTPurposeNone && !jwtPurposeEqual(aud, options.Purpose) {
					return core.ErrJWTInvalid
				}

				return nil
			})

			if err != nil {
				unauthorized := true
				if errors.Is(err, jwt.ErrTokenExpired) && options.ExpiredAllowed {
					unauthorized = false
				}

				if !unauthorized && audList == nil {
					if audList == nil {
						var claim jwt.RegisteredClaims

						unverified, _, err := jwt.NewParser().ParseUnverified(authToken, &claim)
						if err != nil {
							http.Error(w, err.Error(), http.StatusInternalServerError)
							return
						}

						audList, err := unverified.Claims.GetAudience()
						if err != nil {
							http.Error(w, err.Error(), http.StatusInternalServerError)
							return
						}

						if jwtPurposeEqual(audList, options.Purpose) {
							unauthorized = true
						}

					}
				}

				if unauthorized {
					http.Error(w, err.Error(), http.StatusUnauthorized)
					return
				}
			}

			if claim == nil && options.ExpiredAllowed {
				next.ServeHTTP(w, r)
				return
			}

			userId, err := strconv.ParseUint(claim.Subject, 10, 64)

			if err != nil {
				http.Error(w, core.ErrJWTInvalid.Error(), http.StatusBadRequest)
				return
			}

			exists, _, err := options.Context.Service(core.USER_SERVICE).(core.UserService).AccountExists(uint(userId))

			if !exists || err != nil {
				http.Error(w, core.ErrJWTInvalid.Error(), http.StatusBadRequest)
				return
			}

			ctx := context.WithValue(r.Context(), UserIdContextKeyType(options.AuthContextKey), uint(userId))
			ctx = context.WithValue(ctx, AUTH_TOKEN_CONTEXT_KEY, authToken)
			r = r.WithContext(ctx)

			next.ServeHTTP(w, r)
		})
	}
}
