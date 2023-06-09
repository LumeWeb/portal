package controller

import (
	"git.lumeweb.com/LumeWeb/portal/controller/request"
	"git.lumeweb.com/LumeWeb/portal/service/account"
	"github.com/kataras/iris/v12"
)

type AccountController struct {
	Controller
}

func (a *AccountController) PostRegister() {
	ri, success := tryParseRequest(request.RegisterRequest{}, a.Ctx)
	if !success {
		return
	}

	r, _ := ri.(*request.RegisterRequest)

	err := account.Register(r.Email, r.Password, r.Pubkey)

	if err != nil {
		if err == account.ErrQueryingAcct || err == account.ErrFailedCreateAccount {
			a.Ctx.StopWithError(iris.StatusInternalServerError, err)
		} else {
			a.Ctx.StopWithError(iris.StatusBadRequest, err)
		}

		return
	}

	// Return a success response to the client.
	a.Ctx.StatusCode(iris.StatusCreated)
}
