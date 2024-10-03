package service

import (
	"errors"
	"fmt"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/event"
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
	ctx       core.Context
	config    config.Manager
	db        *gorm.DB
	user      core.UserService
	mailer    core.MailerService
	subdomain string
}

func NewPasswordResetService() (*PasswordResetServiceDefault, []core.ContextBuilderOption, error) {
	passwordService := PasswordResetServiceDefault{}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			passwordService.ctx = ctx
			passwordService.config = ctx.Config()
			passwordService.db = ctx.DB()
			passwordService.user = core.GetService[core.UserService](ctx, core.USER_SERVICE)
			passwordService.mailer = core.GetService[core.MailerService](ctx, core.MAILER_SERVICE)

			event.Listen[*event.UserServiceSubdomainSetEvent](ctx, event.EVENT_USER_SERVICE_SUBDOMAIN_SET, func(evt *event.UserServiceSubdomainSetEvent) error {
				passwordService.subdomain = evt.Subdomain()
				return nil
			})
			return nil
		}),
	)

	return &passwordService, opts, nil
}

func (p PasswordResetServiceDefault) ID() string {
	return core.PASSWORD_RESET_SERVICE
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

	resetUrl := fmt.Sprintf("%s/reset-password/confirm?token=%s", fmt.Sprintf("https://%s.%s", p.subdomain, p.config.Config().Core.Domain), token)

	vars := map[string]interface{}{
		"FirstName":  user.FirstName,
		"Email":      user.Email,
		"ResetLink":  resetUrl,
		"ExpireTime": reset.ExpiresAt,
		"PortalName": p.config.Config().Core.PortalName,
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

	err = p.user.UpdateAccountInfo(reset.UserID, map[string]interface{}{"password_hash": passwordHash})
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
