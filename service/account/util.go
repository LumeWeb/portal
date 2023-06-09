package account

import (
	"git.lumeweb.com/LumeWeb/portal/logger"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

func hashPassword(password string) (string, error) {
	// Generate a new bcrypt hash from the provided password.
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		logger.Get().Error(ErrFailedHashPassword.Error(), zap.Error(err))
		return "", ErrFailedHashPassword
	}

	// Convert the hashed password to a string and return it.
	return string(hashedPassword), nil
}
