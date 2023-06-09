package account

import (
	"errors"
	"git.lumeweb.com/LumeWeb/portal/db"
	"git.lumeweb.com/LumeWeb/portal/logger"
	"git.lumeweb.com/LumeWeb/portal/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"strings"
)

var (
	ErrEmailExists         = errors.New("Account with email already exists")
	ErrPubkeyExists        = errors.New("Account with pubkey already exists")
	ErrQueryingAcct        = errors.New("Error querying accounts")
	ErrFailedHashPassword  = errors.New("Failed to hash password")
	ErrFailedCreateAccount = errors.New("Failed to create account")
)

func Register(email string, password string, pubkey string) error {
	// Check if an account with the same email address already exists.
	existingAccount := model.Account{}
	err := db.Get().Where("email = ?", email).First(&existingAccount).Error
	if err == nil {
		logger.Get().Debug(ErrEmailExists.Error(), zap.Error(err), zap.String("email", email))
		// An account with the same email address already exists.
		// Return an error response to the client.
		return ErrEmailExists
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		logger.Get().Error(ErrQueryingAcct.Error(), zap.Error(err))
		return ErrQueryingAcct
	}

	if len(pubkey) > 0 {
		pubkey = strings.ToLower(pubkey)
		var count int64
		err := db.Get().Model(&model.Key{}).Where("pubkey = ?", pubkey).Count(&count).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			logger.Get().Error(ErrQueryingAcct.Error(), zap.Error(err), zap.String("pubkey", pubkey))
			return ErrQueryingAcct
		}
		if count > 0 {
			logger.Get().Debug(ErrPubkeyExists.Error(), zap.Error(err), zap.String("pubkey", pubkey))
			// An account with the same pubkey already exists.
			// Return an error response to the client.
			return ErrPubkeyExists
		}

	}

	// Create a new Account model with the provided email and hashed password.
	account := model.Account{
		Email: email,
	}

	// Hash the password before saving it to the database.
	if len(password) > 0 {
		hashedPassword, err := hashPassword(password)
		if err != nil {
			return err
		}

		account.Password = &hashedPassword
	}

	err = db.Get().Transaction(func(tx *gorm.DB) error {
		// do some database operations in the transaction (use 'tx' from this point, not 'db')
		if err := tx.Create(&account).Error; err != nil {
			return err
		}

		if len(pubkey) > 0 {
			if err := tx.Create(&model.Key{Account: account, Pubkey: pubkey}).Error; err != nil {
				return err
			}
		}

		// return nil will commit the whole transaction
		return nil
	})
	if err != nil {
		logger.Get().Error(ErrFailedCreateAccount.Error(), zap.Error(err))
		return ErrFailedCreateAccount
	}

	return nil
}
