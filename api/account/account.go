package account

import (
	"context"
	"crypto/ed25519"
	"net/http"

	"go.uber.org/zap"

	"github.com/julienschmidt/httprouter"

	"git.lumeweb.com/LumeWeb/portal/account"
	"git.lumeweb.com/LumeWeb/portal/api/middleware"
	"git.lumeweb.com/LumeWeb/portal/api/registry"
	"github.com/spf13/viper"
	"go.sia.tech/jape"
	"go.uber.org/fx"
)

var (
	_ registry.API = (*AccountAPI)(nil)
)

type AccountAPI struct {
	config   *viper.Viper
	accounts *account.AccountServiceDefault
	identity ed25519.PrivateKey
	logger   *zap.Logger
}

type AccountAPIParams struct {
	fx.In
	Config   *viper.Viper
	Accounts *account.AccountServiceDefault
	Identity ed25519.PrivateKey
	Logger   *zap.Logger
}

func NewS5(params AccountAPIParams) AccountApiResult {
	api := &AccountAPI{
		config:   params.Config,
		accounts: params.Accounts,
		identity: params.Identity,
		logger:   params.Logger,
	}

	return AccountApiResult{
		API:        api,
		AccountAPI: api,
	}
}

var Module = fx.Module("s5_api",
	fx.Provide(NewS5),
)

type AccountApiResult struct {
	fx.Out
	API        registry.API `group:"api"`
	AccountAPI *AccountAPI
}

func (a AccountAPI) Name() string {
	return "account"
}

func (a *AccountAPI) Init() error {
	return nil
}

func (a AccountAPI) Start(ctx context.Context) error {
	return nil
}

func (a AccountAPI) Stop(ctx context.Context) error {
	return nil
}

func (a AccountAPI) login(jc jape.Context) {
	var request LoginRequest

	if jc.Decode(&request) != nil {
		return
	}

	exists, _, err := a.accounts.EmailExists(request.Email)

	if !exists {
		_ = jc.Error(account.NewAccountError(account.ErrKeyInvalidLogin, nil), http.StatusUnauthorized)
		if err != nil {
			a.logger.Error("failed to check if email exists", zap.Error(err))
		}
		return
	}

	jwt, _, err := a.accounts.LoginPassword(request.Email, request.Password, jc.Request.RemoteAddr)
	if err != nil {
		return
	}

	jc.ResponseWriter.Header().Set("Authorization", "Bearer "+jwt)
	jc.ResponseWriter.WriteHeader(http.StatusOK)
}

func (a AccountAPI) register(jc jape.Context) {
	var request RegisterRequest

	if jc.Decode(&request) != nil {
		return
	}

	user, err := a.accounts.CreateAccount(request.Email, request.Password)
	if err != nil {
		_ = jc.Error(err, http.StatusUnauthorized)
		a.logger.Error("failed to update account name", zap.Error(err))
		return
	}

	err = a.accounts.UpdateAccountName(user.ID, request.FirstName, request.LastName)

	if err != nil {
		_ = jc.Error(account.NewAccountError(account.ErrKeyAccountCreationFailed, err), http.StatusBadRequest)
		a.logger.Error("failed to update account name", zap.Error(err))
		return
	}
}

func (a AccountAPI) otpGenerate(jc jape.Context) {
	user := middleware.GetUserFromContext(jc.Request.Context())

	otp, err := a.accounts.OTPGenerate(user)
	if jc.Check("failed to generate otp", err) != nil {
		return
	}

	jc.Encode(&OTPGenerateResponse{
		OTP: otp,
	})
}

func (a AccountAPI) otpVerify(jc jape.Context) {
	user := middleware.GetUserFromContext(jc.Request.Context())

	var request OTPVerifyRequest

	if jc.Decode(&request) != nil {
		return
	}

	err := a.accounts.OTPEnable(user, request.OTP)

	if jc.Check("failed to verify otp", err) != nil {
		return
	}
}

func (a AccountAPI) otpValidate(jc jape.Context) {
	user := middleware.GetUserFromContext(jc.Request.Context())

	var request OTPValidateRequest

	if jc.Decode(&request) != nil {
		return
	}

	jwt, err := a.accounts.LoginOTP(user, request.OTP)
	if jc.Check("failed to validate otp", err) != nil {
		return
	}

	account.SendJWT(jc, jwt)
}

func (a AccountAPI) otpDisable(jc jape.Context) {
	user := middleware.GetUserFromContext(jc.Request.Context())

	var request OTPDisableRequest

	if jc.Decode(&request) != nil {
		return
	}

	valid, _, err := a.accounts.ValidLoginByUserID(user, request.Password)

	if !valid {
		_ = jc.Error(account.NewAccountError(account.ErrKeyInvalidLogin, nil), http.StatusUnauthorized)
		return
	}

	err = a.accounts.OTPDisable(user)
	if jc.Check("failed to disable otp", err) != nil {
		return
	}
}

func (a AccountAPI) Routes() (*httprouter.Router, error) {
	authMw2fa := authMiddleware(middleware.AuthMiddlewareOptions{
		Identity: a.identity,
		Accounts: a.accounts,
		Config:   a.config,
		Purpose:  account.JWTPurpose2FA,
	})

	authMw := authMiddleware(middleware.AuthMiddlewareOptions{
		Identity: a.identity,
		Accounts: a.accounts,
		Config:   a.config,
		Purpose:  account.JWTPurposeLogin,
	})

	return jape.Mux(map[string]jape.Handler{
		"/api/auth/login":        middleware.ApplyMiddlewares(a.login, authMw2fa, middleware.ProxyMiddleware),
		"/api/auth/register":     a.register,
		"/api/auth/otp/generate": middleware.ApplyMiddlewares(a.otpGenerate, authMw, middleware.ProxyMiddleware),
		"/api/auth/otp/verify":   middleware.ApplyMiddlewares(a.otpVerify, authMw, middleware.ProxyMiddleware),
		"/api/auth/otp/validate": middleware.ApplyMiddlewares(a.otpValidate, authMw, middleware.ProxyMiddleware),
		"/api/auth/otp/disable":  middleware.ApplyMiddlewares(a.otpDisable, authMw, middleware.ProxyMiddleware),
	}), nil
}
