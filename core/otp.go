package core

import (
	"errors"
	"github.com/pquerna/otp/totp"
)

const OTP_SERVICE = "otp"

var (
	ErrInvalidOTPCode = errors.New("Invalid OTP code")
)

type OTPService interface {
	// OTPGenerate generates a new OTP secret for the given user ID.
	// It returns the OTP secret and an error if any.
	OTPGenerate(userId uint) (string, error)

	// OTPVerify verifies the provided OTP code for the given user ID.
	// It returns a boolean indicating whether the code is valid, and an error if any.
	OTPVerify(userId uint, code string) (bool, error)

	// OTPEnable enables OTP for the given user ID after verifying the provided code.
	// It returns an error if any.
	OTPEnable(userId uint, code string) error

	// OTPDisable disables OTP for the given user ID.
	// It returns an error if any.
	OTPDisable(userId uint) error

	Service
}

func TOTPGenerate(domain string, email string) (string, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      domain,
		AccountName: email,
	})
	if err != nil {
		return "", err
	}

	return key.Secret(), nil
}

func TOTPValidate(secret string, code string) bool {
	return totp.Validate(code, secret)
}
