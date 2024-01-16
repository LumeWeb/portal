package api

import (
	"git.lumeweb.com/LumeWeb/portal/api/s5"
	"git.lumeweb.com/LumeWeb/portal/interfaces"
	"git.lumeweb.com/LumeWeb/portal/protocols"
	"go.sia.tech/jape"
)

var (
	_ interfaces.API = (*S5API)(nil)
)

type S5API struct {
}

func NewS5() *S5API {
	return &S5API{}
}

func (s S5API) Initialize(portal interfaces.Portal, protocol interfaces.Protocol) error {
	s5protocol := protocol.(*protocols.S5Protocol)
	s5http := s5.NewHttpHandler(portal)
	registerProtocolSubdomain(portal, s5protocol.Node().Services().HTTP().GetHttpRouter(getRoutes(s5http)), "s5")

	return nil
}

func getRoutes(h *s5.HttpHandler) map[string]jape.Handler {
	return map[string]jape.Handler{
		"POST /s5/upload":           h.SmallFileUpload,
		"GET /s5/account/register":  h.AccountRegisterChallenge,
		"POST /s5/account/register": h.AccountRegister,
		"GET /s5/account/login":     h.AccountLoginChallenge,
		"POST /s5/account/login":    h.AccountLogin,
	}
}
