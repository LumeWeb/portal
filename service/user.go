package service

import (
	"errors"
	"fmt"
	"github.com/go-sql-driver/mysql"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/event"
	"go.lumeweb.com/portal/service/internal/user"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"net/url"
	"time"
)

var _ core.UserService = (*UserServiceDefault)(nil)
var _ core.Cronable = (*UserServiceDefault)(nil)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID: core.USER_SERVICE,
		Factory: func() (core.Service, []core.ContextBuilderOption, error) {
			return NewUserService()
		},
		Depends: []string{core.MAILER_SERVICE, core.CRON_SERVICE},
	})
}

type UserServiceDefault struct {
	ctx       core.Context
	config    config.Manager
	db        *gorm.DB
	mailer    core.MailerService
	cron      core.CronService
	subdomain string
	access    core.AccessService
}

func NewUserService() (*UserServiceDefault, []core.ContextBuilderOption, error) {
	_user := &UserServiceDefault{}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			_user.ctx = ctx
			_user.config = ctx.Config()
			_user.db = ctx.DB()
			_user.mailer = core.GetService[core.MailerService](ctx, core.MAILER_SERVICE)
			_user.cron = core.GetService[core.CronService](ctx, core.CRON_SERVICE)
			_user.access = core.GetService[core.AccessService](ctx, core.ACCESS_SERVICE)

			_user.cron.RegisterEntity(_user)

			event.Listen[*event.UserServiceSubdomainSetEvent](ctx, event.EVENT_USER_SERVICE_SUBDOMAIN_SET, func(evt *event.UserServiceSubdomainSetEvent) error {
				_user.subdomain = evt.Subdomain()
				return nil
			})
			return nil
		}),
	)

	return _user, opts, nil
}

func (u UserServiceDefault) RegisterTasks(crn core.CronService) error {
	crn.RegisterTask(user.CronTaskProcessAccountDeletionRequestsName, core.CronTaskFuncHandler(user.CronTaskProcessAccountDeletionRequests), core.CronTaskDefinitionDaily, core.CronTaskNoArgsFactory, true)

	return nil
}

func (u UserServiceDefault) ScheduleJobs(cron core.CronService) error {
	err := cron.CreateJobIfNotExists(user.CronTaskProcessAccountDeletionRequestsName, nil)
	if err != nil {
		return err
	}

	return nil
}

func (u UserServiceDefault) ID() string {
	return core.USER_SERVICE
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

	_user := models.User{
		Email:        email,
		PasswordHash: passwordHash,
	}

	isFirstUser := false
	err = db.RetryableTransaction(u.ctx, u.db, func(tx *gorm.DB) *gorm.DB {
		var count int64
		if err := tx.Model(&models.User{}).Count(&count).Error; err != nil {
			_ = tx.AddError(err)
			return tx
		}
		isFirstUser = count == 0
		return tx.Create(&_user)
	})

	if err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, core.NewAccountError(core.ErrKeyEmailAlreadyExists, nil)
		}

		if mysqlErr, ok := err.(*mysql.MySQLError); ok && mysqlErr.Number == 1062 {
			return nil, core.NewAccountError(core.ErrKeyEmailAlreadyExists, nil)
		}

		return nil, core.NewAccountError(core.ErrKeyAccountCreationFailed, err)
	}

	if isFirstUser {
		_user.Verified = true
		if err := u.UpdateAccountInfo(_user.ID, map[string]interface{}{"verified": true}); err != nil {
			return nil, err
		}

		if err := u.access.AssignRoleToUser(_user.ID, core.ACCESS_ADMIN_ROLE); err != nil {
			return nil, core.NewAccountError(core.ErrKeyAssigningAdminRoleFailed, err)
		}
	} else if verifyEmail {
		if err := u.SendEmailVerification(_user.ID); err != nil {
			return nil, err
		}
	}

	if err := u.access.AssignRoleToUser(_user.ID, core.ACCESS_USER_ROLE); err != nil {
		return nil, core.NewAccountError(core.ErrorAssigningUserRoleFailed, err)
	}

	if err := event.FireUserCreatedEvent(u.ctx, &_user); err != nil {
		return nil, err
	}

	if isFirstUser || !verifyEmail {
		if err := event.FireUserActivatedEvent(u.ctx, &_user); err != nil {
			return nil, err
		}
	}

	return &_user, nil
}

