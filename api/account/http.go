package account

import (
	"errors"
	"git.lumeweb.com/LumeWeb/portal/account"
	"git.lumeweb.com/LumeWeb/portal/api/middleware"
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

	jwt, _, err := h.accounts.LoginPassword(request.Email, request.Password, jc.Request.RemoteAddr)
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

func (h *HttpHandler) otpGenerate(jc jape.Context) {
	user := middleware.GetUserFromContext(jc.Request.Context())

	otp, err := h.accounts.OTPGenerate(user)
	if jc.Check("failed to generate otp", err) != nil {
		return
	}

	jc.Encode(&OTPGenerateResponse{
		OTP: otp,
	})
}

func (h *HttpHandler) otpVerify(jc jape.Context) {
	user := middleware.GetUserFromContext(jc.Request.Context())

	var request OTPVerifyRequest

	if jc.Decode(&request) != nil {
		return
	}

	err := h.accounts.OTPEnable(user, request.OTP)

	if jc.Check("failed to verify otp", err) != nil {
		return
	}
}

func (h *HttpHandler) otpValidate(jc jape.Context) {
	user := middleware.GetUserFromContext(jc.Request.Context())

	var request OTPValidateRequest

	if jc.Decode(&request) != nil {
		return
	}

	jwt, err := h.accounts.LoginOTP(user, request.OTP)
	if jc.Check("failed to validate otp", err) != nil {
		return
	}

	account.SendJWT(jc, jwt)
}

func (h *HttpHandler) otpDisable(jc jape.Context) {
	user := middleware.GetUserFromContext(jc.Request.Context())

	var request OTPDisableRequest

	if jc.Decode(&request) != nil {
		return
	}

	valid, _, err := h.accounts.ValidLoginByUserID(user, request.Password)

	if !valid {
		if err != nil {
			err = errors.Join(errInvalidLogin, err)
		}

		if jc.Check("failed to validate password", err) != nil {
			return
		}
	}

	err = h.accounts.OTPDisable(user)
	if jc.Check("failed to disable otp", err) != nil {
		return
	}
}
