package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/go-sql-driver/mysql"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/event"
	dbHelper "go.lumeweb.com/portal/service/internal/db"
	"go.lumeweb.com/portal/service/internal/mailer"
	"go.lumeweb.com/portal/service/internal/user"
	userInternal "go.lumeweb.com/portal/service/internal/user"
	"github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var _ core.UserService = (*UserServiceDefault)(nil)
var _ core.Cronable = (*UserServiceDefault)(nil)

func (u UserServiceDefault) getAuthService() core.AuthService {
	return core.GetService[core.AuthService](u.Context(), core.AUTH_SERVICE)
}

func init() {
	core.RegisterService(core.ServiceInfo{
		ID:      core.USER_SERVICE,
		Factory: NewUserService,
		Depends: []string{core.MAILER_SERVICE, core.CRON_SERVICE},
		Metrics: user.GetCollectors(),
	})
}

type UserServiceDefault struct {
	*core.BaseComponent
	mailer    core.MailerService
	cron      core.CronService
	subdomain string
	access    core.AccessService
}

func NewUserService() (core.Service, []core.ContextBuilderOption, error) {
	_user := &UserServiceDefault{}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			_user.mailer = core.GetService[core.MailerService](ctx, core.MAILER_SERVICE)
			_user.cron = core.GetService[core.CronService](ctx, core.CRON_SERVICE)
			_user.access = core.GetService[core.AccessService](ctx, core.ACCESS_SERVICE)

			_user.cron.RegisterEntity(_user)

			core.Listen[event.UserServiceSubdomainSetEvent](ctx, event.EVENT_USER_SERVICE_SUBDOMAIN_SET, func(e *core.CoreEvent[event.UserServiceSubdomainSetEvent]) error {
				_user.subdomain = e.Data.Subdomain
				return nil
			})
			return nil
		}),
	)

	return _user, opts, nil
}

func (u UserServiceDefault) RegisterTasks(ctx context.Context, crn core.CronService) error {
	ctx, span := core.TraceMethod(ctx, "UserServiceDefault.RegisterTasks")
	defer span.End()

	attime, err := time.Parse(time.Kitchen, "03:00AM")
	if err != nil {
		return fmt.Errorf("failed to parse time: %w", err)
	}

	// Register the job type with the cron service
	err = crn.RegisterJobType(ctx, user.ProcessAccountDeletionRequestsJobType, func() (core.CronJob, error) {
		return user.NewProcessAccountDeletionRequestsJob(), nil
	}, &core.CronScheduleDefinition{
		Type:   core.CronScheduleTypeDaily,
		AtTime: attime,
	})
	if err != nil {
		return fmt.Errorf("failed to register account deletion job: %w", err)
	}

	return nil
}

func (u UserServiceDefault) ScheduleJobs(ctx context.Context, cron core.CronService) error {
	ctx, span := core.TraceMethod(ctx, "UserServiceDefault.ScheduleJobs")
	defer span.End()

	// Create the job if it doesn't exist
	_, err := cron.JobFactory().CreateJob(ctx, user.ProcessAccountDeletionRequestsJobType)
	if err != nil {
		return fmt.Errorf("failed to create account deletion job: %w", err)
	}

	return nil
}

func (u UserServiceDefault) ID() string {
	return core.USER_SERVICE
}

type emailExistsResult struct {
	exists bool
	user   *models.User
}

func (u UserServiceDefault) EmailExists(ctx context.Context, email string) (bool, *models.User, error) {
	ctx, span := core.TraceMethod(ctx, "UserServiceDefault.EmailExists")
	defer span.End()

	result, err := core.MetricTrackResult(
		userInternal.UserOperationDuration.WithLabelValues(userInternal.LabelOpCheckExists),
		userInternal.UserOperationFailed.WithLabelValues(userInternal.LabelOpCheckExists),
		func() (*emailExistsResult, error) {
			_user := &models.User{}
			exists, model, err := u.Exists(ctx, _user, map[string]interface{}{"email": email})
			if !exists || err != nil {
				return nil, err
			}
			return &emailExistsResult{
				exists: true,
				user:   model.(*models.User),
			}, nil
		},
	)

	if err == nil {
		userInternal.AccountsExistsQueried.WithLabelValues(userInternal.LabelOpCheckExists).Inc()
	}
	if result == nil {
		return false, nil, err
	}
	return result.exists, result.user, err
}

