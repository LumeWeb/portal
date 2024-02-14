package account

import (
	"errors"
	"git.lumeweb.com/LumeWeb/portal/account"
	"go.sia.tech/jape"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"net/http"
)

var (
	errInvalidLogin          = errors.New("invalid login")
	errFailedToCreateAccount = errors.New("failed to create account")
)

type HttpHandler struct {
	accounts *account.AccountServiceDefault
	logger   *zap.Logger
}

type HttpHandlerParams struct {
	fx.In
	Accounts *account.AccountServiceDefault
	Logger   *zap.Logger
}

func NewHttpHandler(params HttpHandlerParams) *HttpHandler {
	return &HttpHandler{
		accounts: params.Accounts,
		logger:   params.Logger,
	}
}

func (h *HttpHandler) login(jc jape.Context) {
	var request LoginRequest

	if jc.Decode(&request) != nil {
		return
	}

	exists, _, err := h.accounts.EmailExists(request.Email)

	if !exists {
		_ = jc.Error(errInvalidLogin, http.StatusUnauthorized)
		if err != nil {
			h.logger.Error("failed to check if email exists", zap.Error(err))
		}
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
