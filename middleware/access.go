package middleware

import (
	"go.lumeweb.com/portal/core"
	"net/http"
)

func AccessMiddleware(ctx core.Context) func(http.Handler) http.Handler {
	userService := ctx.Service(core.USER_SERVICE).(core.UserService)
	accessService := ctx.Service(core.ACCESS_SERVICE).(core.AccessService)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			deny := func() {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
			}

			user, err := GetUserFromContext(r.Context())
			if err != nil {
				deny()
				return
			}

			exists, m, err := userService.AccountExists(user)
			if err != nil || !exists {
				deny()
				return
			}

			ok, err := accessService.CheckAccess(m.ID, r.URL.Hostname(), r.Host, r.Method)
			if err != nil {
				deny()
				return
			}
			if err != nil || !ok {
				deny()
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
