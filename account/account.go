package account

import "git.lumeweb.com/LumeWeb/portal/interfaces"

var (
	_ interfaces.AccountService = (*AccountServiceImpl)(nil)
)

type AccountServiceImpl struct {
	portal interfaces.Portal
}

func NewAccountService(portal interfaces.Portal) interfaces.AccountService {
	return &AccountServiceImpl{portal: portal}
}
