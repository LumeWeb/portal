package middleware

import (
	"github.com/LumeWeb/portal/core"
	"github.com/golang-jwt/jwt/v5"
	"slices"
)

func jwtPurposeEqual(aud jwt.ClaimStrings, purpose core.JWTPurpose) bool {
	return slices.Contains[jwt.ClaimStrings, string](aud, string(purpose))
}
