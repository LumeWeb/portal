package account

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"time"

	"git.lumeweb.com/LumeWeb/portal/metadata"

	"git.lumeweb.com/LumeWeb/portal/mailer"

	"gorm.io/gorm/clause"

	"git.lumeweb.com/LumeWeb/portal/config"

	"git.lumeweb.com/LumeWeb/portal/db/models"
	"go.uber.org/fx"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var (
	ErrInvalidOTPCode = errors.New("Invalid OTP code")
)

type AccountServiceParams struct {
	fx.In
	Db       *gorm.DB
	Config   *config.Manager
	Identity ed25519.PrivateKey
	Mailer   *mailer.Mailer
	Metadata metadata.MetadataService
}

var Module = fx.Module("account",
	fx.Options(
		fx.Provide(NewAccountService),
	),
)

type AccountServiceDefault struct {
	db       *gorm.DB
	config   *config.Manager
	identity ed25519.PrivateKey
	mailer   *mailer.Mailer
	metadata metadata.MetadataService
}

func NewAccountService(params AccountServiceParams) *AccountServiceDefault {
	return &AccountServiceDefault{db: params.Db, config: params.Config, identity: params.Identity, mailer: params.Mailer, metadata: params.Metadata}
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
		return "", NewAccountError(ErrKeyHashingFailed, err)
	}
	return string(bytes), nil
}

func (s *AccountServiceDefault) CreateAccount(email string, password string, verifyEmail bool) (*models.User, error) {
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
		if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
			return nil, NewAccountError(ErrKeyEmailAlreadyExists, nil)
		}
		return nil, NewAccountError(ErrKeyAccountCreationFailed, result.Error)
	}

	if verifyEmail {
		err = s.SendEmailVerification(&user)
		if err != nil {
			return nil, err
		}
	}

	return &user, nil
}

func (s *AccountServiceDefault) SendEmailVerification(user *models.User) error {
	token := GenerateSecurityToken()

	var verification models.EmailVerification

	verification.UserID = user.ID
	verification.Token = token
	verification.ExpiresAt = time.Now().Add(time.Hour)

	err := s.db.Create(&verification).Error
	if err != nil {
		return NewAccountError(ErrKeyDatabaseOperationFailed, err)
	}

	vars := map[string]interface{}{
		"FirstName":        user.FirstName,
		"Email":            user.Email,
		"VerificationCode": token,
		"ExpireTime":       verification.ExpiresAt.Sub(time.Now()).Round(time.Second * 2),
		"PortalName":       s.config.Config().Core.PortalName,
	}

	return s.mailer.TemplateSend(mailer.TPL_VERIFY_EMAIL, vars, vars, user.Email)
}

func (s AccountServiceDefault) SendPasswordReset(user *models.User) error {
	token := GenerateSecurityToken()

	var reset models.PasswordReset

	reset.UserID = user.ID
	reset.Token = token
	reset.ExpiresAt = time.Now().Add(time.Hour)

	err := s.db.Create(&reset).Error
	if err != nil {
		return NewAccountError(ErrKeyDatabaseOperationFailed, err)
	}

	vars := map[string]interface{}{
		"FirstName":    user.FirstName,
		"Email":        user.Email,
		"ResetCode":    token,
		"ExpireTime":   reset.ExpiresAt,
		"PortalName":   s.config.Config().Core.PortalName,
		"PortalDomain": s.config.Config().Core.Domain,
	}

	return s.mailer.TemplateSend(mailer.TPL_PASSWORD_RESET, vars, vars, user.Email)
}

func (s AccountServiceDefault) VerifyEmail(email string, token string) error {
	var verification models.EmailVerification

	verification.Token = token

	result := s.db.Model(&verification).
		Preload("User").
		Where(&verification).
		First(&verification)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return NewAccountError(ErrKeyUserNotFound, result.Error)
		}

		return NewAccountError(ErrKeyDatabaseOperationFailed, result.Error)
	}

	if verification.ExpiresAt.Before(time.Now()) {
		return NewAccountError(ErrKeySecurityTokenExpired, nil)
	}

	if len(verification.NewEmail) > 0 && verification.NewEmail != email {
		return NewAccountError(ErrKeySecurityInvalidToken, nil)
	} else if verification.User.Email != email {
		return NewAccountError(ErrKeySecurityInvalidToken, nil)
	}

	var update models.User

	doUpdate := false

	if !verification.User.Verified {
		update.Verified = true
		doUpdate = true
	}

	if len(verification.NewEmail) > 0 {
		update.Email = verification.NewEmail
		doUpdate = true
	}

	if doUpdate {
		err := s.updateAccountInfo(verification.UserID, update)
		if err != nil {
			return err
		}
	}

	verification = models.EmailVerification{
		UserID: verification.UserID,
	}

	if result := s.db.Where(&verification).Delete(&verification); result.Error != nil {
		return NewAccountError(ErrKeyDatabaseOperationFailed, result.Error)
	}

	return nil
}