type keyIdentityExistsResult struct {
	exists       bool
	keyIdentity  *models.KeyIdentity
}

func (u UserServiceDefault) KeyIdentityExists(ctx context.Context, keyType string, key string) (bool, *models.KeyIdentity, error) {
	ctx, span := core.TraceMethod(ctx, "UserServiceDefault.KeyIdentityExists")
	defer span.End()

	// Require a registered handler so keys are always looked up in canonical form
	handler, ok := core.GetKeyIdentityHandler(keyType)
	if !ok {
		return false, nil, core.NewAccountError(core.ErrKeyInvalidLogin, fmt.Errorf("no handler registered for key type %q", keyType))
	}
	normalized, err := handler.NormalizeKey(key)
	if err != nil {
		return false, nil, err
	}
	key = normalized

	result, err := core.MetricTrackResult(
		userInternal.UserOperationDuration.WithLabelValues(userInternal.LabelOpCheckExists),
		userInternal.UserOperationFailed.WithLabelValues(userInternal.LabelOpCheckExists),
		func() (*keyIdentityExistsResult, error) {
			keyIdentity := &models.KeyIdentity{}
			exists, model, err := u.Exists(ctx, keyIdentity, map[string]interface{}{
				"type": keyType,
				"key":  key,
			})
			if !exists || err != nil {
				return nil, err
			}
			return &keyIdentityExistsResult{
				exists:      true,
				keyIdentity: model.(*models.KeyIdentity),
			}, nil
		},
	)

	if err == nil {
		userInternal.AccountsExistsQueried.WithLabelValues(userInternal.LabelOpCheckExists).Inc()
	}
	if result == nil {
		return false, nil, err
	}
	return result.exists, result.keyIdentity, err
}

type accountExistsResult struct {
	exists bool
	user   *models.User
}

func (u UserServiceDefault) AccountExists(ctx context.Context, id uint) (bool, *models.User, error) {
	ctx, span := core.TraceMethod(ctx, "UserServiceDefault.AccountExists")
	defer span.End()

	result, err := core.MetricTrackResult(
		userInternal.UserOperationDuration.WithLabelValues(userInternal.LabelOpCheckExists),
		userInternal.UserOperationFailed.WithLabelValues(userInternal.LabelOpCheckExists),
		func() (*accountExistsResult, error) {
			_user := &models.User{}
			exists, model, err := u.Exists(ctx, _user, map[string]interface{}{"id": id})
			if !exists || err != nil {
				return nil, err
			}
			return &accountExistsResult{
				exists: true,
				user:   model.(*models.User),
			}, nil
		},
	)

	if err == nil {
		userInternal.AccountsExistsQueried.WithLabelValues(userInternal.LabelOpCheckExists).Inc()
	}
	if result == nil {
		return false, nil, err
	}
	return result.exists, result.user, err
}

func (u UserServiceDefault) HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", core.NewAccountError(core.ErrKeyHashingFailed, err)
	}
	return string(bytes), nil
}

