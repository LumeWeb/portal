package middleware

import (
	"git.lumeweb.com/LumeWeb/portal/service/account"
	"git.lumeweb.com/LumeWeb/portal/service/auth"
	"github.com/kataras/iris/v12"
)

func VerifyJwt(ctx iris.Context) {
	token := auth.GetRequestAuthCode(ctx)

	if len(token) == 0 {
		ctx.StopWithError(iris.StatusUnauthorized, auth.ErrInvalidToken)
		return
	}

	acct, err := auth.VerifyLoginToken(token)

	if err != nil {
		ctx.StopWithError(iris.StatusUnauthorized, auth.ErrInvalidToken)
		return
	}

	err = ctx.SetUser(account.NewUser(acct))
	if err != nil {
		ctx.StopWithError(iris.StatusInternalServerError, err)
	}
}
