package core

import (
	"context"

	"go.lumeweb.com/portal/db/models"
)

const PASSWORD_RESET_SERVICE = "password_reset"

type PasswordResetService interface {
	// SendPasswordReset sends a password reset email to the given user.
	SendPasswordReset(ctx context.Context, user *models.User) error

	// ResetPassword resets the password for the given email, using the provided token and new password.
	ResetPassword(ctx context.Context, email string, token string, password string) error

	Service
}
