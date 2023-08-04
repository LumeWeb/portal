package account

import (
	"errors"
	"git.lumeweb.com/LumeWeb/portal/db"
	"git.lumeweb.com/LumeWeb/portal/logger"
	"git.lumeweb.com/LumeWeb/portal/model"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

var (
	ErrEmailExists         = errors.New("Account with email already exists")
	ErrPubkeyExists        = errors.New("Account with pubkey already exists")
	ErrQueryingAcct        = errors.New("Error querying accounts")
	ErrFailedHashPassword  = errors.New("Failed to hash password")
	ErrFailedCreateAccount = errors.New("Failed to create account")
)

func Register(email string, password string, pubkey string) error {
	err := db.Get().Transaction(func(tx *gorm.DB) error {
		existingAccount := model.Account{}
		err := tx.Where("email = ?", email).First(&existingAccount).Error
		if err == nil {
			return ErrEmailExists
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if len(pubkey) > 0 {
			var count int64
			err := tx.Model(&model.Key{}).Where("pubkey = ?", pubkey).Count(&count).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if count > 0 {
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

		if err := tx.Create(&account).Error; err != nil {
			return err
		}

		if len(pubkey) > 0 {
			if err := tx.Create(&model.Key{Account: account, Pubkey: pubkey}).Error; err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		logger.Get().Error(ErrFailedCreateAccount.Error(), zap.Error(err))
		return err
	}

	return nil
}
