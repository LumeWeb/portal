package request

import (
	"git.lumeweb.com/LumeWeb/portal/controller/validators"
	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"
)

type RegisterRequest struct {
	validatable validators.ValidatableImpl
	Email       string `json:"email"`
	Password    string `json:"password"`
	Pubkey      string `json:"pubkey"`
}

func (r RegisterRequest) Validate() error {
	return validation.ValidateStruct(&r,
		validation.Field(&r.Email, validation.Required, is.EmailFormat),
		validation.Field(&r.Pubkey, validation.When(len(r.Password) == 0, validation.Required, validation.By(validators.CheckPubkeyValidator))),
		validation.Field(&r.Password, validation.When(len(r.Pubkey) == 0, validation.Required)),
	)
}
func (r RegisterRequest) Import(d map[string]interface{}) (validators.Validatable, error) {
	return r.validatable.Import(d, r)
}
