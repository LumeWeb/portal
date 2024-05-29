package types

import (
	"crypto/ed25519"
	"errors"
	"github.com/go-viper/mapstructure/v2"
	"go.sia.tech/renterd/wallet"
	"gopkg.in/yaml.v3"
)

var _ yaml.Marshaler = (*Identity)(nil)
var _ mapstructure.Unmarshaler = (*Identity)(nil)

type Identity struct {
	seed string
	key  ed25519.PrivateKey
}

func (i Identity) Valid() bool {
	err := i.derive()
	if err != nil {
		return false
	}

	return true
}

func (i Identity) derive() error {
	key, err := wallet.KeyFromPhrase(i.seed)

	if err != nil {
		return err
	}

	i.key = ed25519.PrivateKey(key)

	return nil
}

func (i Identity) Derive() error {
	return i.derive()
}

func (i Identity) PrivateKey() ed25519.PrivateKey {
	if i.key == nil || len(i.key) != ed25519.PrivateKeySize {
		err := i.derive()
		if err != nil {
			panic(err)
		}
	}

	return i.key
}

func (i Identity) PublicKey() ed25519.PublicKey {
	sk := i.PrivateKey()

	return sk.Public().(ed25519.PublicKey)
}

func (i Identity) MarshalYAML() (interface{}, error) {
	return i.seed, nil
}

func (i *Identity) DecodeMapstructure(value interface{}) error {
	if _, ok := value.(string); !ok {
		return errors.New("identity must be a string")
	}

	identity := NewIdentityFromSeed(value.(string))
	err := identity.Derive()
	if err != nil {
		return err
	}

	*i = *identity

	return nil
}

func NewIdentityFromSeed(seed string) *Identity {
	return &Identity{
		seed: seed,
	}
}
