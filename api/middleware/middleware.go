package middleware

import (
	"go.sia.tech/jape"
	"net/http"
	"strings"
)

type JapeMiddlewareFunc func(jape.Handler) jape.Handler
type HttpMiddlewareFunc func(http.Handler) http.Handler

func AdaptMiddleware(mid func(http.Handler) http.Handler) JapeMiddlewareFunc {
	return jape.Adapt(func(h http.Handler) http.Handler {
		handler := mid(h)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handler.ServeHTTP(w, r)
		})
	})
}

// proxyMiddleware creates a new HTTP middleware for handling X-Forwarded-For headers.
func proxyMiddleware(next http.Handler) http.Handler {
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
		case HttpMiddlewareFunc:
			mid := middlewares[i].(HttpMiddlewareFunc)
			handler = AdaptMiddleware(mid)(handler)

		default:
			panic("Invalid middleware type")
		}
	}
	return handler
}
