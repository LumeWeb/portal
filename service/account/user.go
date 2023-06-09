package account

import (
	"git.lumeweb.com/LumeWeb/portal/model"
	"strconv"
)

type User struct {
	account *model.Account
}

func (u User) GetID() (string, error) {
	return strconv.Itoa(int(u.account.ID)), nil
}

func NewUser(account *model.Account) *User {
	return &User{account: account}
}
