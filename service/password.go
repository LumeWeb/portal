package service

import (
	"errors"
	"github.com/LumeWeb/portal/config"
	"github.com/LumeWeb/portal/core"
	"github.com/LumeWeb/portal/db/models"
	"github.com/LumeWeb/portal/service/internal/mailer"
	"gorm.io/gorm"
	"time"
)

type PasswordResetServiceDefault struct {
	ctx    *core.Context
	config config.Manager
	db     *gorm.DB
	user   core.UserService
	mailer core.MailerService
}

func NewPasswordResetService(ctx *core.Context) *PasswordResetServiceDefault {
	passwordService := PasswordResetServiceDefault{
		ctx:    ctx,
		config: ctx.Config(),
		db:     ctx.DB(),
		user:   ctx.Services().User(),
		mailer: ctx.Services().Mailer(),
	}

	ctx.RegisterService(passwordService)

	return &passwordService
}

func (p PasswordResetServiceDefault) SendPasswordReset(user *models.User) error {
	token := core.GenerateSecurityToken()

	var reset models.PasswordReset

	reset.UserID = user.ID
	reset.Token = token
	reset.ExpiresAt = time.Now().Add(time.Hour)

	err := p.db.Create(&reset).Error
	if err != nil {
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

	return p.mailer.TemplateSend(mailer.TPL_PASSWORD_RESET, vars, vars, user.Email)
}

func (p PasswordResetServiceDefault) ResetPassword(email string, token string, password string) error {
	var reset models.PasswordReset

	reset.Token = token

	result := p.db.Model(&reset).
		Preload("User").
		Where(&reset).
		First(&reset)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return core.NewAccountError(core.ErrKeyUserNotFound, result.Error)
		}

		return core.NewAccountError(core.ErrKeyDatabaseOperationFailed, result.Error)
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

	if result := p.db.Where(&reset).Delete(&reset); result.Error != nil {
		return core.NewAccountError(core.ErrKeyDatabaseOperationFailed, result.Error)
	}

	return nil
}