package models

import (
	"errors"
	"time"

	emailverifier "github.com/AfterShip/email-verifier"
	"gorm.io/gorm"
)

func init() {
	registerModel(&User{})
}

type User struct {
	gorm.Model
	FirstName          string
	LastName           string
	Email              string `gorm:"unique"` // Keep unique for runtime validation
	PasswordHash       string
	Role               string
	KeyIdentities      []KeyIdentity
	Uploads            []Upload
	LastLogin          *time.Time
	LastLoginIP        string
	OTPEnabled         bool
	OTPVerified        bool
	OTPSecret          string
	OTPAuthUrl         string
	Verified           bool
	EmailVerifications []EmailVerification
	PasswordResets     []PasswordReset
}

func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.Email != "" {
		verify, err := getEmailVerifier().Verify(u.Email)
		if err != nil {
			return err
		}
		if !verify.Syntax.Valid {
			return errors.New("email is invalid")
		}
	}
	return nil
}

func (u *User) BeforeUpdate(tx *gorm.DB) error {
	var email string
	var changed bool

	switch dest := tx.Statement.Dest.(type) {
	case *User:
		email = dest.Email
		changed = tx.Statement.Changed("Email")
	case map[string]interface{}:
		if e, ok := dest["email"]; ok {
			if emailStr, ok := e.(string); ok {
				email = emailStr
				changed = true // Assume changed if present in the map
			}
		}
	default:
		// Handle other types or return an error if necessary
		return errors.New("unsupported destination type")
	}

	if changed && email != "" {
		verify, err := getEmailVerifier().Verify(email)
		if err != nil {
			return err
		}
		if !verify.Syntax.Valid {
			return errors.New("email is invalid")
		}
	}

	return nil
}

func getEmailVerifier() *emailverifier.Verifier {
	verifier := emailverifier.NewVerifier()

	verifier.DisableSMTPCheck()
	verifier.DisableGravatarCheck()
	verifier.DisableDomainSuggest()
	verifier.DisableAutoUpdateDisposable()

	return verifier
}
