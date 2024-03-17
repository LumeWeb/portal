package account

import (
	"net/http"

	"git.lumeweb.com/LumeWeb/portal/account"

	"git.lumeweb.com/LumeWeb/portal/api/middleware"
)

const (
	authCookieName = account.AUTH_COOKIE_NAME
	authQueryParam = "auth_token"
)

func findToken(r *http.Request) string {
	return middleware.FindAuthToken(r, authCookieName, authQueryParam)
}

func authMiddleware(options middleware.AuthMiddlewareOptions) middleware.HttpMiddlewareFunc {
	options.FindToken = findToken
	return middleware.AuthMiddleware(options)
}
