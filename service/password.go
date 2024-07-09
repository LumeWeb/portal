package service

import (
	"errors"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/db/models"
	"gorm.io/gorm"
	"time"
)

var _ core.PasswordResetService = (*PasswordResetServiceDefault)(nil)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID: core.PASSWORD_RESET_SERVICE,
		Factory: func() (core.Service, []core.ContextBuilderOption, error) {
			return NewPasswordResetService()
		},
		Depends: []string{core.USER_SERVICE, core.MAILER_SERVICE},
	})
}

type PasswordResetServiceDefault struct {
	ctx    core.Context
	config config.Manager
	db     *gorm.DB
	user   core.UserService
	mailer core.MailerService
}

func NewPasswordResetService() (*PasswordResetServiceDefault, []core.ContextBuilderOption, error) {
	passwordService := PasswordResetServiceDefault{}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			passwordService.ctx = ctx
			passwordService.config = ctx.Config()
			passwordService.db = ctx.DB()
			passwordService.user = ctx.Service(core.USER_SERVICE).(core.UserService)
			passwordService.mailer = ctx.Service(core.MAILER_SERVICE).(core.MailerService)
			return nil
		}),
	)

	return &passwordService, opts, nil
}

func (p PasswordResetServiceDefault) SendPasswordReset(user *models.User) error {
	token := core.GenerateSecurityToken()

	var reset models.PasswordReset

	reset.UserID = user.ID
	reset.Token = token
	reset.ExpiresAt = time.Now().Add(time.Hour)

	if err := db.RetryOnLock(p.db, func(db *gorm.DB) *gorm.DB {
		return db.Create(&reset)
	}); err != nil {
		return core.NewAccountError(core.ErrKeyDatabaseOperationFailed, err)
	}

	vars := map[string]interface{}{
		"FirstName":    user.FirstName,
		"Email":        user.Email,
		"ResetCode":    token,
		"ExpireTime":   reset.ExpiresAt,
		"PortalName":   p.config.Config().Core.PortalName,
		"PortalDomain": p.config.Config().Core.Domain,
	}

	return p.mailer.TemplateSend(core.MAILER_TPL_PASSWORD_RESET, vars, vars, user.Email)
}

func (p PasswordResetServiceDefault) ResetPassword(email string, token string, password string) error {
	var reset models.PasswordReset

	reset.Token = token

	if err := db.RetryOnLock(p.db, func(db *gorm.DB) *gorm.DB {
		return db.Model(&reset).
			Preload("User").
			Where(&reset).
			First(&reset)

	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return core.NewAccountError(core.ErrKeyUserNotFound, err)
		}

		return core.NewAccountError(core.ErrKeyDatabaseOperationFailed, err)
	}

	if reset.ExpiresAt.Before(time.Now()) {
		return core.NewAccountError(core.ErrKeySecurityTokenExpired, nil)
	}

	if reset.User.Email != email {
		return core.NewAccountError(core.ErrKeySecurityInvalidToken, nil)
	}

	passwordHash, err := p.user.HashPassword(password)
	if err != nil {
		return err
	}

	err = p.user.UpdateAccountInfo(reset.UserID, models.User{PasswordHash: passwordHash})
	if err != nil {
		return err
	}

	reset = models.PasswordReset{
		UserID: reset.UserID,
	}

	if err = db.RetryOnLock(p.db, func(db *gorm.DB) *gorm.DB {
		return db.Where(&reset).Delete(&reset)
	}); err != nil {
		return core.NewAccountError(core.ErrKeyDatabaseOperationFailed, err)
	}

	return nil
}
