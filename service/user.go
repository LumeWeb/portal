package service

import (
	"errors"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/service/internal/mailer"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"time"
)

var _ core.UserService = (*UserServiceDefault)(nil)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID: core.USER_SERVICE,
		Factory: func() (core.Service, []core.ContextBuilderOption, error) {
			return NewUserService()
		},
		Depends: []string{core.MAILER_SERVICE},
	})
}

type UserServiceDefault struct {
	ctx       core.Context
	config    config.Manager
	db        *gorm.DB
	mailer    core.MailerService
	subdomain string
}

func NewUserService() (*UserServiceDefault, []core.ContextBuilderOption, error) {
	user := &UserServiceDefault{}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			user.ctx = ctx
			user.config = ctx.Config()
			user.db = ctx.DB()
			user.mailer = ctx.Service(core.MAILER_SERVICE).(core.MailerService)
			return nil
		}),
	)

	return user, opts, nil
}

func (u *UserServiceDefault) SetAccountSubdomain(subdomain string) {
	u.subdomain = subdomain
}

func (u UserServiceDefault) EmailExists(email string) (bool, *models.User, error) {
	user := &models.User{}
	exists, model, err := u.Exists(user, map[string]interface{}{"email": email})
	if !exists || err != nil {
		return false, nil, err
	}
	return true, model.(*models.User), nil // Type assertion since `Exists` returns interface{}
}
func (u UserServiceDefault) PubkeyExists(pubkey string) (bool, *models.PublicKey, error) {
	publicKey := &models.PublicKey{}
	exists, model, err := u.Exists(publicKey, map[string]interface{}{"key": pubkey})
	if !exists || err != nil {
		return false, nil, err
	}
	return true, model.(*models.PublicKey), nil // Type assertion is necessary
}

func (u UserServiceDefault) AccountExists(id uint) (bool, *models.User, error) {
	user := &models.User{}
	exists, model, err := u.Exists(user, map[string]interface{}{"id": id})
	if !exists || err != nil {
		return false, nil, err
	}
	return true, model.(*models.User), nil // Ensure to assert the type correctly
}

func (u UserServiceDefault) HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", core.NewAccountError(core.ErrKeyHashingFailed, err)
	}
	return string(bytes), nil
}

func (u UserServiceDefault) CreateAccount(email string, password string, verifyEmail bool) (*models.User, error) {
	passwordHash, err := u.HashPassword(password)
	if err != nil {
		return nil, err
	}

	user := models.User{
		Email:        email,
		PasswordHash: passwordHash,
	}

	result := u.db.Create(&user)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
			return nil, core.NewAccountError(core.ErrKeyEmailAlreadyExists, nil)
		}

		if err, ok := result.Error.(*mysql.MySQLError); ok {
			if err.Number == 1062 {
				return nil, core.NewAccountError(core.ErrKeyEmailAlreadyExists, nil)
			}
		}

		return nil, core.NewAccountError(core.ErrKeyAccountCreationFailed, result.Error)
	}

	if verifyEmail {
		err = u.SendEmailVerification(user.ID)
		if err != nil {
			return nil, err
		}
	}

	return &user, nil
}

func (u UserServiceDefault) UpdateAccountName(userId uint, firstName string, lastName string) error {
	return u.UpdateAccountInfo(userId, models.User{FirstName: firstName, LastName: lastName})
}
func (u UserServiceDefault) UpdateAccountEmail(userId uint, email string, password string) error {
	exists, euser, err := u.EmailExists(email)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) || (exists && euser.ID != userId) {
		return core.NewAccountError(core.ErrKeyEmailAlreadyExists, nil)
	}

	valid, user, err := u.ValidLoginByUserID(userId, password)
	if err != nil {
		return err
	}

	if !valid {
		return core.NewAccountError(core.ErrKeyInvalidLogin, nil)
	}

	if user.Email == email {
		return core.NewAccountError(core.ErrKeyUpdatingSameEmail, nil)
	}

	var update models.User

	update.Email = email

	return u.UpdateAccountInfo(userId, update)
}

func (u UserServiceDefault) UpdateAccountPassword(userId uint, password string, newPassword string) error {
	valid, _, err := u.ValidLoginByUserID(userId, password)
	if err != nil {
		return err
	}

	if !valid {
		return core.NewAccountError(core.ErrKeyInvalidPassword, nil)
	}

	passwordHash, err := u.HashPassword(newPassword)
	if err != nil {
		return err
	}

	return u.UpdateAccountInfo(userId, models.User{PasswordHash: passwordHash})
}

