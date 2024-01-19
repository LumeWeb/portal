package middleware

import (
	"git.lumeweb.com/LumeWeb/portal/api"
	"go.sia.tech/jape"
)

func ApplyMiddlewares(handler jape.Handler, middlewares ...api.MiddlewareFunc) jape.Handler {
	// Apply each middleware in reverse order
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}