func (u UserServiceDefault) CreateAccount(ctx context.Context, email string, password string, verifyEmail bool) (*models.User, error) {
	ctx, span := core.TraceMethod(ctx, "UserServiceDefault.CreateAccount")
	defer span.End()

	result, err := core.MetricTrackResult(
		userInternal.UserOperationDuration.WithLabelValues(userInternal.LabelOpCreate),
		userInternal.UserOperationFailed.WithLabelValues(userInternal.LabelOpCreate),
		func() (*models.User, error) {
			// First check if email already exists
			exists, _, err := u.EmailExists(ctx, email)
			if err != nil {
				return nil, fmt.Errorf("error checking email existence: %w", err)
			}
			if exists {
				return nil, core.NewAccountError(core.ErrKeyEmailAlreadyExists, nil)
			}

			passwordHash, err := u.HashPassword(password)
			if err != nil {
				return nil, err
			}

			_user := models.User{
				Email:        email,
				PasswordHash: passwordHash,
			}

			isFirstUser := false
			err = db.RetryableComponentTransaction(u, ctx, func(tx *gorm.DB) *gorm.DB {
				var count int64
				if err := tx.Model(&models.User{}).Count(&count).Error; err != nil {
					_ = tx.AddError(err)
					return tx
				}
				isFirstUser = count == 0
				return tx.Create(&_user)
			})

			if err != nil {
				if u.isDuplicateKeyError(err) {
					return nil, core.NewAccountError(core.ErrKeyEmailAlreadyExists, nil)
				}

				return nil, core.NewAccountError(core.ErrKeyAccountCreationFailed, err)
			}

			if isFirstUser {
				_user.Verified = true
				if err := u.UpdateAccountInfo(ctx, _user.ID, map[string]interface{}{"verified": true}); err != nil {
					return nil, err
				}

				if err := u.access.AssignRoleToUser(nil, _user.ID, core.ACCESS_ADMIN_ROLE); err != nil {
					return nil, core.NewAccountError(core.ErrKeyAssigningAdminRoleFailed, err)
				}
			} else if verifyEmail {
				if err := u.SendEmailVerification(ctx, _user.ID); err != nil {
					u.Logger().Warn("Failed to send email verification during account creation, but account was created successfully",
						zap.Uint("user_id", _user.ID),
						zap.String("email", _user.Email),
						zap.Error(err))
					mailer.MailerFailed.WithLabelValues(mailer.LabelOpSend).Inc()
				}
			}

			if err := u.access.AssignRoleToUser(nil, _user.ID, core.ACCESS_USER_ROLE); err != nil {
				return nil, core.NewAccountError(core.ErrKeyAssigningUserRoleFailed, err)
			}

			if err := u.Context().Fire(event.EVENT_USER_CREATED, event.NewUserCreatedEvent(ctx, &_user)); err != nil {
				return nil, err
			}

			if isFirstUser || !verifyEmail {
				if err := u.Context().Fire(event.EVENT_USER_ACTIVATED, event.NewUserActivatedEvent(ctx, &_user)); err != nil {
					return nil, err
				}
			}

			return &_user, nil
		},
	)

	if err == nil {
		userInternal.AccountsCreated.WithLabelValues(userInternal.LabelOpCreate).Inc()
	}
	return result, err
}

func (u UserServiceDefault) UpdateAccountName(ctx context.Context, userId uint, firstName string, lastName string) error {
	ctx, span := core.TraceMethod(ctx, "UserServiceDefault.UpdateAccountName")
	defer span.End()

	return core.MetricTrack(
		userInternal.UserOperationDuration.WithLabelValues(userInternal.LabelOpUpdate),
		userInternal.UserOperationFailed.WithLabelValues(userInternal.LabelOpUpdate),
		func() error {
			err := u.UpdateAccountInfo(ctx, userId, map[string]any{
				"first_name": firstName,
				"last_name":  lastName,
			})
			if err == nil {
				userInternal.AccountsUpdated.WithLabelValues(userInternal.LabelOpUpdate).Inc()
			}
			return err
		},
	)
}

func (u UserServiceDefault) UpdateAccountEmail(ctx context.Context, userId uint, email string, password string) error {
	ctx, span := core.TraceMethod(ctx, "UserServiceDefault.UpdateAccountEmail")
	defer span.End()

	return core.MetricTrack(
		userInternal.UserOperationDuration.WithLabelValues(userInternal.LabelOpUpdate),
		userInternal.UserOperationFailed.WithLabelValues(userInternal.LabelOpUpdate),
		func() error {
			exists, euser, err := u.EmailExists(ctx, email)
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) || (exists && euser.ID != userId) {
				return core.NewAccountError(core.ErrKeyEmailAlreadyExists, nil)
			}

			valid, _user, err := u.getAuthService().ValidLoginByUserID(ctx, userId, password)
			if err != nil {
				return err
			}

			if !valid {
				return core.NewAccountError(core.ErrKeyInvalidLogin, nil)
			}

			if _user.Email == email {
				return core.NewAccountError(core.ErrKeyUpdatingSameEmail, nil)
			}

			err = u.UpdateAccountInfo(ctx, userId, map[string]any{
				"email": email,
			})
			if err == nil {
				userInternal.AccountsUpdated.WithLabelValues(userInternal.LabelOpUpdate).Inc()
			}
			return err
		},
	)
}

