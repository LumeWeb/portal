package api

import (
	"git.lumeweb.com/LumeWeb/portal/interfaces"
	"github.com/julienschmidt/httprouter"
	"go.sia.tech/jape"
)

type MiddlewareFunc func(jape.Handler) jape.Handler

func Init(router interfaces.APIRegistry) error {
	router.Register("s5", NewS5())
	return nil
}

func registerProtocolSubdomain(portal interfaces.Portal, mux *httprouter.Router, name string) {

	router := portal.ApiRegistry().Router()
	domain := portal.Config().GetString("core.domain")

	(*router)[name+"."+domain] = mux
}
