package service

import (
	"errors"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"time"
)

var _ core.AuthService = (*AuthServiceDefault)(nil)

type AuthServiceDefault struct {
	ctx    *core.Context
	config config.Manager
	db     *gorm.DB
	user   core.UserService
	opt    core.OTPService
}

func NewAuthService(ctx *core.Context) *AuthServiceDefault {
	authService := &AuthServiceDefault{
		ctx:    ctx,
		config: ctx.Config(),
		db:     ctx.DB(),
		user:   ctx.Services().User(),
		opt:    ctx.Services().Otp(),
	}
	ctx.RegisterService(authService)

	return authService
}

func (a AuthServiceDefault) LoginPassword(email string, password string, ip string) (string, *models.User, error) {
	valid, user, err := a.ValidLoginByEmail(email, password)

	if err != nil {
		return "", nil, err
	}

	if !valid {
		return "", nil, nil
	}

	token, err := a.doLogin(user, ip, false)

	if err != nil {
		return "", nil, err
	}

	return token, user, nil
}

func (a AuthServiceDefault) LoginOTP(userId uint, code string) (string, error) {
	valid, err := a.opt.OTPVerify(userId, code)

	if err != nil {
		return "", err
	}

	if !valid {
		return "", core.NewAccountError(core.ErrKeyInvalidOTPCode, nil)
	}

	var user models.User
	user.ID = userId

	token, tokenErr := core.JWTGenerateToken(a.config.Config().Core.Domain, a.ctx.Config().Config().Core.Identity.PrivateKey(), user.ID, core.JWTPurposeLogin)
	if tokenErr != nil {
		return "", err
	}

	return token, nil
}

func (a AuthServiceDefault) LoginPubkey(pubkey string, ip string) (string, error) {
	var model models.PublicKey

	result := a.db.Model(&models.PublicKey{}).Preload("User").Where(&models.PublicKey{Key: pubkey}).First(&model)

	if result.RowsAffected == 0 || result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return "", core.NewAccountError(core.ErrKeyInvalidLogin, result.Error)
		}

		return "", core.NewAccountError(core.ErrKeyDatabaseOperationFailed, result.Error)
	}

	user := model.User

	token, err := a.doLogin(&user, ip, true)

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

	result := a.db.Model(&models.User{}).Where(&models.User{Email: email}).First(&user)

	if result.RowsAffected == 0 || result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return false, nil, core.NewAccountError(core.ErrKeyInvalidLogin, result.Error)
		}

		return false, nil, core.NewAccountError(core.ErrKeyDatabaseOperationFailed, result.Error)
	}

	valid := a.ValidLoginByUserObj(&user, password)

	if !valid {
		return false, nil, nil
	}

	return true, &user, nil
}

func (a AuthServiceDefault) ValidLoginByUserID(id uint, password string) (bool, *models.User, error) {
	var user models.User

	user.ID = id

	result := a.db.Model(&user).Where(&user).First(&user)

	if result.RowsAffected == 0 || result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return false, nil, core.NewAccountError(core.ErrKeyInvalidLogin, result.Error)
		}

		return false, nil, core.NewAccountError(core.ErrKeyDatabaseOperationFailed, result.Error)
	}

	valid := a.ValidLoginByUserObj(&user, password)

	if !valid {
		return false, nil, nil
	}

	return true, &user, nil
}
func (a AuthServiceDefault) doLogin(user *models.User, ip string, bypassSecurity bool) (string, error) {
	purpose := core.JWTPurposeLogin

	if user.OTPEnabled && !bypassSecurity {
		purpose = core.JWTPurpose2FA
	}

	token, jwtErr := core.JWTGenerateToken(a.config.Config().Core.Domain, a.ctx.Config().Config().Core.Identity.PrivateKey(), user.ID, purpose)
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
