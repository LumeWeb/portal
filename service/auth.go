package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"go.lumeweb.com/portal-middleware/auth/jwt"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/db/models"
	dbHelper "go.lumeweb.com/portal/service/internal/db"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var _ core.AuthService = (*AuthServiceDefault)(nil)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID:      core.AUTH_SERVICE,
		Factory: NewAuthService,
		Depends: []string{core.USER_SERVICE, core.OTP_SERVICE},
	})
}

type AuthServiceDefault struct {
	*core.BaseComponent
	user core.UserService
	otp  core.OTPService
}

func NewAuthService() (core.Service, []core.ContextBuilderOption, error) {
	authService := &AuthServiceDefault{}
	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			authService.user = core.GetService[core.UserService](ctx, core.USER_SERVICE)
			authService.otp = core.GetService[core.OTPService](ctx, core.OTP_SERVICE)
			return nil
		}),
	)

	return authService, opts, nil
}

func (a AuthServiceDefault) ID() string {
	return core.AUTH_SERVICE
}

func (a AuthServiceDefault) LoginPassword(ctx context.Context, email string, password string, ip string, rememberMe bool) (string, *models.User, error) {
	ctx, span := core.TraceMethod(ctx, "AuthServiceDefault.LoginPassword")
	defer span.End()

	valid, user, err := a.ValidLoginByEmail(ctx, email, password)

	if err != nil {
		return "", nil, err
	}

	if !valid {
		return "", nil, core.NewAccountError(core.ErrKeyInvalidPassword, nil)
	}

	token, err := a.doLogin(ctx, user, ip, false, rememberMe)

	if err != nil {
		return "", nil, err
	}

	return token, user, nil
}

func (a AuthServiceDefault) LoginOTP(ctx context.Context, userId uint, code string, rememberMe bool) (string, error) {
	ctx, span := core.TraceMethod(ctx, "AuthServiceDefault.LoginOTP")
	defer span.End()

	valid, err := a.otp.OTPVerify(ctx, userId, code)

	if err != nil {
		return "", err
	}

	if !valid {
		return "", core.NewAccountError(core.ErrKeyInvalidOTPCode, nil)
	}

	var user models.User
	user.ID = userId

	token, err := a.doLogin(ctx, &user, "", false, rememberMe)
	if err != nil {
		return "", err
	}

	return token, nil
}

func (a AuthServiceDefault) LoginKeyIdentity(ctx context.Context, keyType string, key string, proof []byte, ip string, rememberMe bool) (string, error) {
	return a.LoginKeyIdentityWithContext(a.Context().WithRequestContext(ctx), keyType, key, proof, ip, rememberMe)
}

func (a AuthServiceDefault) LoginKeyIdentityWithContext(ctx core.Context, keyType string, key string, proof []byte, ip string, rememberMe bool) (string, error) {
	traceCtx, span := core.TraceMethod(ctx, "AuthServiceDefault.LoginKeyIdentityWithContext")
	defer span.End()

	// Require a registered handler — no handler means we can't normalize or verify
	handler, ok := core.GetKeyIdentityHandler(keyType)
	if !ok {
		return "", core.NewAccountError(core.ErrKeyInvalidLogin, fmt.Errorf("no handler registered for key type %q", keyType))
	}

	// Normalize the key before lookup
	normalized, err := handler.NormalizeKey(key)
	if err != nil {
		return "", core.NewAccountError(core.ErrKeyInvalidLogin, err)
	}
	key = normalized

	var model models.KeyIdentity
	var rowsAffected int64

	err = db.RetryableComponentTransaction(a, ctx, func(tx *gorm.DB) *gorm.DB {
		tx = tx.WithContext(traceCtx).Model(&models.KeyIdentity{}).Preload("User").
			Where(&models.KeyIdentity{Type: keyType, Key: key}).First(&model)
		rowsAffected = tx.RowsAffected
		return tx
	})

	if rowsAffected == 0 || err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", core.NewAccountError(core.ErrKeyInvalidLogin, err)
		}
		return "", dbHelper.HandleDBError(err)
	}

	// Verify proof of ownership before accepting the key
	// Default nil metadata to empty JSON for migrated legacy rows
	metadata := model.Metadata
	if metadata == nil {
		metadata = json.RawMessage(`{}`)
	}
	if err := handler.VerifyProof(ctx, key, metadata, proof); err != nil {
		return "", core.NewAccountError(core.ErrKeyInvalidLogin, err)
	}

	user := model.User

	token, err := a.doLogin(traceCtx, &user, ip, true, rememberMe)

	if err != nil {
		return "", err
	}

	return token, nil
}

