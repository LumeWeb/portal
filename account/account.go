package account

import (
	"crypto/ed25519"
	"errors"
	"git.lumeweb.com/LumeWeb/portal/db/models"
	"github.com/spf13/viper"
	"go.uber.org/fx"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"time"
)

var (
	ErrInvalidOTPCode = errors.New("Invalid OTP code")
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

func (s *AccountServiceDefault) EmailExists(email string) (bool, *models.User, error) {
	user := &models.User{}
	exists, model, err := s.exists(user, map[string]interface{}{"email": email})
	if !exists || err != nil {
		return false, nil, err
	}
	return true, model.(*models.User), nil // Type assertion since `exists` returns interface{}
}

func (s *AccountServiceDefault) PubkeyExists(pubkey string) (bool, *models.PublicKey, error) {
	publicKey := &models.PublicKey{}
	exists, model, err := s.exists(publicKey, map[string]interface{}{"key": pubkey})
	if !exists || err != nil {
		return false, nil, err
	}
	return true, model.(*models.PublicKey), nil // Type assertion is necessary
}

func (s *AccountServiceDefault) AccountExists(id uint) (bool, *models.User, error) {
	user := &models.User{}
	exists, model, err := s.exists(user, map[string]interface{}{"id": id})
	if !exists || err != nil {
		return false, nil, err
	}
	return true, model.(*models.User), nil // Ensure to assert the type correctly
}

func (s *AccountServiceDefault) HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func (s *AccountServiceDefault) CreateAccount(email string, password string) (*models.User, error) {
	passwordHash, err := s.HashPassword(password)
	if err != nil {
		return nil, err
	}

	user := models.User{
		Email:        email,
		PasswordHash: passwordHash,
	}

	result := s.db.Create(&user)
	if result.Error != nil {
		return nil, result.Error
	}

	return &user, nil
}

func (s AccountServiceDefault) UpdateAccountName(userId uint, firstName string, lastName string) error {
	return s.updateAccountInfo(userId, models.User{FirstName: firstName, LastName: lastName})
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
func (s AccountServiceDefault) LoginPassword(email string, password string, ip string) (string, *models.User, error) {
	valid, user, err := s.ValidLoginByEmail(email, password)

	if err != nil {
		return "", nil, err
	}

	if !valid {
		return "", nil, nil
	}

	token, err := s.doLogin(user, ip)

	if err != nil {
		return "", nil, err
	}

	return token, user, nil
}

func (s AccountServiceDefault) LoginOTP(userId uint, code string) (string, error) {
	valid, err := s.OTPVerify(userId, code)

	if err != nil {
		return "", err
	}

	if !valid {
		return "", ErrInvalidOTPCode
	}

	var user models.User
	user.ID = userId

	token, err := JWTGenerateToken(s.config.GetString("core.domain"), s.identity, user.ID, JWTPurposeLogin)
	if err != nil {
		return "", err
	}

	return token, nil
}

func (s AccountServiceDefault) ValidLoginByUserObj(user *models.User, password string) bool {
	return s.validPassword(user, password)
}

func (s AccountServiceDefault) ValidLoginByEmail(email string, password string) (bool, *models.User, error) {
	var user models.User

	result := s.db.Model(&models.User{}).Where(&models.User{Email: email}).First(&user)

	if result.RowsAffected == 0 || result.Error != nil {
		return false, nil, result.Error
	}

	valid := s.ValidLoginByUserObj(&user, password)

	if !valid {
		return false, nil, nil
	}

	return true, nil, nil
}

func (s AccountServiceDefault) ValidLoginByUserID(id uint, password string) (bool, *models.User, error) {
	var user models.User

	user.ID = id

	result := s.db.Model(&user).Where(&user).First(&user)

	if result.RowsAffected == 0 || result.Error != nil {
		return false, nil, result.Error
	}

	valid := s.ValidLoginByUserObj(&user, password)

	if !valid {
		return false, nil, nil
	}

	return true, &user, nil
}

func (s AccountServiceDefault) LoginPubkey(pubkey string) (string, error) {
	var model models.PublicKey

	result := s.db.Model(&models.PublicKey{}).Preload("User").Where(&models.PublicKey{Key: pubkey}).First(&model)

	if result.RowsAffected == 0 || result.Error != nil {
		return "", result.Error
	}

	user := model.User

	token, err := s.doLogin(&user, "")

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

func (s AccountServiceDefault) OTPGenerate(userId uint) (string, error) {
	exists, user, err := s.AccountExists(userId)

	if !exists || err != nil {
		return "", err
	}

	otp, err := TOTPGenerate(user.Email, s.config.GetString("core.domain"))
	if err != nil {
		return "", err
	}

	err = s.updateAccountInfo(user.ID, models.User{OTPSecret: otp})
	return otp, nil
}

func (s AccountServiceDefault) OTPVerify(userId uint, code string) (bool, error) {
	exists, user, err := s.AccountExists(userId)

	if !exists || err != nil {
		return false, err
	}

	valid := TOTPValidate(user.OTPSecret, code)
	if !valid {
		return false, nil
	}

	return true, nil
}

func (s AccountServiceDefault) OTPEnable(userId uint, code string) error {
	verify, err := s.OTPVerify(userId, code)
	if err != nil {
		return err
	}

	if !verify {
		return ErrInvalidOTPCode
	}

	return s.updateAccountInfo(userId, models.User{OTPEnabled: true})
}

func (s AccountServiceDefault) OTPDisable(userId uint) error {
	return s.updateAccountInfo(userId, models.User{OTPEnabled: false, OTPSecret: ""})
}

func (s AccountServiceDefault) doLogin(user *models.User, ip string) (string, error) {
	purpose := JWTPurposeLogin

	if user.OTPEnabled {
		purpose = JWTPurpose2FA
	}

	token, err := JWTGenerateToken(s.config.GetString("core.domain"), s.identity, user.ID, purpose)
	if err != nil {
		return "", err
	}

	now := time.Now()

	err = s.updateAccountInfo(user.ID, models.User{LastLoginIP: ip, LastLogin: &now})
	if err != nil {
		return "", err
	}

	return token, nil
}

func (s AccountServiceDefault) updateAccountInfo(userId uint, info interface{}) error {
	var user models.User

	user.ID = userId

	result := s.db.Model(&models.User{}).Where(&user).Updates(info)

	if result.Error != nil {
		return result.Error
	}

	return nil
}

func (s AccountServiceDefault) exists(model interface{}, conditions map[string]interface{}) (bool, interface{}, error) {
	// Conduct a query with the provided model and conditions
	result := s.db.Model(model).Where(conditions).First(model)

	// Check if any rows were found
	exists := result.RowsAffected > 0

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return false, nil, nil
	}

	return exists, model, result.Error
}

func (s AccountServiceDefault) validPassword(user *models.User, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))

	return err == nil
}
