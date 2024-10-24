package core

import (
	"go.lumeweb.com/portal/db/models"
)

const USER_SERVICE = "user"

type UserService interface {
	// Exists checks if a record with the given conditions exists.
	Exists(model any, conditions map[string]any) (bool, any, error)

	// EmailExists checks if an email already exists in the system.
	EmailExists(email string) (bool, *models.User, error)

	// PubkeyExists checks if a public key already exists in the system.
	PubkeyExists(pubkey string) (bool, *models.PublicKey, error)

	// AccountExists checks if an account with the given ID exists.
	AccountExists(id uint) (bool, *models.User, error)

	// HashPassword hashes the provided password using bcrypt.
	HashPassword(password string) (string, error)

	// CreateAccount creates a new user account with the given email and password.
	CreateAccount(email string, password string, verifyEmail bool) (*models.User, error)

	// UpdateAccountInfo updates the account information of the user with the given ID.
	UpdateAccountInfo(userId uint, info map[string]any) error

	// UpdateAccountName updates the first and last name of the user with the given ID.
	UpdateAccountName(userId uint, firstName string, lastName string) error

	// UpdateAccountEmail updates the email of the user with the given ID after verifying the password.
	UpdateAccountEmail(userId uint, email string, password string) error

	// UpdateAccountPassword updates the password of the user with the given ID after verifying the old password.
	UpdateAccountPassword(userId uint, password string, newPassword string) error

	// AddPubkeyToAccount adds a public key to the account of the user with the given ID.
	AddPubkeyToAccount(user models.User, pubkey string) error

	// SendEmailVerification sends an email verification email to the user with the given ID.
	// It returns an error if any.
	SendEmailVerification(userId uint) error

	// VerifyEmail verifies the email for the given email address and token.
	// It returns an error if any.
	VerifyEmail(email string, token string) error

	// IsAccountVerified checks if the email of the user with the given ID is verified.
	IsAccountVerified(userId uint) (bool, error)

	// DeleteAccount deletes the account of the user with the given ID.
	DeleteAccount(userId uint) error

	// RequestAccountDeletion requests the deletion of the account of the user with the given ID.
	RequestAccountDeletion(userId uint, userIP string) error

	// IsAccountPendingDeletion checks if the account deletion is pending for the user with the given ID.
	IsAccountPendingDeletion(userId uint) (bool, error)

	// GetAccountsPendingDeletion returns a list of accounts that are pending deletion.
	GetAccountsPendingDeletion() ([]*models.User, error)

	Service
}
