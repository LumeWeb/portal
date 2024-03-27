package account

import (
	"context"
	"crypto/ed25519"
	"embed"
	_ "embed"
	"errors"
	"io/fs"
	"net/http"
	"strings"

	"github.com/rs/cors"

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

//go:embed all:app/build/client
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
	if err != nil || user == nil {
		_ = jc.Error(account.NewAccountError(account.ErrKeyInvalidLogin, err), http.StatusUnauthorized)
		if err != nil {
			a.logger.Error("failed to login", zap.Error(err))
		}
		return
	}

	account.SetAuthCookie(jc, a.config, jwt)
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

	user, err := a.accounts.CreateAccount(request.Email, request.Password, true)
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

	if request.Email == "" || request.Token == "" {
		_ = jc.Error(errors.New("invalid request"), http.StatusBadRequest)
		return
	}

	err := a.accounts.VerifyEmail(request.Email, request.Token)

	if jc.Check("Failed to verify email", err) != nil {
		return
	}
}

func (a AccountAPI) resendVerifyEmail(jc jape.Context) {
	user := middleware.GetUserFromContext(jc.Request.Context())

	err := a.accounts.SendEmailVerification(user)

	if jc.Check("failed to resend email verification", err) != nil {
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

	account.SetAuthCookie(jc, a.config, jwt)
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
	token := middleware.GetAuthTokenFromContext(jc.Request.Context())
	account.EchoAuthCookie(jc, a.config)
	jc.Encode(&PongResponse{
		Ping:  "pong",
		Token: token,
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
		Verified:  acct.Verified,
	})

}

func (a AccountAPI) logout(c jape.Context) {
	account.ClearAuthCookie(c, a.config)
}

func (a AccountAPI) uploadLimit(c jape.Context) {
	c.Encode(&UploadLimitResponse{
		Limit: a.config.Config().Core.PostUploadLimit,
	})
}

func (a AccountAPI) updateEmail(c jape.Context) {
	user := middleware.GetUserFromContext(c.Request.Context())

	var request UpdateEmailRequest

	if c.Decode(&request) != nil {
		return
	}

	err := a.accounts.UpdateAccountEmail(user, request.Email, request.Password)
	if c.Check("failed to update email", err) != nil {
		return
	}
}

func (a AccountAPI) updatePassword(c jape.Context) {
	user := middleware.GetUserFromContext(c.Request.Context())

	var request UpdatePasswordRequest

	if c.Decode(&request) != nil {
		return
	}

	err := a.accounts.UpdateAccountPassword(user, request.CurrentPassword, request.NewPassword)
	if c.Check("failed to update password", err) != nil {
		return
	}

}

func (a AccountAPI) meta(c jape.Context) {
	c.Encode(&MetaResponse{
		Domain: a.config.Config().Core.Domain,
	})

}

func (a *AccountAPI) Routes() (*httprouter.Router, error) {
	loginAuthMw2fa := authMiddleware(middleware.AuthMiddlewareOptions{
		Identity:       a.identity,
		Accounts:       a.accounts,
		Config:         a.config,
		Purpose:        account.JWTPurpose2FA,
		EmptyAllowed:   true,
		ExpiredAllowed: true,
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

	appFiles, _ := fs.Sub(appFs, "app/build/client")
	appServ := http.FileServer(http.FS(appFiles))

	appHandler := func(c jape.Context) {
		appServ.ServeHTTP(c.ResponseWriter, c.Request)
	}

	appServer := middleware.ApplyMiddlewares(appHandler, middleware.ProxyMiddleware)

	swaggerRoutes, err := swagger.Swagger(swagSpec, map[string]jape.Handler{})
	if err != nil {
		return nil, err
	}

	swaggerJape := jape.Mux(swaggerRoutes)

	getApiJape := jape.Mux(map[string]jape.Handler{
		"GET /api/auth/otp/generate": middleware.ApplyMiddlewares(a.otpGenerate, authMw, middleware.ProxyMiddleware),
		"GET /api/account":           middleware.ApplyMiddlewares(a.accountInfo, authMw, middleware.ProxyMiddleware),
		"GET /api/upload-limit":      middleware.ApplyMiddlewares(a.uploadLimit, middleware.ProxyMiddleware),
		"GET /api/meta":              middleware.ApplyMiddlewares(a.meta, middleware.ProxyMiddleware),
	})

	getHandler := func(c jape.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api") {
			getApiJape.ServeHTTP(c.ResponseWriter, c.Request)
			return
		}

		if strings.HasPrefix(c.Request.URL.Path, "/swagger") {
			swaggerJape.ServeHTTP(c.ResponseWriter, c.Request)
			return
		}

		if !strings.HasPrefix(c.Request.URL.Path, "/assets") && c.Request.URL.Path != "favicon.ico" && c.Request.URL.Path != "/" && !strings.HasSuffix(c.Request.URL.Path, ".html") {
			c.Request.URL.Path = "/"
		}
		appServer(c)
	}

	corsMw := cors.New(cors.Options{
		AllowOriginFunc: func(origin string) bool {
			return true
		},
		AllowedMethods: []string{"GET", "POST", "DELETE"},
		AllowedHeaders: []string{"Authorization", "Content-Type"},
	})

	corsOptionsHandler := func(c jape.Context) {
		c.ResponseWriter.WriteHeader(http.StatusOK)
	}

	routes := map[string]jape.Handler{
		// Auth
		"POST /api/auth/ping":         middleware.ApplyMiddlewares(a.ping, corsMw.Handler, pingAuthMw, middleware.ProxyMiddleware),
		"POST /api/auth/login":        middleware.ApplyMiddlewares(a.login, corsMw.Handler, loginAuthMw2fa, middleware.ProxyMiddleware),
		"POST /api/auth/register":     middleware.ApplyMiddlewares(a.register, corsMw.Handler, middleware.ProxyMiddleware),
		"POST /api/auth/otp/validate": middleware.ApplyMiddlewares(a.otpValidate, corsMw.Handler, authMw, middleware.ProxyMiddleware),
		"POST /api/auth/logout":       middleware.ApplyMiddlewares(a.logout, corsMw.Handler, authMw, middleware.ProxyMiddleware),

		// Account
		"POST /api/account/verify-email":           middleware.ApplyMiddlewares(a.verifyEmail, corsMw.Handler, authMw, middleware.ProxyMiddleware),
		"POST /api/account/verify-email/resend":    middleware.ApplyMiddlewares(a.resendVerifyEmail, corsMw.Handler, authMw, middleware.ProxyMiddleware),
		"POST /api/account/otp/verify":             middleware.ApplyMiddlewares(a.otpVerify, corsMw.Handler, authMw, middleware.ProxyMiddleware),
		"POST /api/account/otp/disable":            middleware.ApplyMiddlewares(a.otpDisable, corsMw.Handler, authMw, middleware.ProxyMiddleware),
		"POST /api/account/password-reset/request": middleware.ApplyMiddlewares(a.passwordResetRequest, corsMw.Handler, middleware.ProxyMiddleware),
		"POST /api/account/password-reset/confirm": middleware.ApplyMiddlewares(a.passwordResetConfirm, corsMw.Handler, middleware.ProxyMiddleware),
		"POST /api/account/update-email":           middleware.ApplyMiddlewares(a.updateEmail, corsMw.Handler, authMw, middleware.ProxyMiddleware),
		"POST /api/account/update-password":        middleware.ApplyMiddlewares(a.updatePassword, corsMw.Handler, authMw, middleware.ProxyMiddleware),

		// CORS
		"OPTIONS /api/auth/ping":         middleware.ApplyMiddlewares(corsOptionsHandler, corsMw.Handler, authMw, middleware.ProxyMiddleware),
		"OPTIONS /api/auth/login":        middleware.ApplyMiddlewares(corsOptionsHandler, corsMw.Handler, loginAuthMw2fa, middleware.ProxyMiddleware),
		"OPTIONS /api/auth/register":     middleware.ApplyMiddlewares(corsOptionsHandler, corsMw.Handler, middleware.ProxyMiddleware),
		"OPTIONS /api/auth/otp/validate": middleware.ApplyMiddlewares(corsOptionsHandler, corsMw.Handler, middleware.ProxyMiddleware),
		"OPTIONS /api/auth/logout":       middleware.ApplyMiddlewares(corsOptionsHandler, corsMw.Handler, authMw, middleware.ProxyMiddleware),

		"OPTIONS /api/account/verify-email":           middleware.ApplyMiddlewares(corsOptionsHandler, corsMw.Handler, middleware.ProxyMiddleware),
		"OPTIONS /api/account/verify-email/resend":    middleware.ApplyMiddlewares(corsOptionsHandler, corsMw.Handler, authMw, middleware.ProxyMiddleware),
		"OPTIONS /api/account/otp/verify":             middleware.ApplyMiddlewares(corsOptionsHandler, corsMw.Handler, authMw, middleware.ProxyMiddleware),
		"OPTIONS /api/account/otp/disable":            middleware.ApplyMiddlewares(corsOptionsHandler, corsMw.Handler, authMw, middleware.ProxyMiddleware),
		"OPTIONS /api/account/password-reset/request": middleware.ApplyMiddlewares(corsOptionsHandler, corsMw.Handler, middleware.ProxyMiddleware),
		"OPTIONS /api/account/password-reset/confirm": middleware.ApplyMiddlewares(corsOptionsHandler, corsMw.Handler, middleware.ProxyMiddleware),
		"OPTIONS /api/account/update-email":           middleware.ApplyMiddlewares(corsOptionsHandler, corsMw.Handler, authMw, middleware.ProxyMiddleware),
		"OPTIONS /api/account/update-password":        middleware.ApplyMiddlewares(corsOptionsHandler, corsMw.Handler, authMw, middleware.ProxyMiddleware),

		// Get Routes
		"OPTIONS /api/upload-limit":      middleware.ApplyMiddlewares(corsOptionsHandler, corsMw.Handler, middleware.ProxyMiddleware),
		"OPTIONS /api/account":           middleware.ApplyMiddlewares(corsOptionsHandler, corsMw.Handler, authMw, middleware.ProxyMiddleware),
		"OPTIONS /api/auth/otp/generate": middleware.ApplyMiddlewares(corsOptionsHandler, corsMw.Handler, authMw, middleware.ProxyMiddleware),

		"GET /*path": middleware.ApplyMiddlewares(getHandler, corsMw.Handler),
	}

	return jape.Mux(routes), nil
}
func (a AccountAPI) Can(w http.ResponseWriter, r *http.Request) bool {
	return false
}

func (a AccountAPI) Handle(w http.ResponseWriter, r *http.Request) {
}

func (a *AccountAPI) Domain() string {
	return router.BuildSubdomain(a, a.config)
}

func (a AccountAPI) AuthTokenName() string {
	return account.AUTH_COOKIE_NAME
}
