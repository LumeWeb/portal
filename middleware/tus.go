package middleware

import (
	"github.com/gorilla/mux"
	"github.com/rs/cors"
	"go.lumeweb.com/portal/core"
	"net/http"
	"net/url"
	"strings"
)

type tusJwtResponseWriter struct {
	http.ResponseWriter
	req *http.Request
}

func (w *tusJwtResponseWriter) WriteHeader(statusCode int) {
	// Check if this is the specific route and status
	if statusCode == http.StatusCreated {
		location := w.Header().Get("Location")
		authToken := ParseAuthTokenHeader(w.req.Header)

		if authToken != "" && location != "" {
			parsedUrl, _ := url.Parse(location)

			query := parsedUrl.Query()
			query.Set(core.AUTH_TOKEN_NAME, authToken)
			parsedUrl.RawQuery = query.Encode()

			w.Header().Set("Location", parsedUrl.String())
		}
	}

	w.ResponseWriter.WriteHeader(statusCode)
}

func TusPathMiddleware(basePath string) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, basePath) {
				// Strip prefix
				r.URL.Path = strings.TrimPrefix(r.URL.Path, basePath)

				// Inject JWT
				res := w
				if r.Method == http.MethodPost && r.URL.Path == "" {
					res = &tusJwtResponseWriter{ResponseWriter: w, req: r}
				}

				next.ServeHTTP(res, r)
			} else {
				next.ServeHTTP(w, r)
			}
		})
	}
}

func TusCorsMiddleware() func(h http.Handler) http.Handler {
	return cors.New(cors.Options{
		AllowOriginFunc: func(origin string) bool {
			return true
		},
		AllowedMethods: []string{"GET", "POST", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		AllowedHeaders: []string{
			"Authorization",
			"Expires",
			"Upload-Concat",
			"Upload-Length",
			"Upload-Metadata",
			"Upload-Offset",
			"X-Requested-With",
			"Tus-Version",
			"Tus-Resumable",
			"Tus-Extension",
			"Tus-Max-Size",
			"X-HTTP-Method-Override",
			"Content-Type",
		},
		AllowCredentials: true,
	}).Handler
}