func (u UserServiceDefault) UpdateAccountPassword(ctx context.Context, userId uint, password string, newPassword string) error {
	ctx, span := core.TraceMethod(ctx, "UserServiceDefault.UpdateAccountPassword")
	defer span.End()

	return core.MetricTrack(
		userInternal.UserOperationDuration.WithLabelValues(userInternal.LabelOpUpdate),
		userInternal.UserOperationFailed.WithLabelValues(userInternal.LabelOpUpdate),
		func() error {
			valid, _, err := u.getAuthService().ValidLoginByUserID(ctx, userId, password)
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

			err = u.UpdateAccountInfo(ctx, userId, map[string]any{
				"password_hash": passwordHash,
			})
			if err == nil {
				userInternal.AccountsUpdated.WithLabelValues(userInternal.LabelOpUpdate).Inc()
			}
			return err
		},
	)
}

func (u UserServiceDefault) UpdateAccountInfo(ctx context.Context, userId uint, info map[string]any) error {
	ctx, span := core.TraceMethod(ctx, "UserServiceDefault.UpdateAccountInfo")
	defer span.End()

	return core.MetricTrack(
		userInternal.UserOperationDuration.WithLabelValues(userInternal.LabelOpUpdate),
		userInternal.UserOperationFailed.WithLabelValues(userInternal.LabelOpUpdate),
		func() error {
			var _user models.User
			_user.ID = userId

			if err := db.RetryableComponentTransaction(u, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.Model(&_user).Where(&_user).Updates(info)
			}); err != nil {
				return core.NewAccountError(core.ErrKeyDatabaseOperationFailed, err)
			}

			return nil
		},
	)
}

func (u UserServiceDefault) AddKeyIdentity(ctx context.Context, user models.User, keyType string, key string, metadata json.RawMessage) error {
	ctx, span := core.TraceMethod(ctx, "UserServiceDefault.AddKeyIdentity")
	defer span.End()

	// Require a registered handler so keys are always stored in canonical form
	handler, ok := core.GetKeyIdentityHandler(keyType)
	if !ok {
		return core.NewAccountError(core.ErrKeyAddKeyIdentityFailed, fmt.Errorf("no handler registered for key type %q", keyType))
	}
	normalized, err := handler.NormalizeKey(key)
	if err != nil {
		return err
	}
	key = normalized

	// Validate metadata using the type handler
	// Apply nil default first so handlers never see nil metadata
	if metadata == nil {
		metadata = json.RawMessage(`{}`)
	}
	validated, err := handler.ValidateMetadata(metadata)
	if err != nil {
		return err
	}
	metadata = validated

	// Always default to empty JSON if metadata is nil
	if metadata == nil {
		metadata = json.RawMessage(`{}`)
	}

	return core.MetricTrack(
		userInternal.UserOperationDuration.WithLabelValues(userInternal.LabelOpAddPubkey),
		userInternal.UserOperationFailed.WithLabelValues(userInternal.LabelOpAddPubkey),
		func() error {
			var model models.KeyIdentity

			model.Type = keyType
			model.Key = key
			model.Metadata = metadata
			model.UserID = user.ID

			if err := db.RetryableComponentTransaction(u, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.WithContext(ctx).Create(&model)
			}); err != nil {
				if u.isDuplicateKeyError(err) {
					return core.NewAccountError(core.ErrKeyKeyIdentityExists, err)
				}

				if u.isConstraintViolationError(err) {
					if errors.Is(err, gorm.ErrForeignKeyViolated) {
						return core.NewAccountError(core.ErrKeyUserNotFound, err)
					}
					return core.NewAccountError(core.ErrKeyDatabaseOperationFailed, err)
				}

				return core.NewAccountError(core.ErrKeyDatabaseOperationFailed, err)
			}

			userInternal.PublicKeyAdded.WithLabelValues(userInternal.LabelOpAddPubkey).Inc()
			return nil
		},
	)
}

