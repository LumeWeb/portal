package models

import (
	oautgorm "go.lumeweb.com/oauth/storage/gorm"
)

func init() {
	registerModel(&oautgorm.OAuthClient{})
	registerModel(&oautgorm.OAuthAuthorizationCode{})
	registerModel(&oautgorm.OAuthRefreshToken{})
	registerModel(&oautgorm.OAuthAccessToken{})
}