func (s AccountServiceDefault) ResetPassword(email string, token string, password string) error {
	var reset models.PasswordReset

	reset.Token = token

	result := s.db.Model(&reset).
		Preload("User").
		Where(&reset).
		First(&reset)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return NewAccountError(ErrKeyUserNotFound, result.Error)
		}

		return NewAccountError(ErrKeyDatabaseOperationFailed, result.Error)
	}

	if reset.ExpiresAt.Before(time.Now()) {
		return NewAccountError(ErrKeySecurityTokenExpired, nil)
	}

	if reset.User.Email != email {
		return NewAccountError(ErrKeySecurityInvalidToken, nil)
	}

	passwordHash, err := s.HashPassword(password)
	if err != nil {
		return err
	}

	err = s.updateAccountInfo(reset.UserID, models.User{PasswordHash: passwordHash})
	if err != nil {
		return err
	}

	reset = models.PasswordReset{
		UserID: reset.UserID,
	}

	if result := s.db.Where(&reset).Delete(&reset); result.Error != nil {
		return NewAccountError(ErrKeyDatabaseOperationFailed, result.Error)
	}

	return nil
}

func (s AccountServiceDefault) UpdateAccountName(userId uint, firstName string, lastName string) error {
	return s.updateAccountInfo(userId, models.User{FirstName: firstName, LastName: lastName})
}

func (s AccountServiceDefault) UpdateAccountEmail(userId uint, email string, password string) error {
	exists, euser, err := s.EmailExists(email)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) || (exists && euser.ID != userId) {
		return NewAccountError(ErrKeyEmailAlreadyExists, nil)
	}

	valid, user, err := s.ValidLoginByUserID(userId, password)
	if err != nil {
		return err
	}

	if !valid {
		return NewAccountError(ErrKeyInvalidLogin, nil)
	}

	if user.Email == email {
		return NewAccountError(ErrKeyUpdatingSameEmail, nil)
	}

	var update models.User

	update.Email = email

	return s.updateAccountInfo(userId, update)
}

func (s AccountServiceDefault) UpdateAccountPassword(userId uint, password string, newPassword string) error {
	valid, _, err := s.ValidLoginByUserID(userId, password)
	if err != nil {
		return err
	}

	if !valid {
		return NewAccountError(ErrKeyInvalidPassword, nil)
	}

	passwordHash, err := s.HashPassword(newPassword)
	if err != nil {
		return err
	}

	return s.updateAccountInfo(userId, models.User{PasswordHash: passwordHash})
}

func (s AccountServiceDefault) AddPubkeyToAccount(user models.User, pubkey string) error {
	var model models.PublicKey

	model.Key = pubkey
	model.UserID = user.ID

	result := s.db.Create(&model)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
			return NewAccountError(ErrKeyPublicKeyExists, result.Error)
		}

		return NewAccountError(ErrKeyDatabaseOperationFailed, result.Error)
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

	token, err := s.doLogin(user, ip, false)

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
		return "", NewAccountError(ErrKeyInvalidOTPCode, nil)
	}

	var user models.User
	user.ID = userId

	token, tokenErr := JWTGenerateToken(s.config.Config().Core.Domain, s.identity, user.ID, JWTPurposeLogin)
	if tokenErr != nil {
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
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return false, nil, NewAccountError(ErrKeyInvalidLogin, result.Error)
		}

		return false, nil, NewAccountError(ErrKeyDatabaseOperationFailed, result.Error)
	}

	valid := s.ValidLoginByUserObj(&user, password)

	if !valid {
		return false, nil, nil
	}

	return true, &user, nil
}

func (s AccountServiceDefault) ValidLoginByUserID(id uint, password string) (bool, *models.User, error) {
	var user models.User

	user.ID = id

	result := s.db.Model(&user).Where(&user).First(&user)

	if result.RowsAffected == 0 || result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return false, nil, NewAccountError(ErrKeyInvalidLogin, result.Error)
		}

		return false, nil, NewAccountError(ErrKeyDatabaseOperationFailed, result.Error)
	}

	valid := s.ValidLoginByUserObj(&user, password)

	if !valid {
		return false, nil, nil
	}

	return true, &user, nil
}

func (s AccountServiceDefault) LoginPubkey(pubkey string, ip string) (string, error) {
	var model models.PublicKey

	result := s.db.Model(&models.PublicKey{}).Preload("User").Where(&models.PublicKey{Key: pubkey}).First(&model)

	if result.RowsAffected == 0 || result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return "", NewAccountError(ErrKeyInvalidLogin, result.Error)
		}

		return "", NewAccountError(ErrKeyDatabaseOperationFailed, result.Error)
	}

	user := model.User

	token, err := s.doLogin(&user, ip, true)

	if err != nil {
		return "", err
	}

	return token, nil
}