func (u UserServiceDefault) Exists(ctx context.Context, model any, conditions map[string]any) (bool, any, error) {
	ctx, span := core.TraceMethod(ctx, "UserServiceDefault.Exists")
	defer span.End()

	var rowsAffected int64
	// Conduct a query with the provided model and conditions
	err := db.RetryableComponentTransaction(u, ctx, func(tx *gorm.DB) *gorm.DB {
		tx = tx.Preload(clause.Associations).Model(model).Where(conditions).First(model)
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

	return false, model, dbHelper.HandleDBError(err)
}

func (u UserServiceDefault) SendEmailVerification(ctx context.Context, userId uint) error {
	ctx, span := core.TraceMethod(ctx, "UserServiceDefault.SendEmailVerification")
	defer span.End()

	return core.MetricTrack(
		userInternal.UserOperationDuration.WithLabelValues(userInternal.LabelOpSendVerification),
		userInternal.UserOperationFailed.WithLabelValues(userInternal.LabelOpSendVerification),
		func() error {
			if u.subdomain == "" {
				return core.NewAccountError(core.ErrKeyAccountSubdomainNotSet, nil)
			}

			exists, _user, err := u.AccountExists(ctx, userId)
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

			if err = db.RetryableComponentTransaction(u, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.Create(&verification)
			}); err != nil {
				return core.NewAccountError(core.ErrKeyDatabaseOperationFailed, err)
			}

			queryVars := url.Values{}
			queryVars.Set("token", token)
			queryVars.Set("email", _user.Email)

			verifyUrl := fmt.Sprintf("%s/account/verify?%s", fmt.Sprintf("https://%s.%s", u.subdomain, u.Config().Config().Core.Domain), queryVars.Encode())
			vars := map[string]interface{}{
				"FirstName":        _user.FirstName,
				"Email":            _user.Email,
				"VerificationLink": verifyUrl,
				"ExpireTime":       time.Until(verification.ExpiresAt).Round(time.Second),
				"PortalName":       u.Config().Config().Core.PortalName,
			}

			err = u.mailer.TemplateSend(core.MAILER_TPL_VERIFY_EMAIL, vars, vars, _user.Email)
			if err == nil {
				userInternal.EmailVerificationSent.WithLabelValues(userInternal.LabelOpSendVerification).Inc()
			}
			return err
		},
	)
}

func (u UserServiceDefault) IsAccountVerified(ctx context.Context, userId uint) (bool, error) {
	ctx, span := core.TraceMethod(ctx, "UserServiceDefault.IsAccountVerified")
	defer span.End()

	var _user models.User
	_user.ID = userId

	if err := db.RetryableComponentTransaction(u, ctx, func(tx *gorm.DB) *gorm.DB {
		return tx.Model(&_user).Where(&_user).First(&_user)
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, err
		}

		return false, dbHelper.HandleDBError(err)
	}

	return _user.Verified, nil
}

func (u UserServiceDefault) VerifyEmail(ctx context.Context, email string, token string) error {
	ctx, span := core.TraceMethod(ctx, "UserServiceDefault.VerifyEmail")
	defer span.End()

	return core.MetricTrack(
		userInternal.UserOperationDuration.WithLabelValues(userInternal.LabelOpVerifyEmail),
		userInternal.UserOperationFailed.WithLabelValues(userInternal.LabelOpVerifyEmail),
		func() error {
			var verification models.EmailVerification

			verification.Token = token

			if err := db.RetryableComponentTransaction(u, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.Model(&verification).
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
				err := u.UpdateAccountInfo(ctx, verification.UserID, updateFields)
				if err != nil {
					return err
				}
			}

			verification = models.EmailVerification{
				UserID: verification.UserID,
			}

			if err := db.RetryableComponentTransaction(u, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.Where(&verification).Delete(&verification)
			}); err != nil {
				return core.NewAccountError(core.ErrKeyDatabaseOperationFailed, err)
			}

			err := u.Context().Fire(event.EVENT_USER_ACTIVATED, event.NewUserActivatedEvent(ctx, &verification.User))
			if err != nil {
				return err
			}

			userInternal.EmailVerified.WithLabelValues(userInternal.LabelOpVerifyEmail).Inc()
			return nil
		},
	)
}

