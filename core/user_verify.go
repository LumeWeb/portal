package core

const EMAIL_VERIFICATION_SERVICE = "email_verification"

type EmailVerificationService interface {
	// SendEmailVerification sends an email verification email to the user with the given ID.
	// It returns an error if any.
	SendEmailVerification(userId uint) error

	// VerifyEmail verifies the email for the given email address and token.
	// It returns an error if any.
	VerifyEmail(email string, token string) error

	Service
}
