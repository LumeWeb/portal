package middleware

import (
	"github.com/golang-jwt/jwt/v5"
	"go.lumeweb.com/portal/core"
	"slices"
)

func jwtPurposeEqual(aud jwt.ClaimStrings, purpose core.JWTPurpose) bool {
	return slices.Contains[jwt.ClaimStrings, string](aud, string(purpose))
}