func (u UserServiceDefault) ValidLoginByUserID(id uint, password string) (bool, *models.User, error) {
	var user models.User

	user.ID = id

	result := u.db.Model(&user).Where(&user).First(&user)

	if result.RowsAffected == 0 || result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return false, nil, core.NewAccountError(core.ErrKeyInvalidLogin, result.Error)
		}

		return false, nil, core.NewAccountError(core.ErrKeyDatabaseOperationFailed, result.Error)
	}

	valid := u.ValidLoginByUserObj(&user, password)

	if !valid {
		return false, nil, nil
	}

	return true, &user, nil
}

func (u UserServiceDefault) validPassword(user *models.User, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))

	return err == nil
}

func (u UserServiceDefault) UpdateAccountInfo(userId uint, info models.User) error {
	var user models.User

	user.ID = userId

	result := u.db.Model(&models.User{}).Where(&user).Updates(info)

	if result.Error != nil {
		return core.NewAccountError(core.ErrKeyDatabaseOperationFailed, result.Error)
	}

	return nil
}
func (u UserServiceDefault) ValidLoginByUserObj(user *models.User, password string) bool {
	return u.validPassword(user, password)
}
func (u UserServiceDefault) AddPubkeyToAccount(user models.User, pubkey string) error {
	var model models.PublicKey

	model.Key = pubkey
	model.UserID = user.ID

	result := u.db.Create(&model)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrDuplicatedKey) {
			return core.NewAccountError(core.ErrKeyPublicKeyExists, result.Error)
		}

		return core.NewAccountError(core.ErrKeyDatabaseOperationFailed, result.Error)
	}

	return nil
}

func (u UserServiceDefault) Exists(model any, conditions map[string]any) (bool, any, error) {
	// Conduct a query with the provided model and conditions
	result := u.db.Preload(clause.Associations).Model(model).Where(conditions).First(model)

	// Check if any rows were found
	exists := result.RowsAffected > 0

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return false, nil, nil
	}

	if exists {
		return true, model, nil
	}

	return false, model, core.NewAccountError(core.ErrKeyDatabaseOperationFailed, result.Error)
}

func (u UserServiceDefault) SendEmailVerification(userId uint) error {
	if u.subdomain == "" {
		return core.NewAccountError(core.ErrKeyAccountSubdomainNotSet, nil)
	}

	exists, user, err := u.AccountExists(userId)
	if !exists || err != nil {
		return err
	}

	if user.Verified {
		return core.NewAccountError(core.ErrKeyAccountAlreadyVerified, nil)
	}

	token := core.GenerateSecurityToken()

	var verification models.EmailVerification

	verification.UserID = user.ID
	verification.Token = token
	verification.ExpiresAt = time.Now().Add(time.Hour)

	err = u.db.Create(&verification).Error
	if err != nil {
		return core.NewAccountError(core.ErrKeyDatabaseOperationFailed, err)
	}

	verifyUrl := fmt.Sprintf("%s/account/verify?token=%s", fmt.Sprintf("https://%s.%s", u.subdomain, u.config.Config().Core.Domain), token)
	vars := map[string]interface{}{
		"FirstName":        user.FirstName,
		"Email":            user.Email,
		"VerificationLink": verifyUrl,
		"ExpireTime":       time.Until(verification.ExpiresAt).Round(time.Second * 2),
		"PortalName":       u.config.Config().Core.PortalName,
	}

	return u.mailer.TemplateSend(mailer.TPL_VERIFY_EMAIL, vars, vars, user.Email)
}

func (u UserServiceDefault) VerifyEmail(email string, token string) error {
	var verification models.EmailVerification

	verification.Token = token

	result := u.db.Model(&verification).
		Preload("User").
		Where(&verification).
		First(&verification)

	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return core.NewAccountError(core.ErrKeySecurityInvalidToken, nil)
		}

		return core.NewAccountError(core.ErrKeyDatabaseOperationFailed, nil)
	}

	if verification.ExpiresAt.Before(time.Now()) {
		return core.NewAccountError(core.ErrKeySecurityTokenExpired, nil)
	}

	if len(verification.NewEmail) > 0 && verification.NewEmail != email {
		return core.NewAccountError(core.ErrKeySecurityInvalidToken, nil)
	} else if verification.User.Email != email {
		return core.NewAccountError(core.ErrKeySecurityInvalidToken, nil)
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
		err := u.UpdateAccountInfo(verification.UserID, update)
		if err != nil {
			return err
		}
	}

	verification = models.EmailVerification{
		UserID: verification.UserID,
	}

	if result := u.db.Where(&verification).Delete(&verification); result.Error != nil {
		return core.NewAccountError(core.ErrKeyDatabaseOperationFailed, result.Error)
	}

	return nil
}
