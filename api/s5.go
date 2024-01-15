package api

import (
	"git.lumeweb.com/LumeWeb/portal/interfaces"
	"git.lumeweb.com/LumeWeb/portal/protocols"
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
	registerProtocolSubdomain(portal, s5protocol.Node().Services().HTTP().GetHttpRouter(), "s5")

	return nil
}
