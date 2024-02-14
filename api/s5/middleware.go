package s5

import (
	"git.lumeweb.com/LumeWeb/portal/api/middleware"
	"net/http"
)

const (
	authCookieName = "s5-auth-token"
	authQueryParam = "auth_token"
)

func findToken(r *http.Request) string {
	return middleware.FindAuthToken(r, authCookieName, authQueryParam)
}

func authMiddleware(options middleware.AuthMiddlewareOptions) middleware.HttpMiddlewareFunc {
	options.FindToken = findToken
	return middleware.AuthMiddleware(options)
}