func (a AuthServiceDefault) LoginID(ctx context.Context, id uint, ip string, rememberMe bool) (string, error) {
	ctx, span := core.TraceMethod(ctx, "AuthServiceDefault.LoginID")
	defer span.End()

	var user models.User
	var rowsAffected int64

	user.ID = id

	err := db.RetryableComponentTransaction(a, ctx, func(tx *gorm.DB) *gorm.DB {
		tx = tx.Model(&user).Where(&user).First(&user)
		rowsAffected = tx.RowsAffected
		return tx
	})

	if rowsAffected == 0 || err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", core.NewAccountError(core.ErrKeyInvalidLogin, err)
		}
		return "", dbHelper.HandleDBError(err)
	}

	token, err := a.doLogin(ctx, &user, ip, true, rememberMe)

	if err != nil {
		return "", err
	}

	return token, nil
}

func (a AuthServiceDefault) ValidLoginByUserObj(ctx context.Context, user *models.User, password string) bool {
	ctx, span := core.TraceMethod(ctx, "AuthServiceDefault.ValidLoginByUserObj")
	defer span.End()

	return a.validPassword(user, password)
}

func (a AuthServiceDefault) ValidLoginByEmail(ctx context.Context, email string, password string) (bool, *models.User, error) {
	ctx, span := core.TraceMethod(ctx, "AuthServiceDefault.ValidLoginByEmail")
	defer span.End()

	var user models.User
	var rowsAffected int64

	err := db.RetryableComponentTransaction(a, ctx, func(tx *gorm.DB) *gorm.DB {
		tx = tx.Model(&models.User{}).Where(&models.User{Email: email}).First(&user)
		rowsAffected = tx.RowsAffected
		return tx
	})

	if rowsAffected == 0 || err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil, core.NewAccountError(core.ErrKeyInvalidLogin, err)
		}
		return false, nil, dbHelper.HandleDBError(err)
	}

	valid := a.ValidLoginByUserObj(ctx, &user, password)

	if !valid {
		return false, nil, nil
	}

	return true, &user, nil
}

func (a AuthServiceDefault) ValidLoginByUserID(ctx context.Context, id uint, password string) (bool, *models.User, error) {
	ctx, span := core.TraceMethod(ctx, "AuthServiceDefault.ValidLoginByUserID")
	defer span.End()

	var user models.User
	var rowsAffected int64

	user.ID = id

	err := db.RetryableComponentTransaction(a, ctx, func(db *gorm.DB) *gorm.DB {
		tx := db.Model(&user).Where(&user).First(&user)
		rowsAffected = tx.RowsAffected
		return tx
	})

	if rowsAffected == 0 || err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil, core.NewAccountError(core.ErrKeyInvalidLogin, err)
		}
		return false, nil, dbHelper.HandleDBError(err)
	}

	valid := a.ValidLoginByUserObj(ctx, &user, password)

	if !valid {
		return false, nil, nil
	}

	return true, &user, nil
}
func (a AuthServiceDefault) doLogin(ctx context.Context, user *models.User, ip string, bypassSecurity bool, rememberMe bool) (string, error) {
	ctx, span := core.TraceMethod(ctx, "AuthServiceDefault.doLogin")
	defer span.End()

	purpose := jwt.PurposeLogin

	if user.OTPEnabled && !bypassSecurity {
		purpose = jwt.Purpose2FA
	}

	deletionPending, err := a.user.IsAccountPendingDeletion(ctx, user.ID)
	if err != nil {
		return "", err
	}

	if deletionPending {
		return "", core.NewAccountError(core.ErrKeyAccountPendingDeletion, nil)
	}

	dur := time.Hour * 24

	if rememberMe {
		dur = time.Hour * 24 * time.Duration(a.Config().Config().Core.Account.RememberMeTTL)
	}

	token, jwtErr := jwt.CreateToken(a.Config().Config().Core.Identity.PrivateKey(), a.Config().Config().Core.Domain, strconv.Itoa(int(user.ID)), purpose, dur)
	if jwtErr != nil {
		return "", core.NewAccountError(core.ErrKeyJWTGenerationFailed, jwtErr)
	}

	now := time.Now()

	err = a.user.UpdateAccountInfo(ctx, user.ID, map[string]any{"last_login_ip": ip, "last_login": &now})
	if err != nil {
		return "", err
	}

	return token, nil
}
func (a AuthServiceDefault) validPassword(user *models.User, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))

	return err == nil
}
