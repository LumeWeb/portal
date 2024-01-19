package middleware

import (
	"go.sia.tech/jape"
)

type MiddlewareFunc func(jape.Handler) jape.Handler

func ApplyMiddlewares(handler jape.Handler, middlewares ...MiddlewareFunc) jape.Handler {
	// Apply each middleware in reverse order
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}
