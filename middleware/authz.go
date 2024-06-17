package middleware

import (
	"github.com/casbin/casbin/v2"
	"go.lumeweb.com/portal/core"
	"net/http"
)

type AuthzOptions struct {
	Context core.Context
	Casbin  *casbin.Enforcer
	Role    string
}

func AuthzMiddleware(options AuthzOptions) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := r.Context().Value(DEFAULT_USER_ID_CONTEXT_KEY)

			deny := func() {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
			}

			exists, m, err := options.Context.Service(core.USER_SERVICE).(core.UserService).AccountExists(user.(uint))
			if err != nil || !exists {
				deny()
				return
			}

			if options.Role != m.Role {
				deny()
				return
			}

			ok, err := options.Casbin.Enforce(m.Role, r.URL.Path, r.Method)
			if err != nil || !ok {
				deny()
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
