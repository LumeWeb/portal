package account

import (
	"git.lumeweb.com/LumeWeb/portal/db/models"
	"git.lumeweb.com/LumeWeb/portal/interfaces"
	"golang.org/x/crypto/bcrypt"
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

	token, err := generateToken(s.portal.Identity(), user.ID)
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

	token, err := generateToken(s.portal.Identity(), model.UserID)
	if err != nil {
		return "", err
	}

	return token, nil
}
