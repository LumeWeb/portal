package request

import (
	"git.lumeweb.com/LumeWeb/portal/controller/validators"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

type PubkeyChallengeRequest struct {
	validatable validators.ValidatableImpl
	Pubkey      string `json:"pubkey"`
}

func (r PubkeyChallengeRequest) Validate() error {
	return validation.ValidateStruct(&r,
		validation.Field(&r.Pubkey, validation.Required, validation.By(validators.CheckPubkeyValidator)),
	)
}

func (r PubkeyChallengeRequest) Import(d map[string]interface{}) (validators.Validatable, error) {
	return r.validatable.Import(d, r)
}
