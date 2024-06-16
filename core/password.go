package core

import "go.lumeweb.com/portal/db/models"

type PasswordResetService interface {
	// SendPasswordReset sends a password reset email to the given user.
	SendPasswordReset(user *models.User) error

	// ResetPassword resets the password for the given email, using the provided token and new password.
	ResetPassword(email string, token string, password string) error
}
