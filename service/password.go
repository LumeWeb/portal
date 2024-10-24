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
	"net/url"
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

	if err := db.RetryableTransaction(p.ctx, p.db, func(tx *gorm.DB) *gorm.DB {
		return tx.Create(&reset)
	}); err != nil {
		return core.NewAccountError(core.ErrKeyDatabaseOperationFailed, err)
	}

	queryVars := url.Values{}
	queryVars.Set("email", user.Email)
	queryVars.Set("token", token)
	resetUrl := fmt.Sprintf("%s/reset-password/confirm?%s", fmt.Sprintf("https://%s.%s", p.subdomain, p.config.Config().Core.Domain), queryVars.Encode())

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

	exists, user, err := p.user.EmailExists(email)
	if err != nil {
		return err
	}

	if !exists {
		return core.NewAccountError(core.ErrKeyUserNotFound, nil)
	}

	reset.Token = token
	reset.UserID = user.ID

	if err := db.RetryableTransaction(p.ctx, p.db, func(tx *gorm.DB) *gorm.DB {
		return tx.Model(&reset).
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

	if err := db.RetryableTransaction(p.ctx, p.db, func(tx *gorm.DB) *gorm.DB {
		return tx.Where(&reset).Delete(&reset)
	}); err != nil {
		return core.NewAccountError(core.ErrKeyDatabaseOperationFailed, err)
	}

	return nil
}
