package models

import (
	"errors"
	emailverifier "github.com/AfterShip/email-verifier"
	"gorm.io/gorm"
	"time"
)

type User struct {
	gorm.Model
	FirstName    string
	LastName     string
	Email        string `gorm:"unique"`
	PasswordHash string
	Role         string
	PublicKeys   []PublicKey
	APIKeys      []APIKey
	Uploads      []Upload
	LastLogin    *time.Time
	LastLoginIP  string
	OTPEnabled   bool `gorm:"default:false;"`
	OTPVerified  bool `gorm:"default:false;"`
	OTPSecret    string
	OTPAuthUrl   string
}

func (u *User) BeforeUpdate(tx *gorm.DB) error {
	if len(u.FirstName) == 0 {
		return errors.New("first name is empty")
	}

	if len(u.LastName) == 0 {
		return errors.New("last name is empty")
	}

	verify, err := getEmailVerfier().Verify(u.Email)

	if err != nil {
		return err
	}

	if !verify.Syntax.Valid {
		return errors.New("email is invalid")
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
