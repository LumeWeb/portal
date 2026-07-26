package models

import (
	"encoding/json"

	"gorm.io/gorm"
)

func init() {
	registerModel(&KeyIdentity{})
}

// KeyIdentity represents a cryptographic key linked to a user account.
// The Type field is the registry key (e.g., "ethereum", "solana", "nostr",
// "webauthn"). The Key field is the canonical key string (e.g., lowercase
// ETH address, npub, credential ID). Metadata stores type-specific data
// as JSON (chain_id, relays, transports, etc.).
//
// UNIQUE(Type, Key) allows the same key string on different types to
// belong to different users (e.g., an ETH address and a Solana pubkey
// might collide as strings). Within a type, a key maps to exactly one user.
type KeyIdentity struct {
	gorm.Model
	UserID   uint            `gorm:"not null;index" json:"user_id"`
	User     User            `gorm:"foreignKey:UserID" json:"-"`
	Type     string          `gorm:"not null;size:50;index;default:ethereum" json:"type"`
	Key      string          `gorm:"not null;size:255" json:"key"`
	Metadata json.RawMessage `gorm:"type:json" json:"metadata,omitempty"`
}

func (KeyIdentity) TableName() string {
	return "key_identities"
}
