package service

import (
	"errors"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/db/models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"time"
)

var _ core.AuthService = (*AuthServiceDefault)(nil)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID: core.AUTH_SERVICE,
		Factory: func() (core.Service, []core.ContextBuilderOption, error) {
			return NewAuthService()
		},
		Depends: []string{core.USER_SERVICE, core.OTP_SERVICE},
	})
}

type AuthServiceDefault struct {
	ctx    core.Context
	config config.Manager
	db     *gorm.DB
	user   core.UserService
	otp    core.OTPService
}

func NewAuthService() (*AuthServiceDefault, []core.ContextBuilderOption, error) {
	authService := &AuthServiceDefault{}
	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			authService.ctx = ctx
			authService.config = ctx.Config()
			authService.db = ctx.DB()
			authService.user = core.GetService[core.UserService](ctx, core.USER_SERVICE)
			authService.otp = core.GetService[core.OTPService](ctx, core.OTP_SERVICE)
			return nil
		}),
	)

	return authService, opts, nil
}

func (a AuthServiceDefault) ID() string {
	return core.AUTH_SERVICE
}

func (a AuthServiceDefault) LoginPassword(email string, password string, ip string, rememberMe bool) (string, *models.User, error) {
	valid, user, err := a.ValidLoginByEmail(email, password)

	if err != nil {
		return "", nil, err
	}

	if !valid {
		return "", nil, nil
	}

	token, err := a.doLogin(user, ip, false, rememberMe)

	if err != nil {
		return "", nil, err
	}

	return token, user, nil
}

func (a AuthServiceDefault) LoginOTP(userId uint, code string) (string, error) {
	valid, err := a.otp.OTPVerify(userId, code)

	if err != nil {
		return "", err
	}

	if !valid {
		return "", core.NewAccountError(core.ErrKeyInvalidOTPCode, nil)
	}

	var user models.User
	user.ID = userId

	token, tokenErr := core.JWTGenerateToken(a.config.Config().Core.Domain, a.ctx.Config().Config().Core.Identity.PrivateKey(), user.ID, core.JWTPurposeLogin, false)
	if tokenErr != nil {
		return "", err
	}

	return token, nil
}

func (a AuthServiceDefault) LoginPubkey(pubkey string, ip string) (string, error) {
	var model models.PublicKey
	var rowsAffected int64

	err := db.RetryOnLock(a.db, func(db *gorm.DB) *gorm.DB {
		tx := db.Model(&models.PublicKey{}).Preload("User").Where(&models.PublicKey{Key: pubkey}).First(&model)
		rowsAffected = tx.RowsAffected
		return tx
	})

	if rowsAffected == 0 || err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", core.NewAccountError(core.ErrKeyInvalidLogin, err)
		}
		return "", core.NewAccountError(core.ErrKeyDatabaseOperationFailed, err)
	}

	user := model.User

	token, err := a.doLogin(&user, ip, true, false)

	if err != nil {
		return "", err
	}

	return token, nil
}

func (a AuthServiceDefault) LoginID(id uint, ip string) (string, error) {
	var user models.User
	var rowsAffected int64

	user.ID = id

	err := db.RetryOnLock(a.db, func(db *gorm.DB) *gorm.DB {
		tx := db.Model(&user).Where(&user).First(&user)
		rowsAffected = tx.RowsAffected
		return tx
	})

	if rowsAffected == 0 || err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", core.NewAccountError(core.ErrKeyInvalidLogin, err)
		}
		return "", core.NewAccountError(core.ErrKeyDatabaseOperationFailed, err)
	}

	token, err := a.doLogin(&user, ip, true, false)

	if err != nil {
		return "", err
	}

	return token, nil
}

func (a AuthServiceDefault) ValidLoginByUserObj(user *models.User, password string) bool {
	return a.validPassword(user, password)
}

func (a AuthServiceDefault) ValidLoginByEmail(email string, password string) (bool, *models.User, error) {
	var user models.User
	var rowsAffected int64

	err := db.RetryOnLock(a.db, func(db *gorm.DB) *gorm.DB {
		tx := db.Model(&models.User{}).Where(&models.User{Email: email}).First(&user)
		rowsAffected = tx.RowsAffected
		return tx
	})

	if rowsAffected == 0 || err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil, core.NewAccountError(core.ErrKeyInvalidLogin, err)
		}
		return false, nil, core.NewAccountError(core.ErrKeyDatabaseOperationFailed, err)
	}

	valid := a.ValidLoginByUserObj(&user, password)

	if !valid {
		return false, nil, nil
	}

	return true, &user, nil
}

func (a AuthServiceDefault) ValidLoginByUserID(id uint, password string) (bool, *models.User, error) {
	var user models.User
	var rowsAffected int64

	user.ID = id

	err := db.RetryOnLock(a.db, func(db *gorm.DB) *gorm.DB {
		tx := db.Model(&user).Where(&user).First(&user)
		rowsAffected = tx.RowsAffected
		return tx
	})

	if rowsAffected == 0 || err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil, core.NewAccountError(core.ErrKeyInvalidLogin, err)
		}
		return false, nil, core.NewAccountError(core.ErrKeyDatabaseOperationFailed, err)
	}

	valid := a.ValidLoginByUserObj(&user, password)

	if !valid {
		return false, nil, nil
	}

	return true, &user, nil
}
func (a AuthServiceDefault) doLogin(user *models.User, ip string, bypassSecurity bool, rememberMe bool) (string, error) {
	purpose := core.JWTPurposeLogin

	if user.OTPEnabled && !bypassSecurity {
		purpose = core.JWTPurpose2FA
	}

	token, jwtErr := core.JWTGenerateToken(a.config.Config().Core.Domain, a.ctx.Config().Config().Core.Identity.PrivateKey(), user.ID, purpose, rememberMe)
	if jwtErr != nil {
		return "", core.NewAccountError(core.ErrKeyJWTGenerationFailed, jwtErr)
	}

	now := time.Now()

	err := a.user.UpdateAccountInfo(user.ID, models.User{LastLoginIP: ip, LastLogin: &now})
	if err != nil {
		return "", err
	}

	return token, nil
}
func (a AuthServiceDefault) validPassword(user *models.User, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))

	return err == nil
}
