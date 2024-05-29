package service

import (
	"errors"
	"fmt"
	"github.com/LumeWeb/portal/config"
	"github.com/LumeWeb/portal/core"
	"github.com/LumeWeb/portal/db/models"
	"github.com/LumeWeb/portal/service/mailer"
	"gorm.io/gorm"
	"time"
)

type EmailVerificationServiceDefault struct {
	ctx    *core.Context
	config config.Manager
	db     *gorm.DB
	user   core.UserService
	mailer core.MailerService
}

func NewEmailVerificationService(ctx *core.Context) *EmailVerificationServiceDefault {
	emailVerification := EmailVerificationServiceDefault{
		ctx:    ctx,
		config: ctx.Config(),
		db:     ctx.DB(),
		user:   ctx.Services().User(),
		mailer: ctx.Services().Mailer(),
	}

	ctx.RegisterService(emailVerification)

	return &emailVerification
}

func (e EmailVerificationServiceDefault) SendEmailVerification(userId uint) error {
	exists, user, err := e.user.AccountExists(userId)
	if !exists || err != nil {
		return err
	}

	if user.Verified {
		return core.NewAccountError(core.ErrKeyAccountAlreadyVerified, nil)
	}

	token := core.GenerateSecurityToken()

	var verification models.EmailVerification

	verification.UserID = user.ID
	verification.Token = token
	verification.ExpiresAt = time.Now().Add(time.Hour)

	err = e.db.Create(&verification).Error
	if err != nil {
		return core.NewAccountError(core.ErrKeyDatabaseOperationFailed, err)
	}

	verifyUrl := fmt.Sprintf("%s/account/verify?token=%s", fmt.Sprintf("https://%s.%s", e.config.Config().Core.AccountSubdomain, e.config.Config().Core.Domain), token)

	vars := map[string]interface{}{
		"FirstName":        user.FirstName,
		"Email":            user.Email,
		"VerificationLink": verifyUrl,
		"ExpireTime":       time.Until(verification.ExpiresAt).Round(time.Second * 2),
		"PortalName":       e.config.Config().Core.PortalName,
	}

	return e.mailer.TemplateSend(mailer.TPL_VERIFY_EMAIL, vars, vars, user.Email)
}

func (e EmailVerificationServiceDefault) VerifyEmail(email string, token string) error {
	var verification models.EmailVerification

	verification.Token = token

	result := e.db.Model(&verification).
		Preload("User").
		Where(&verification).
		First(&verification)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return core.NewAccountError(core.ErrKeySecurityInvalidToken, nil)
		}

		return core.NewAccountError(core.ErrKeyDatabaseOperationFailed, nil)
	}

	if verification.ExpiresAt.Before(time.Now()) {
		return core.NewAccountError(core.ErrKeySecurityTokenExpired, nil)
	}

	if len(verification.NewEmail) > 0 && verification.NewEmail != email {
		return core.NewAccountError(core.ErrKeySecurityInvalidToken, nil)
	} else if verification.User.Email != email {
		return core.NewAccountError(core.ErrKeySecurityInvalidToken, nil)
	}

	var update models.User

	doUpdate := false

	if !verification.User.Verified {
		update.Verified = true
		doUpdate = true
	}

	if len(verification.NewEmail) > 0 {
		update.Email = verification.NewEmail
		doUpdate = true
	}

	if doUpdate {
		err := e.user.UpdateAccountInfo(verification.UserID, update)
		if err != nil {
			return err
		}
	}

	verification = models.EmailVerification{
		UserID: verification.UserID,
	}

	if result := e.db.Where(&verification).Delete(&verification); result.Error != nil {
		return core.NewAccountError(core.ErrKeyDatabaseOperationFailed, result.Error)
	}

	return nil
}
