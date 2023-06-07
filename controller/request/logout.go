package request

import (
	"git.lumeweb.com/LumeWeb/portal/controller/validators"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

type LogoutRequest struct {
	validatable validators.ValidatableImpl
	Token       string `json:"token"`
}

func (r LogoutRequest) Validate() error {
	return validation.ValidateStruct(&r, validation.Field(&r.Token, validation.Required))
}

func (r LogoutRequest) Import(d map[string]interface{}) (validators.Validatable, error) {
	return r.validatable.Import(d, r)
}
