package jwt

import (
	"github.com/kataras/iris/v12/middleware/jwt"
	_ "github.com/kataras/iris/v12/middleware/jwt"
)

var (
	Secret = []byte("signature_hmac_secret_shared_key")
	v      *jwt.Verifier
)

func init() {
	v = jwt.NewVerifier(jwt.HS256, Secret)
	v.WithDefaultBlocklist()
}

func Get() *jwt.Verifier {
	return v
}
