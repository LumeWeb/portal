package request

import (
	"git.lumeweb.com/LumeWeb/portal/controller/validators"
	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"
)

type LoginRequest struct {
	validatable validators.ValidatableImpl
	Email       string `json:"email"`
	Password    string `json:"password"`
}

func (r LoginRequest) Validate() error {
	return validation.ValidateStruct(&r,
		validation.Field(&r.Email, is.EmailFormat, validation.Required),
		validation.Field(&r.Password, validation.Required),
	)
}
func (r LoginRequest) Import(d map[string]interface{}) (validators.Validatable, error) {
	return r.validatable.Import(d, r)
}
