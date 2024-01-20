package account

import (
	"git.lumeweb.com/LumeWeb/portal/db/models"
	"git.lumeweb.com/LumeWeb/portal/interfaces"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	_ interfaces.AccountService = (*AccountServiceImpl)(nil)
)

type AccountServiceImpl struct {
	portal interfaces.Portal
}

func NewAccountService(portal interfaces.Portal) interfaces.AccountService {
	return &AccountServiceImpl{portal: portal}
}

func (s AccountServiceImpl) EmailExists(email string) (bool, models.User) {
	var user models.User

	result := s.portal.Database().Model(&models.User{}).Where(&models.User{Email: email}).First(&user)

	return result.RowsAffected > 0, user
}
func (s AccountServiceImpl) PubkeyExists(pubkey string) (bool, models.PublicKey) {
	var model models.PublicKey

	result := s.portal.Database().Model(&models.PublicKey{}).Where(&models.PublicKey{Key: pubkey}).First(&model)

	return result.RowsAffected > 0, model
}

func (s AccountServiceImpl) AccountExists(id uint64) (bool, models.User) {
	var model models.User

	result := s.portal.Database().Model(&models.User{}).First(&model, id)

	return result.RowsAffected > 0, model
}
func (s AccountServiceImpl) CreateAccount(email string, password string) (*models.User, error) {
	var user models.User

	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user.Email = email
	user.PasswordHash = string(bytes)

	result := s.portal.Database().Create(&user)

	if result.Error != nil {
		return nil, result.Error
	}

	return &user, nil
}
func (s AccountServiceImpl) AddPubkeyToAccount(user models.User, pubkey string) error {
	var model models.PublicKey

	model.Key = pubkey
	model.UserID = user.ID

	result := s.portal.Database().Create(&model)

	if result.Error != nil {
		return result.Error
	}

	return nil
}
func (s AccountServiceImpl) LoginPassword(email string, password string) (string, error) {
	var user models.User

	result := s.portal.Database().Model(&models.User{}).Where(&models.User{Email: email}).First(&user)

	if result.RowsAffected == 0 || result.Error != nil {
		return "", result.Error
	}

	err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		return "", err
	}

	token, err := GenerateToken(s.portal.Identity(), user.ID)
	if err != nil {
		return "", err
	}

	return token, nil
}

func (s AccountServiceImpl) LoginPubkey(pubkey string) (string, error) {
	var model models.PublicKey

	result := s.portal.Database().Model(&models.PublicKey{}).Where(&models.PublicKey{Key: pubkey}).First(&model)

	if result.RowsAffected == 0 || result.Error != nil {
		return "", result.Error
	}

	token, err := GenerateToken(s.portal.Identity(), model.UserID)
	if err != nil {
		return "", err
	}

	return token, nil
}

func (s AccountServiceImpl) AccountPins(id uint64, createdAfter uint64) ([]models.Pin, error) {
	var pins []models.Pin

	result := s.portal.Database().Model(&models.Pin{}).Where(&models.Pin{UserID: uint(id)}).Where("created_at > ?", createdAfter).Order("created_at desc").Find(&pins)

	if result.Error != nil {
		return nil, result.Error
	}

	return pins, nil
}

func (s AccountServiceImpl) DeletePinByHash(hash string, accountID uint) error {
	// Define a struct for the query condition
	uploadQuery := models.Upload{Hash: hash}

	// Retrieve the upload ID for the given hash
	var uploadID uint
	result := s.portal.Database().
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
	result = s.portal.Database().
		Where(&pinQuery).
		Delete(&models.Pin{})

	if result.Error != nil {
		return result.Error
	}

	return nil
}
func (s AccountServiceImpl) PinByHash(hash string, accountID uint) error {
	// Define a struct for the query condition
	uploadQuery := models.Upload{Hash: hash}

	// Retrieve the upload ID for the given hash
	var uploadID uint
	result := s.portal.Database().
		Model(&models.Upload{}).
		Where(&uploadQuery).
		First(&uploadID)

	if result.Error != nil {
		return result.Error
	}

	return s.PinByID(uploadID, accountID)
}

func (s AccountServiceImpl) PinByID(uploadId uint, accountID uint) error {
	result := s.portal.Database().Model(&models.Pin{}).Where(&models.Pin{UploadID: uploadId, UserID: accountID}).First(&models.Pin{})

	if result.Error != nil && result.Error != gorm.ErrRecordNotFound {
		return result.Error
	}

	if result.RowsAffected > 0 {
		return nil
	}

	// Create a pin with the retrieved upload ID and matching account ID
	pinQuery := models.Pin{UploadID: uploadId, UserID: accountID}
	result = s.portal.Database().Create(&pinQuery)

	if result.Error != nil {
		return result.Error
	}

	return nil
}
