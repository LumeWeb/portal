package validators

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/imdario/mergo"
	"reflect"
)

func CheckPubkeyValidator(value interface{}) error {
	p, _ := value.(string)
	pubkeyBytes, err := hex.DecodeString(p)
	if err != nil {
		return err
	}

	if len(pubkeyBytes) != ed25519.PublicKeySize {
		return errors.New(fmt.Sprintf("pubkey must be %d bytes in hexadecimal format", ed25519.PublicKeySize))
	}

	return nil
}

type Validatable interface {
	validation.Validatable
	Import(d map[string]interface{}) (Validatable, error)
}

type ValidatableImpl struct {
}

func (v ValidatableImpl) Import(d map[string]interface{}, destType Validatable) (Validatable, error) {
	instance := reflect.New(reflect.TypeOf(destType)).Interface().(Validatable)
	// Perform the import logic
	if err := mergo.Map(instance, d, mergo.WithOverride); err != nil {
		return nil, err
	}

	return instance, nil
}
