package interfaces

import "git.lumeweb.com/LumeWeb/portal/db/models"

type AccountService interface {
	EmailExists(email string) (bool, models.User)
	PubkeyExists(pubkey string) (bool, models.PublicKey)
	AccountExists(id uint64) (bool, models.User)
	CreateAccount(email string, password string) (*models.User, error)
	AddPubkeyToAccount(user models.User, pubkey string) error
	LoginPassword(email string, password string) (string, error)
	LoginPubkey(pubkey string) (string, error)
	AccountPins(id uint64, createdAfter uint64) ([]models.Pin, error)
	DeletePinByHash(hash string, accountID uint) error
}
