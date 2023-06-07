package request

import (
	"git.lumeweb.com/LumeWeb/portal/controller/validators"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

type PubkeyLoginRequest struct {
	validatable validators.ValidatableImpl
	Pubkey      string `json:"pubkey"`
	Challenge   string `json:"challenge"`
	Signature   string `json:"signature"`
}

func (r PubkeyLoginRequest) Validate() error {
	return validation.ValidateStruct(&r,
		validation.Field(&r.Pubkey, validation.Required, validation.By(validators.CheckPubkeyValidator)),
		validation.Field(&r.Challenge, validation.Required),
		validation.Field(&r.Signature, validation.Required, validation.Length(128, 128)),
	)
}

func (r PubkeyLoginRequest) Import(d map[string]interface{}) (validators.Validatable, error) {
	return r.validatable.Import(d, r)
}
