package account

import "github.com/pquerna/otp/totp"

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
