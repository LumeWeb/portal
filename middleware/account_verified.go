package middleware

import (
	"go.lumeweb.com/portal/core"
	"net/http"
)

func AccountVerifiedMiddleware(ctx core.Context) func(http.Handler) http.Handler {
	userService := ctx.Service(core.USER_SERVICE).(core.UserService)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			user, err := GetUserFromContext(r.Context())
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			verified, err := userService.IsAccountVerified(user)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			if !verified {
				acctErr := core.NewAccountError(core.ErrKeyAccountNotVerified, nil)
				http.Error(w, acctErr.Error(), acctErr.HttpStatus())
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
