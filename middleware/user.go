package middleware

import (
	"context"
	"errors"
)

const DEFAULT_USER_ID_CONTEXT_KEY UserIdContextKeyType = "user_id"
const AUTH_TOKEN_CONTEXT_KEY AuthTokenContextKeyType = "auth_token"

var (
	ErrorUserContextInvalid      = errors.New("user id stored in context is not of type uint")
	ErrorAuthTokenContextInvalid = errors.New("auth token stored in context is not of type string")
)

func GetUserFromContext(ctx context.Context, key ...string) (uint, error) {
	realKey := ""

	if len(key) > 0 {
		realKey = key[0]
	}

	if realKey == "" {
		realKey = string(DEFAULT_USER_ID_CONTEXT_KEY)
	}

	realKeyCtx := UserIdContextKeyType(realKey)

	userId, ok := ctx.Value(realKeyCtx).(uint)

	if !ok {
		return 0, ErrorUserContextInvalid
	}

	return userId, nil
}

func GetAuthTokenFromContext(ctx context.Context) (string, error) {
	authToken, ok := ctx.Value(AUTH_TOKEN_CONTEXT_KEY).(string)

	if !ok {
		return "", ErrorAuthTokenContextInvalid
	}

	return authToken, nil
}
