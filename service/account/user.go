package account

import (
	"git.lumeweb.com/LumeWeb/portal/model"
	"github.com/kataras/iris/v12/context"
	"strconv"
)

type User struct {
	context.User
	account *model.Account
}

func (u User) GetID() (string, error) {
	return strconv.Itoa(int(u.account.ID)), nil
}

func NewUser(account *model.Account) *User {
	return &User{account: account}
}
