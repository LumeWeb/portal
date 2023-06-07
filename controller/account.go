package controller

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"git.lumeweb.com/LumeWeb/portal/db"
	"git.lumeweb.com/LumeWeb/portal/logger"
	"git.lumeweb.com/LumeWeb/portal/model"
	"github.com/go-ozzo/ozzo-validation/v4"
	"github.com/go-ozzo/ozzo-validation/v4/is"
	"github.com/kataras/iris/v12"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"strings"
)

type AccountController struct {
	Ctx iris.Context
}

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Pubkey   string `json:"pubkey"`
}

func checkPubkey(value interface{}) error {
	p, _ := value.(string)
	pubkeyBytes, err := hex.DecodeString(p)
	if err != nil {
		return err
	}

	if len(pubkeyBytes) != ed25519.PublicKeySize {
		return errors.New(fmt.Sprintf("pubkey must be %d bytes in hexadecimal format", ed25519.PublicKeySize))
	}

	return nil
}

func (r RegisterRequest) Validate() error {
	return validation.ValidateStruct(&r,
		validation.Field(&r.Email, validation.Required, is.EmailFormat),
		validation.Field(&r.Pubkey, validation.When(len(r.Password) == 0, validation.Required, validation.By(checkPubkey))),
		validation.Field(&r.Password, validation.When(len(r.Pubkey) == 0, validation.Required)),
	)
}

func hashPassword(password string) (string, error) {
	// Generate a new bcrypt hash from the provided password.
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		logger.Get().Error("failed to hash password", zap.Error(err))
		return "", err
	}

	// Convert the hashed password to a string and return it.
	return string(hashedPassword), nil
}

func (a *AccountController) PostRegister() {
	var r RegisterRequest

	if err := a.Ctx.ReadJSON(&r); err != nil {
		logger.Get().Debug("failed to parse request", zap.Error(err))
		a.Ctx.StopWithError(iris.StatusBadRequest, err)
		return
	}

	// Check if an account with the same email address already exists.
	existingAccount := model.Account{}
	err := db.Get().Where("email = ?", r.Email).First(&existingAccount).Error
	if err == nil {
		logger.Get().Debug("account with email already exists", zap.Error(err), zap.String("email", r.Email))
		// An account with the same email address already exists.
		// Return an error response to the client.
		a.Ctx.StopWithError(iris.StatusConflict, errors.New("an account with this email address already exists"))
		return
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		logger.Get().Error("error querying accounts", zap.Error(err), zap.String("email", r.Email))
		// An unexpected error occurred while querying the database.
		// Return an error response to the client.
		a.Ctx.StopWithError(iris.StatusInternalServerError, err)
		return
	}

	// Create a new Account model with the provided email and hashed password.
	account := model.Account{
		Email: r.Email,
	}

	// Hash the password before saving it to the database.
	if len(r.Password) > 0 {
		hashedPassword, err := hashPassword(r.Password)
		if err != nil {
			a.Ctx.StopWithError(iris.StatusInternalServerError, err)
			return
		}

		account.Password = &hashedPassword
	}

	err = db.Get().Transaction(func(tx *gorm.DB) error {
		// do some database operations in the transaction (use 'tx' from this point, not 'db')
		if err := tx.Create(&account).Error; err != nil {
			return err
		}

		if len(r.Pubkey) > 0 {
			if err := tx.Create(&model.Key{Account: account, Pubkey: strings.ToLower(r.Pubkey)}).Error; err != nil {
				return err
			}
		}

		// return nil will commit the whole transaction
		return nil
	})
	if err != nil {
		logger.Get().Error("failed to create account", zap.Error(err))
		a.Ctx.StopWithError(iris.StatusInternalServerError, err)
		return
	}

	// Return a success response to the client.
	a.Ctx.StatusCode(iris.StatusCreated)
}
