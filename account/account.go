package account

import (
	"crypto/ed25519"
	"git.lumeweb.com/LumeWeb/portal/db/models"
	"github.com/spf13/viper"
	"go.uber.org/fx"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type AccountServiceParams struct {
	fx.In
	Db       *gorm.DB
	Config   *viper.Viper
	Identity ed25519.PrivateKey
}

var Module = fx.Module("account",
	fx.Options(
		fx.Provide(NewAccountService),
	),
)

type AccountServiceDefault struct {
	db       *gorm.DB
	config   *viper.Viper
	identity ed25519.PrivateKey
}

func NewAccountService(params AccountServiceParams) *AccountServiceDefault {
	return &AccountServiceDefault{db: params.Db, config: params.Config, identity: params.Identity}
}

func (s AccountServiceDefault) EmailExists(email string) (bool, models.User) {
	var user models.User

	result := s.db.Model(&models.User{}).Where(&models.User{Email: email}).First(&user)

	return result.RowsAffected > 0, user
}
func (s AccountServiceDefault) PubkeyExists(pubkey string) (bool, models.PublicKey) {
	var model models.PublicKey

	result := s.db.Model(&models.PublicKey{}).Where(&models.PublicKey{Key: pubkey}).First(&model)

	return result.RowsAffected > 0, model
}

func (s AccountServiceDefault) AccountExists(id uint64) (bool, models.User) {
	var model models.User

	result := s.db.Model(&models.User{}).First(&model, id)

	return result.RowsAffected > 0, model
}
func (s AccountServiceDefault) CreateAccount(email string, password string) (*models.User, error) {
	var user models.User

	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user.Email = email
	user.PasswordHash = string(bytes)

	result := s.db.Create(&user)

	if result.Error != nil {
		return nil, result.Error
	}

	return &user, nil
}
func (s AccountServiceDefault) AddPubkeyToAccount(user models.User, pubkey string) error {
	var model models.PublicKey

	model.Key = pubkey
	model.UserID = user.ID

	result := s.db.Create(&model)

	if result.Error != nil {
		return result.Error
	}

	return nil
}
func (s AccountServiceDefault) LoginPassword(email string, password string) (string, error) {
	var user models.User

	result := s.db.Model(&models.User{}).Where(&models.User{Email: email}).First(&user)

	if result.RowsAffected == 0 || result.Error != nil {
		return "", result.Error
	}

	err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		return "", err
	}

	token, err := GenerateToken(s.identity, user.ID)
	if err != nil {
		return "", err
	}

	return token, nil
}

func (s AccountServiceDefault) LoginPubkey(pubkey string) (string, error) {
	var model models.PublicKey

	result := s.db.Model(&models.PublicKey{}).Where(&models.PublicKey{Key: pubkey}).First(&model)

	if result.RowsAffected == 0 || result.Error != nil {
		return "", result.Error
	}

	token, err := GenerateToken(s.identity, model.UserID)
	if err != nil {
		return "", err
	}

	return token, nil
}

func (s AccountServiceDefault) AccountPins(id uint64, createdAfter uint64) ([]models.Pin, error) {
	var pins []models.Pin

	result := s.db.Model(&models.Pin{}).
		Preload("Upload"). // Preload the related Upload for each Pin
		Where(&models.Pin{UserID: uint(id)}).
		Where("created_at > ?", createdAfter).
		Order("created_at desc").
		Find(&pins)

	if result.Error != nil {
		return nil, result.Error
	}

	return pins, nil
}

func (s AccountServiceDefault) DeletePinByHash(hash string, accountID uint) error {
	// Define a struct for the query condition
	uploadQuery := models.Upload{Hash: hash}

	// Retrieve the upload ID for the given hash
	var uploadID uint
	result := s.db.
		Model(&models.Upload{}).
		Where(&uploadQuery).
		Select("id").
		First(&uploadID)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			// No record found, nothing to delete
			return nil
		}
		return result.Error
	}

	// Delete pins with the retrieved upload ID and matching account ID
	pinQuery := models.Pin{UploadID: uploadID, UserID: accountID}
	result = s.db.
		Where(&pinQuery).
		Delete(&models.Pin{})

	if result.Error != nil {
		return result.Error
	}

	return nil
}
func (s AccountServiceDefault) PinByHash(hash string, accountID uint) error {
	// Define a struct for the query condition
	uploadQuery := models.Upload{Hash: hash}

	// Retrieve the upload ID for the given hash
	var uploadID uint
	result := s.db.
		Model(&models.Upload{}).
		Where(&uploadQuery).
		First(&uploadID)

	if result.Error != nil {
		return result.Error
	}

	return s.PinByID(uploadID, accountID)
}

func (s AccountServiceDefault) PinByID(uploadId uint, accountID uint) error {
	result := s.db.Model(&models.Pin{}).Where(&models.Pin{UploadID: uploadId, UserID: accountID}).First(&models.Pin{})

	if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
		return result.Error
	}

	if result.RowsAffected > 0 {
		return nil
	}

	// Create a pin with the retrieved upload ID and matching account ID
	pinQuery := models.Pin{UploadID: uploadId, UserID: accountID}
	result = s.db.Create(&pinQuery)

	if result.Error != nil {
		return result.Error
	}

	return nil
}