func (u UserServiceDefault) UpdateAccountName(userId uint, firstName string, lastName string) error {
	return u.UpdateAccountInfo(userId, map[string]any{
		"first_name": firstName,
		"last_name":  lastName,
	})
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

	return u.UpdateAccountInfo(userId, map[string]any{
		"email": email,
	})
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

	return u.UpdateAccountInfo(userId, map[string]any{
		"password_hash": passwordHash,
	})
}

func (u UserServiceDefault) ValidLoginByUserID(id uint, password string) (bool, *models.User, error) {
	var user models.User

	user.ID = id

	var rowsAffected int64

	err := db.RetryOnLock(u.db, func(db *gorm.DB) *gorm.DB {
		tx := db.Model(&user).Where(&user).First(&user)
		rowsAffected = tx.RowsAffected

		return tx
	})

	if rowsAffected == 0 || err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil, core.NewAccountError(core.ErrKeyInvalidLogin, err)
		}

		return false, nil, core.NewAccountError(core.ErrKeyDatabaseOperationFailed, err)
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

func (u UserServiceDefault) UpdateAccountInfo(userId uint, info map[string]any) error {
	var user models.User
	user.ID = userId

	if err := db.RetryableTransaction(u.ctx, u.db, func(tx *gorm.DB) *gorm.DB {
		return tx.Model(&user).Where(&user).Updates(info)
	}); err != nil {
		return core.NewAccountError(core.ErrKeyDatabaseOperationFailed, err)
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

	if err := db.RetryOnLock(u.db, func(db *gorm.DB) *gorm.DB {
		return db.Create(&model)

	}); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return core.NewAccountError(core.ErrKeyPublicKeyExists, err)
		}

		return core.NewAccountError(core.ErrKeyDatabaseOperationFailed, err)
	}

	return nil
}

func (u UserServiceDefault) Exists(model any, conditions map[string]any) (bool, any, error) {
	var rowsAffected int64
	// Conduct a query with the provided model and conditions
	err := db.RetryOnLock(u.db, func(db *gorm.DB) *gorm.DB {
		tx := db.Preload(clause.Associations).Model(model).Where(conditions).First(model)
		rowsAffected = tx.RowsAffected

		return tx
	})

	// Check if any rows were found
	exists := rowsAffected > 0

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil, nil
	}

	if exists {
		return true, model, nil
	}

	return false, model, core.NewAccountError(core.ErrKeyDatabaseOperationFailed, err)
}

func (u UserServiceDefault) SendEmailVerification(userId uint) error {
	if u.subdomain == "" {
		return core.NewAccountError(core.ErrKeyAccountSubdomainNotSet, nil)
	}

	exists, _user, err := u.AccountExists(userId)
	if !exists || err != nil {
		return err
	}

	if _user.Verified {
		return core.NewAccountError(core.ErrKeyAccountAlreadyVerified, nil)
	}

	token := core.GenerateSecurityToken()

	var verification models.EmailVerification

	verification.UserID = _user.ID
	verification.Token = token
	verification.ExpiresAt = time.Now().Add(time.Hour)

	if err = db.RetryOnLock(u.db, func(db *gorm.DB) *gorm.DB {
		return db.Create(&verification)
	}); err != nil {
		return core.NewAccountError(core.ErrKeyDatabaseOperationFailed, err)
	}

	queryVars := url.Values{}
	queryVars.Set("token", token)
	queryVars.Set("email", _user.Email)

	verifyUrl := fmt.Sprintf("%s/account/verify?%s", fmt.Sprintf("https://%s.%s", u.subdomain, u.config.Config().Core.Domain), queryVars.Encode())
	vars := map[string]interface{}{
		"FirstName":        _user.FirstName,
		"Email":            _user.Email,
		"VerificationLink": verifyUrl,
		"ExpireTime":       time.Until(verification.ExpiresAt).Round(time.Second * 2),
		"PortalName":       u.config.Config().Core.PortalName,
	}

	return u.mailer.TemplateSend(core.MAILER_TPL_VERIFY_EMAIL, vars, vars, _user.Email)
}

