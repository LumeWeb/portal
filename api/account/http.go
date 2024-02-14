package account

import (
	"errors"
	"git.lumeweb.com/LumeWeb/portal/account"
	"go.sia.tech/jape"
	"go.uber.org/fx"
	"net/http"
)

var (
	errInvalidLogin          = errors.New("invalid login")
	errFailedToCreateAccount = errors.New("failed to create account")
)

type HttpHandler struct {
	accounts *account.AccountServiceDefault
}

type HttpHandlerParams struct {
	fx.In
	Accounts *account.AccountServiceDefault
}

func NewHttpHandler(params HttpHandlerParams) *HttpHandler {
	return &HttpHandler{
		accounts: params.Accounts,
	}
}

func (h *HttpHandler) login(jc jape.Context) {
	var request LoginRequest

	if jc.Decode(&request) != nil {
		return
	}

	exists, _ := h.accounts.AccountExistsByEmail(request.Email)

	if !exists {
		_ = jc.Error(errInvalidLogin, http.StatusUnauthorized)
		return
	}

	jwt, _, err := h.accounts.LoginPassword(request.Email, request.Password)
	if err != nil {
		return
	}

	jc.ResponseWriter.Header().Set("Authorization", "Bearer "+jwt)
	jc.ResponseWriter.WriteHeader(http.StatusOK)
}

func (h *HttpHandler) register(jc jape.Context) {
	var request RegisterRequest

	if jc.Decode(&request) != nil {
		return
	}

	user, err := h.accounts.CreateAccount(request.Email, request.Password)
	if err != nil {
		_ = jc.Error(errFailedToCreateAccount, http.StatusBadRequest)
		return
	}

	err = h.accounts.UpdateAccountName(user.ID, request.FirstName, request.LastName)

	if err != nil {
		_ = jc.Error(errors.Join(errFailedToCreateAccount, err), http.StatusBadRequest)
		return
	}
}
