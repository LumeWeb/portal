package middleware

import (
	"git.lumeweb.com/LumeWeb/portal/service/auth"
	"github.com/kataras/iris/v12"
)

func VerifyJwt(ctx iris.Context) {
	token := auth.GetRequestAuthCode(ctx)

	if len(token) == 0 {
		ctx.StopWithError(iris.StatusUnauthorized, auth.ErrInvalidToken)
		return
	}

	if err := auth.VerifyLoginToken(token); err != nil {
		ctx.StopWithError(iris.StatusUnauthorized, auth.ErrInvalidToken)
		return
	}

	ctx.Next()
}
