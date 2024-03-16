package account

import (
	"context"
	"crypto/ed25519"
	"embed"
	_ "embed"
	"io/fs"
	"net/http"
	"time"

	"git.lumeweb.com/LumeWeb/portal/api/swagger"

	"git.lumeweb.com/LumeWeb/portal/api/router"

	"git.lumeweb.com/LumeWeb/portal/config"

	"go.uber.org/zap"

	"github.com/julienschmidt/httprouter"

	"git.lumeweb.com/LumeWeb/portal/account"
	"git.lumeweb.com/LumeWeb/portal/api/middleware"
	"git.lumeweb.com/LumeWeb/portal/api/registry"
	"go.sia.tech/jape"
	"go.uber.org/fx"
)

//go:embed swagger.yaml
var swagSpec []byte

//go:embed app/build/client/build
var appFs embed.FS

var (
	_ registry.API       = (*AccountAPI)(nil)
	_ router.RoutableAPI = (*AccountAPI)(nil)
)

type AccountAPI struct {
	config   *config.Manager
	accounts *account.AccountServiceDefault
	identity ed25519.PrivateKey
	logger   *zap.Logger
}

type AccountAPIParams struct {
	fx.In
	Config   *config.Manager
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

	jwt, user, err := a.accounts.LoginPassword(request.Email, request.Password, jc.Request.RemoteAddr)
	if err != nil {
		return
	}

	http.SetCookie(jc.ResponseWriter, &http.Cookie{
		Name:     "jwt",
		Value:    jwt,
		Expires:  time.Now().Add(24 * time.Hour),
		HttpOnly: true,
	})
	account.SendJWT(jc, jwt)

	jc.Encode(&LoginResponse{
		Token: jwt,
		Otp:   user.OTPEnabled && user.OTPVerified,
	})
}

func (a AccountAPI) register(jc jape.Context) {
	var request RegisterRequest

	if jc.Decode(&request) != nil {
		return
	}

	if len(request.FirstName) == 0 || len(request.LastName) == 0 {
		_ = jc.Error(account.NewAccountError(account.ErrKeyAccountCreationFailed, nil), http.StatusBadRequest)
		return
	}

	user, err := a.accounts.CreateAccount(request.Email, request.Password, false)
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

func (a AccountAPI) verifyEmail(jc jape.Context) {
	var request VerifyEmailRequest

	if jc.Decode(&request) != nil {
		return
	}

	err := a.accounts.VerifyEmail(request.Email, request.Token)

	if jc.Check("failed to verify email", err) != nil {
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

	http.SetCookie(jc.ResponseWriter, &http.Cookie{
		Name:     "jwt",
		Value:    jwt,
		Expires:  time.Now().Add(24 * time.Hour),
		HttpOnly: true,
	})
	account.SendJWT(jc, jwt)

	jc.Encode(&LoginResponse{
		Token: jwt,
		Otp:   false,
	})
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

func (a AccountAPI) passwordResetRequest(jc jape.Context) {
	var request PasswordResetRequest

	if jc.Decode(&request) != nil {
		return
	}

	exists, user, err := a.accounts.EmailExists(request.Email)
	if jc.Check("invalid request", err) != nil || !exists {
		return
	}

	err = a.accounts.SendPasswordReset(user)
	if jc.Check("failed to request password reset", err) != nil {
		return
	}

	jc.ResponseWriter.WriteHeader(http.StatusOK)
}

func (a AccountAPI) passwordResetConfirm(jc jape.Context) {
	var request PasswordResetVerifyRequest

	if jc.Decode(&request) != nil {
		return
	}

	exists, _, err := a.accounts.EmailExists(request.Email)
	if jc.Check("invalid request", err) != nil || !exists {
		return
	}

	err = a.accounts.ResetPassword(request.Email, request.Password, request.Token)
	if jc.Check("failed to reset password", err) != nil {
		return
	}

	jc.ResponseWriter.WriteHeader(http.StatusOK)
}

func (a AccountAPI) ping(jc jape.Context) {
	jc.Encode(&PongResponse{
		Ping: "pong",
	})
}

func (a AccountAPI) accountInfo(jc jape.Context) {
	user := middleware.GetUserFromContext(jc.Request.Context())

	_, acct, _ := a.accounts.AccountExists(user)

	jc.Encode(&AccountInfoResponse{
		ID:        acct.ID,
		Email:     acct.Email,
		FirstName: acct.FirstName,
		LastName:  acct.LastName,
	})

}

func (a AccountAPI) Routes() (*httprouter.Router, error) {
	loginAuthMw2fa := authMiddleware(middleware.AuthMiddlewareOptions{
		Identity:     a.identity,
		Accounts:     a.accounts,
		Config:       a.config,
		Purpose:      account.JWTPurpose2FA,
		EmptyAllowed: true,
	})

	authMw := authMiddleware(middleware.AuthMiddlewareOptions{
		Identity: a.identity,
		Accounts: a.accounts,
		Config:   a.config,
		Purpose:  account.JWTPurposeNone,
	})

	pingAuthMw := authMiddleware(middleware.AuthMiddlewareOptions{
		Identity: a.identity,
		Accounts: a.accounts,
		Config:   a.config,
		Purpose:  account.JWTPurposeLogin,
	})

	appFiles, _ := fs.Sub(appFs, "app")
	appServ := http.FileServer(http.FS(appFiles))

	appHandler := func(c jape.Context) {
		appServ.ServeHTTP(c.ResponseWriter, c.Request)
	}

	routes := map[string]jape.Handler{
		"POST /api/auth/ping":                   middleware.ApplyMiddlewares(a.ping, pingAuthMw, middleware.ProxyMiddleware),
		"POST /api/auth/login":                  middleware.ApplyMiddlewares(a.login, loginAuthMw2fa, middleware.ProxyMiddleware),
		"POST /api/auth/register":               middleware.ApplyMiddlewares(a.register, middleware.ProxyMiddleware),
		"POST /api/auth/verify-email":           middleware.ApplyMiddlewares(a.verifyEmail, middleware.ProxyMiddleware),
		"GET /api/auth/otp/generate":            middleware.ApplyMiddlewares(a.otpGenerate, authMw, middleware.ProxyMiddleware),
		"POST /api/auth/otp/verify":             middleware.ApplyMiddlewares(a.otpVerify, authMw, middleware.ProxyMiddleware),
		"POST /api/auth/otp/validate":           middleware.ApplyMiddlewares(a.otpValidate, authMw, middleware.ProxyMiddleware),
		"POST /api/auth/otp/disable":            middleware.ApplyMiddlewares(a.otpDisable, authMw, middleware.ProxyMiddleware),
		"POST /api/auth/password-reset/request": middleware.ApplyMiddlewares(a.passwordResetRequest, middleware.ProxyMiddleware),
		"POST /api/auth/password-reset/confirm": middleware.ApplyMiddlewares(a.passwordResetConfirm, middleware.ProxyMiddleware),
		"GET /api/account":                      middleware.ApplyMiddlewares(a.accountInfo, authMw, middleware.ProxyMiddleware),
		"GET /*path":                            middleware.ApplyMiddlewares(appHandler, middleware.ProxyMiddleware),
	}

	routes, err := swagger.Swagger(swagSpec, routes)
	if err != nil {
		return nil, err
	}

	return jape.Mux(routes), nil
}
func (a AccountAPI) Can(w http.ResponseWriter, r *http.Request) bool {
	return false
}

func (a AccountAPI) Handle(w http.ResponseWriter, r *http.Request) {
	// noop
}
