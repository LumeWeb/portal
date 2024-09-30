package middleware

import (
	"github.com/rs/cors"
	"net/http"
)

var defaultCorsOptions = cors.Options{
	AllowOriginFunc: func(origin string) bool {
		return true
	},
	AllowedMethods:   []string{"GET", "POST", "PATCH", "DELETE", "HEAD", "OPTIONS"},
	AllowedHeaders:   []string{"*"},
	AllowCredentials: true,
}

func CorsMiddleware(opts *cors.Options) func(h http.Handler) http.Handler {
	mergedOpts := defaultCorsOptions

	if opts != nil {
		if opts.AllowOriginFunc != nil {
			mergedOpts.AllowOriginFunc = opts.AllowOriginFunc
		}
		if len(opts.AllowedMethods) > 0 {
			mergedOpts.AllowedMethods = opts.AllowedMethods
		}
		if len(opts.AllowedHeaders) > 0 {
			mergedOpts.AllowedHeaders = opts.AllowedHeaders
		}
		if len(opts.ExposedHeaders) > 0 {
			mergedOpts.ExposedHeaders = opts.ExposedHeaders
		}
		if opts.AllowCredentials != mergedOpts.AllowCredentials {
			mergedOpts.AllowCredentials = opts.AllowCredentials
		}
		if opts.MaxAge > 0 {
			mergedOpts.MaxAge = opts.MaxAge
		}
	}

	return cors.New(mergedOpts).Handler
}