func (s AccountServiceDefault) AccountPins(id uint, createdAfter uint64) ([]models.Pin, error) {
	var pins []models.Pin

	result := s.db.Model(&models.Pin{}).
		Preload("Upload"). // Preload the related Upload for each Pin
		Where(&models.Pin{UserID: id}).
		Where("created_at > ?", createdAfter).
		Order("created_at desc").
		Find(&pins)

	if result.Error != nil {
		return nil, NewAccountError(ErrKeyPinsRetrievalFailed, result.Error)
	}

	return pins, nil
}

func (s AccountServiceDefault) DeletePinByHash(hash []byte, userId uint) error {
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
	pinQuery := models.Pin{UploadID: uploadID, UserID: userId}
	result = s.db.
		Where(&pinQuery).
		Delete(&models.Pin{})

	if result.Error != nil {
		return result.Error
	}

	return nil
}
func (s AccountServiceDefault) PinByHash(hash []byte, userId uint) error {
	// Define a struct for the query condition
	uploadQuery := models.Upload{Hash: hash}

	result := s.db.
		Model(&uploadQuery).
		Where(&uploadQuery).
		First(&uploadQuery)

	if result.Error != nil {
		return result.Error
	}

	return s.PinByID(uploadQuery.ID, userId)
}

func (s AccountServiceDefault) PinByID(uploadId uint, userId uint) error {
	result := s.db.Model(&models.Pin{}).Where(&models.Pin{UploadID: uploadId, UserID: userId}).First(&models.Pin{})

	if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return result.Error
	}

	if result.RowsAffected > 0 {
		return nil
	}

	// Create a pin with the retrieved upload ID and matching account ID
	pinQuery := models.Pin{UploadID: uploadId, UserID: userId}
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

	otp, otpErr := TOTPGenerate(user.Email, s.config.Config().Core.Domain)
	if otpErr != nil {
		return "", NewAccountError(ErrKeyOTPGenerationFailed, otpErr)
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

func (s AccountServiceDefault) DNSLinkExists(hash []byte) (bool, *models.DNSLink, error) {
	upload, err := s.metadata.GetUpload(context.Background(), hash)
	if err != nil {
		return false, nil, err
	}

	exists, model, err := s.exists(&models.DNSLink{}, map[string]interface{}{"upload_id": upload.ID})
	if !exists || err != nil {
		return false, nil, err
	}

	pinned, err := s.UploadPinned(hash)
	if err != nil {
		return false, nil, err
	}

	if !pinned {
		return false, nil, nil
	}

	return true, model.(*models.DNSLink), nil
}

func (s AccountServiceDefault) UploadPinned(hash []byte) (bool, error) {
	upload, err := s.metadata.GetUpload(context.Background(), hash)
	if err != nil {
		return false, err
	}

	var pin models.Pin
	result := s.db.Model(&models.Pin{}).Where(&models.Pin{UploadID: upload.ID}).First(&pin)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return false, nil
		}

		return false, result.Error
	}

	return true, nil
}

func GenerateSecurityToken() string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 6)
	_, err := rand.Read(b)
	if err != nil {
		panic(err)
	}
	for i := 0; i < 6; i++ {
		b[i] = charset[b[i]%byte(len(charset))]
	}
	return string(b)
}

func (s AccountServiceDefault) doLogin(user *models.User, ip string, bypassSecurity bool) (string, error) {
	purpose := JWTPurposeLogin

	if user.OTPEnabled && !bypassSecurity {
		purpose = JWTPurpose2FA
	}

	token, jwtErr := JWTGenerateToken(s.config.Config().Core.Domain, s.identity, user.ID, purpose)
	if jwtErr != nil {
		return "", NewAccountError(ErrKeyJWTGenerationFailed, jwtErr)
	}

	now := time.Now()

	err := s.updateAccountInfo(user.ID, models.User{LastLoginIP: ip, LastLogin: &now})
	if err != nil {
		return "", err
	}

	return token, nil
}

func (s AccountServiceDefault) updateAccountInfo(userId uint, info models.User) error {
	var user models.User

	user.ID = userId

	result := s.db.Model(&models.User{}).Where(&user).Updates(info)

	if result.Error != nil {
		return NewAccountError(ErrKeyDatabaseOperationFailed, result.Error)
	}

	return nil
}

func (s AccountServiceDefault) exists(model interface{}, conditions map[string]interface{}) (bool, interface{}, error) {
	// Conduct a query with the provided model and conditions
	result := s.db.Preload(clause.Associations).Model(model).Where(conditions).First(model)

	// Check if any rows were found
	exists := result.RowsAffected > 0

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return false, nil, nil
	}

	if exists {
		return true, model, nil
	}

	return false, model, NewAccountError(ErrKeyDatabaseOperationFailed, result.Error)
}

func (s AccountServiceDefault) validPassword(user *models.User, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))

	return err == nil
}
