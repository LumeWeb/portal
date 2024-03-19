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
	Email              string `gorm:"unique"`
	PasswordHash       string
	Role               string
	PublicKeys         []PublicKey
	APIKeys            []APIKey
	Uploads            []Upload
	LastLogin          *time.Time
	LastLoginIP        string
	OTPEnabled         bool `gorm:"default:false;"`
	OTPVerified        bool `gorm:"default:false;"`
	OTPSecret          string
	OTPAuthUrl         string
	Verified           bool `gorm:"default:false;"`
	EmailVerifications []EmailVerification
	PasswordResets     []PasswordReset
}

func (u *User) BeforeUpdate(tx *gorm.DB) error {
	dest := tx.Statement.Dest.(User)

	if tx.Statement.Changed("Email") {
		verify, err := getEmailVerfier().Verify(dest.Email)
		if err != nil {
			return err
		}
		if !verify.Syntax.Valid {
			return errors.New("email is invalid")
		}
	}

	return nil
}

func getEmailVerfier() *emailverifier.Verifier {
	verifier := emailverifier.NewVerifier()

	verifier.DisableSMTPCheck()
	verifier.DisableGravatarCheck()
	verifier.DisableDomainSuggest()
	verifier.DisableAutoUpdateDisposable()

	return verifier
}