func (u *UserServiceDefault) DeleteAccount(ctx context.Context, userId uint) error {
	ctx, span := core.TraceMethod(ctx, "UserServiceDefault.DeleteAccount")
	defer span.End()

	return core.MetricTrack(
		userInternal.UserOperationDuration.WithLabelValues(userInternal.LabelOpDelete),
		userInternal.UserOperationFailed.WithLabelValues(userInternal.LabelOpDelete),
		func() error {
			err := db.RetryableComponentTransaction(u, ctx, func(tx *gorm.DB) *gorm.DB {
				// First, check if the user exists
				var _user models.User
				if err := tx.First(&_user, userId).Error; err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						_ = tx.AddError(core.NewAccountError(core.ErrKeyUserNotFound, nil))
					} else {
						_ = tx.AddError(dbHelper.HandleDBError(err))
					}
					return tx
				}

				// Delete associated AccountDeletion record if it exists
				if err := tx.Where("user_id = ?", userId).Delete(&models.AccountDeletion{}).Error; err != nil {
					_ = tx.AddError(dbHelper.HandleDBError(err))
					return tx
				}

				// Delete the user
				if err := tx.Delete(&_user).Error; err != nil {
					_ = tx.AddError(dbHelper.HandleDBError(err))
					return tx
				}

				return tx
			})

			if err == nil {
				userInternal.AccountsDeleted.WithLabelValues(userInternal.LabelOpDelete).Inc()
			}
			return err
		},
	)
}

func (u *UserServiceDefault) IsAccountPendingDeletion(ctx context.Context, userId uint) (bool, error) {
	ctx, span := core.TraceMethod(ctx, "UserServiceDefault.IsAccountPendingDeletion")
	defer span.End()

	var count int64
	err := db.RetryableComponentTransaction(u, ctx, func(tx *gorm.DB) *gorm.DB {
		return tx.Model(&models.AccountDeletion{}).
			Where("user_id = ? AND deleted_at IS NULL", userId).
			Count(&count)
	})
	return count > 0, err
}

func (u *UserServiceDefault) RequestAccountDeletion(ctx context.Context, userId uint, userIP string) error {
	ctx, span := core.TraceMethod(ctx, "UserServiceDefault.RequestAccountDeletion")
	defer span.End()

	return core.MetricTrack(
		userInternal.UserOperationDuration.WithLabelValues(userInternal.LabelOpRequestDeletion),
		userInternal.UserOperationFailed.WithLabelValues(userInternal.LabelOpRequestDeletion),
		func() error {
			err := db.RetryableComponentTransaction(u, ctx, func(tx *gorm.DB) *gorm.DB {
				var _user models.User
				if err := tx.First(&_user, userId).Error; err != nil {
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

			if err == nil {
				userInternal.AccountDeletionRequested.WithLabelValues(userInternal.LabelOpRequestDeletion).Inc()
			}
			return err
		},
	)
}

func (u *UserServiceDefault) GetAccountsPendingDeletion(ctx context.Context) ([]*models.User, error) {
	ctx, span := core.TraceMethod(ctx, "UserServiceDefault.GetAccountsPendingDeletion")
	defer span.End()

	return core.MetricTrackResult(
		userInternal.UserOperationDuration.WithLabelValues(userInternal.LabelOpListPending),
		userInternal.UserOperationFailed.WithLabelValues(userInternal.LabelOpListPending),
		func() ([]*models.User, error) {
			var users []*models.User
			gracePeriod := time.Duration(u.Config().Config().Core.Account.DeletionGracePeriod) * time.Hour
			cutoffTime := time.Now().Add(-1 * gracePeriod)

			err := db.RetryableComponentTransaction(u, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.Joins("JOIN account_deletions ON users.id = account_deletions.user_id").
					Where("account_deletions.deleted_at IS NULL AND account_deletions.created_at < ?", cutoffTime).
					Find(&users)
			})

			if err != nil {
				return nil, err
			}

			return users, nil
		},
	)
}

func (u UserServiceDefault) isDuplicateKeyError(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}

	// MySQL duplicate key error
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr != nil && mysqlErr.Number == 1062 {
		return true
	}

	// SQLite unique constraint violation
	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) && sqliteErr.ExtendedCode == sqlite3.ErrConstraintUnique {
		return true
	}

	return false
}

func (u UserServiceDefault) isConstraintViolationError(err error) bool {
	if errors.Is(err, gorm.ErrForeignKeyViolated) || errors.Is(err, gorm.ErrCheckConstraintViolated) {
		return true
	}

	// MySQL constraint violation errors
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr != nil {
		// Common MySQL constraint violation error codes
		switch mysqlErr.Number {
		case 1452: // Cannot add or update a child row: a foreign key constraint fails
			return true
		case 1451: // Cannot delete or update a parent row: a foreign key constraint fails
			return true
		case 1264: // Out of range value
			return true
		case 1048: // Column cannot be null
			return true
		}
	}

	return false
}
