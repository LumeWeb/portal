package models

import (
	"gorm.io/gorm"
)

func init() {
	registerModel(&SocialAccount{})
}

// SocialAccount links a portal user to an external identity (e.g. Google,
// GitHub). Provider is the IdP key (e.g. "google"); ProviderUserID is the
// IdP's stable user identifier. Email is a denormalized convenience value
// used for conflict checks — it is not the account identity.
//
// UNIQUE(Provider, ProviderUserID) means one external identity maps to
// exactly one portal user. The schema (constraints, indexes) is defined in
// the SQL migrations, not via gorm tags.
type SocialAccount struct {
	gorm.Model
	UserID         uint   `json:"user_id"`
	User           User   `gorm:"foreignKey:UserID" json:"-"`
	Provider       string `json:"provider"`
	ProviderUserID string `json:"provider_user_id"`
	Email          string `json:"email"`
}

func (SocialAccount) TableName() string {
	return "social_accounts"
}
