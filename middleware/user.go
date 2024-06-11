package middleware

import "context"

const DEFAULT_USER_ID_CONTEXT_KEY UserIdContextKeyType = "user_id"
const AUTH_TOKEN_CONTEXT_KEY AuthTokenContextKeyType = "auth_token"

func GetUserFromContext(ctx context.Context, key ...string) uint {
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
		panic("user id stored in context is not of type uint")
	}

	return userId
}

func GetAuthTokenFromContext(ctx context.Context) string {
	authToken, ok := ctx.Value(AUTH_TOKEN_CONTEXT_KEY).(string)

	if !ok {
		panic("auth token stored in context is not of type string")
	}

	return authToken
}
