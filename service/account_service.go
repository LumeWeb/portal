package service

import (
	"errors"
	"git.lumeweb.com/LumeWeb/portal/db"
	"git.lumeweb.com/LumeWeb/portal/model"
	"github.com/kataras/iris/v12"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type AccountService struct {
	ctx iris.Context
}

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Pubkey   []byte `json:"pubkey"`
}

func hashPassword(password string) (string, error) {

	// Generate a new bcrypt hash from the provided password.
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}

	// Convert the hashed password to a string and return it.
	return string(hashedPassword), nil
}

func (a *AccountService) PostRegister() {
	var r RegisterRequest

	if err := a.ctx.ReadJSON(&r); err != nil {
		a.ctx.StopWithError(iris.StatusBadRequest, err)
		return
	}

	// Hash the password before saving it to the database.
	hashedPassword, err := hashPassword(r.Password)
	if err != nil {
		a.ctx.StopWithError(iris.StatusInternalServerError, err)
		return
	}

	// Check if an account with the same email address already exists.
	existingAccount := model.Account{}
	err = db.Get().Where("email = ?", r.Email).First(&existingAccount).Error
	if err == nil {
		// An account with the same email address already exists.
		// Return an error response to the client.
		a.ctx.StopWithError(iris.StatusConflict, errors.New("an account with this email address already exists"))
		return
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		// An unexpected error occurred while querying the database.
		// Return an error response to the client.
		a.ctx.StopWithError(iris.StatusInternalServerError, err)
		return
	}

	// Create a new Account model with the provided email and hashed password.
	account := model.Account{
		Email:    r.Email,
		Password: &hashedPassword,
	}

	// Save the new account to the database.
	err = db.Get().Create(&account).Error
	if err != nil {
		a.ctx.StopWithError(iris.StatusInternalServerError, err)
		return
	}

	// Return a success response to the client.
	a.ctx.StatusCode(iris.StatusCreated)
}