func (u UserServiceDefault) IsAccountVerified(userId uint) (bool, error) {
	var _user models.User
	_user.ID = userId

	if err := db.RetryableTransaction(u.ctx, u.db, func(tx *gorm.DB) *gorm.DB {
		return tx.Model(&_user).Where(&_user).First(&_user)
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, err
		}

		return false, core.NewAccountError(core.ErrKeyDatabaseOperationFailed, err)
	}

	return _user.Verified, nil
}

func (u UserServiceDefault) VerifyEmail(email string, token string) error {
	var verification models.EmailVerification

	verification.Token = token

	if err := db.RetryOnLock(u.db, func(db *gorm.DB) *gorm.DB {
		return db.Model(&verification).
			Preload("User").
			Where(&verification).
			First(&verification)
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
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

	updateFields := make(map[string]interface{})

	if !verification.User.Verified {
		updateFields["verified"] = true
	}

	if len(verification.NewEmail) > 0 {
		updateFields["email"] = verification.NewEmail
	}

	if len(updateFields) > 0 {
		err := u.UpdateAccountInfo(verification.UserID, updateFields)
		if err != nil {
			return err
		}
	}

	verification = models.EmailVerification{
		UserID: verification.UserID,
	}

	if err := db.RetryOnLock(u.db, func(db *gorm.DB) *gorm.DB {
		return db.Where(&verification).Delete(&verification)
	}); err != nil {
		return core.NewAccountError(core.ErrKeyDatabaseOperationFailed, err)
	}

	err := event.FireUserActivatedEvent(u.ctx, &verification.User)
	if err != nil {
		return err
	}

	return nil
}

func (u *UserServiceDefault) DeleteAccount(userId uint) error {
	return db.RetryableTransaction(u.ctx, u.db, func(tx *gorm.DB) *gorm.DB {
		// First, check if the user exists
		var _user models.User
		if err := tx.First(&_user, userId).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				_ = tx.AddError(errors.New("user not found"))
			} else {
				_ = tx.AddError(err)
			}
			return tx
		}

		// Delete associated AccountDeletion record if it exists
		if err := tx.Where("user_id = ?", userId).Delete(&models.AccountDeletion{}).Error; err != nil {
			_ = tx.AddError(err)
			return tx
		}

		// Delete the user
		if err := tx.Delete(&_user).Error; err != nil {
			_ = tx.AddError(err)
			return tx
		}

		return tx
	})
}

func (u *UserServiceDefault) IsAccountPendingDeletion(userId uint) (bool, error) {
	var count int64
	err := db.RetryableTransaction(u.ctx, u.db, func(tx *gorm.DB) *gorm.DB {
		return tx.Model(&models.AccountDeletion{}).
			Where("user_id = ? AND deleted_at IS NULL", userId).
			Count(&count)
	})
	return count > 0, err
}

func (u *UserServiceDefault) RequestAccountDeletion(userId uint, userIP string) error {
	return db.RetryableTransaction(u.ctx, u.db, func(tx *gorm.DB) *gorm.DB {
		var user models.User
		if err := tx.First(&user, userId).Error; err != nil {
			_ = tx.AddError(err)
			return tx
		}

		var deletion models.AccountDeletion
		deletion.UserID = userId

		if err := db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return tx.Where(&deletion).
				Where("deleted_at IS NULL").First(&deletion)
		}); err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				_ = tx.AddError(err)
				return tx
			}

			deletion.UserID = userId
			deletion.IP = userIP

			return tx.Create(&deletion)
		}

		_ = tx.AddError(core.NewAccountError(core.ErrKeyAccountDeletionRequestAlreadyExists, nil))
		return tx
	})
}

func (u *UserServiceDefault) GetAccountsPendingDeletion() ([]*models.User, error) {
	var users []*models.User
	gracePeriod := time.Duration(u.config.Config().Core.Account.DeletionGracePeriod) * time.Hour
	cutoffTime := time.Now().Add(-1 * gracePeriod)

	err := db.RetryableTransaction(u.ctx, u.db, func(tx *gorm.DB) *gorm.DB {
		return tx.Joins("JOIN account_deletions ON users.id = account_deletions.user_id").
			Where("account_deletions.deleted_at IS NULL AND account_deletions.created_at < ?", cutoffTime).
			Find(&users)
	})

	if err != nil {
		return nil, err
	}

	return users, nil
}
