package models

import (
	"errors"
	"strings"
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

// reservedTLDs lists TLDs that are guaranteed non-routable (RFC 2606 /
// RFC 6761 / RFC 6762). Accounts with such emails, like the generated anon
// wallet accounts, have no mailbox, so the MX lookup performed by the email
// verifier can never succeed and must be skipped.
var reservedTLDs = []string{".invalid", ".localhost", ".test", ".example"}

func isReservedDomain(email string) bool {
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return false
	}
	domain := strings.ToLower(email[at:])
	for _, tld := range reservedTLDs {
		if strings.HasSuffix(domain, tld) {
			return true
		}
	}
	return false
}

// verifyUserEmail validates an email address before it is persisted. Real
// domains get the full verification, which includes a MX lookup; reserved
// non-routable domains are syntax-checked only, since no mail server can ever
// exist for them.
func verifyUserEmail(email string) error {
	if email == "" {
		return nil
	}
	verifier := getEmailVerifier()

	if isReservedDomain(email) {
		if !verifier.ParseAddress(email).Valid {
			return errors.New("email is invalid")
		}
		return nil
	}

	verify, err := verifier.Verify(email)
	if err != nil {
		return err
	}
	if !verify.Syntax.Valid {
		return errors.New("email is invalid")
	}
	return nil
}

func (u *User) BeforeCreate(tx *gorm.DB) error {
	return verifyUserEmail(u.Email)
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

	if changed {
		return verifyUserEmail(email)
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
