package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sync"
	"time"

	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/db/models"
	pevent "go.lumeweb.com/portal/event"
	"go.lumeweb.com/portal/service/internal/password"
	"gorm.io/gorm"
)

var _ core.PasswordResetService = (*PasswordResetServiceDefault)(nil)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID:      core.PASSWORD_RESET_SERVICE,
		Factory: NewPasswordResetService,
		Depends: []string{core.USER_SERVICE, core.MAILER_SERVICE},
		Metrics: password.GetCollectors(),
	})
}

type PasswordResetServiceDefault struct {
	user      core.UserService
	mailer    core.MailerService
	mu        *sync.RWMutex
	subdomain string
	core.Service
}

func NewPasswordResetService() (core.Service, []core.ContextBuilderOption, error) {
	passwordService := PasswordResetServiceDefault{
		mu: &sync.RWMutex{},
	}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			passwordService.user = core.GetService[core.UserService](ctx, core.USER_SERVICE)
			passwordService.mailer = core.GetService[core.MailerService](ctx, core.MAILER_SERVICE)
			core.Listen(ctx, pevent.EVENT_USER_SERVICE_SUBDOMAIN_SET, func(e *core.CoreEvent[pevent.UserServiceSubdomainSetEvent]) error {
				passwordService.SetSubdomain(e.Data.Subdomain)
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

func (p *PasswordResetServiceDefault) SetSubdomain(subdomain string) {
	p.mu.Lock()
	p.subdomain = subdomain
	p.mu.Unlock()
}

func (p PasswordResetServiceDefault) SendPasswordReset(ctx context.Context, user *models.User) error {
	ctx, span := core.TraceMethod(ctx, "PasswordResetServiceDefault.SendPasswordReset")
	defer span.End()

	return core.MetricTrack(
		password.ResetDuration.WithLabelValues(password.LabelOpSendReset),
		password.ResetFailed.WithLabelValues(password.LabelOpSendReset),
		func() error {
			p.mu.RLock()
			subdomain := p.subdomain
			p.mu.RUnlock()

			if subdomain == "" {
				return errors.New("password reset service subdomain not configured")
			}

			token := core.GenerateSecurityToken()

			var reset models.PasswordReset

			reset.UserID = user.ID
			reset.Token = token
			reset.ExpiresAt = time.Now().Add(time.Hour)

			if err := db.RetryableComponentTransaction(p, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.Create(&reset)
			}); err != nil {
				return core.NewAccountError(core.ErrKeyDatabaseOperationFailed, err)
			}

			queryVars := url.Values{}
			queryVars.Set("email", user.Email)
			queryVars.Set("token", token)

			resetUrl := fmt.Sprintf("%s/reset-password/confirm?%s", fmt.Sprintf("https://%s.%s", subdomain, p.Config().Config().Core.Domain), queryVars.Encode())

			vars := map[string]interface{}{
				"FirstName":  user.FirstName,
				"Email":      user.Email,
				"ResetLink":  resetUrl,
				"ExpireTime": reset.ExpiresAt,
				"PortalName": p.Config().Config().Core.PortalName,
			}

			err := p.mailer.TemplateSend(core.MAILER_TPL_PASSWORD_RESET, vars, vars, user.Email)
			if err == nil {
				password.ResetSent.WithLabelValues(password.LabelOpSendReset).Inc()
			}
			return err
		},
	)
}

func (p PasswordResetServiceDefault) ResetPassword(ctx context.Context, email string, token string, newPassword string) error {
	ctx, span := core.TraceMethod(ctx, "PasswordResetServiceDefault.ResetPassword")
	defer span.End()

	return core.MetricTrack(
		password.ResetDuration.WithLabelValues(password.LabelOpReset),
		password.ResetFailed.WithLabelValues(password.LabelOpReset),
		func() error {
			var reset models.PasswordReset

			exists, user, err := p.user.EmailExists(ctx, email)
			if err != nil {
				return err
			}

			if !exists {
				return core.NewAccountError(core.ErrKeyUserNotFound, nil)
			}

			reset.Token = token
			reset.UserID = user.ID

			if err := db.RetryableComponentTransaction(p, ctx, func(tx *gorm.DB) *gorm.DB {
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

			passwordHash, err := p.user.HashPassword(newPassword)
			if err != nil {
				return err
			}

			err = p.user.UpdateAccountInfo(ctx, reset.UserID, map[string]interface{}{"password_hash": passwordHash})
			if err != nil {
				return err
			}

			reset = models.PasswordReset{
				UserID: reset.UserID,
			}

			if err := db.RetryableComponentTransaction(p, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.Where(&reset).Delete(&reset)
			}); err != nil {
				return core.NewAccountError(core.ErrKeyDatabaseOperationFailed, err)
			}

			password.PasswordReset.WithLabelValues(password.LabelOpReset).Inc()
			return nil
		},
	)
}
