package controller

import (
	"git.lumeweb.com/LumeWeb/portal/controller/validators"
	"git.lumeweb.com/LumeWeb/portal/logger"
	"github.com/kataras/iris/v12"
	"go.uber.org/zap"
)

func tryParseRequest(r interface{}, ctx iris.Context) (interface{}, bool) {
	v, ok := r.(validators.Validatable)

	if !ok {
		return r, true
	}

	var d map[string]interface{}

	// Read the logout request from the client.
	if err := ctx.ReadJSON(&d); err != nil {
		logger.Get().Debug("failed to parse request", zap.Error(err))
		ctx.StopWithError(iris.StatusBadRequest, err)
		return nil, false
	}

	data, err := v.Import(d)
	if err != nil {
		logger.Get().Debug("failed to parse request", zap.Error(err))
		ctx.StopWithError(iris.StatusBadRequest, err)
		return nil, false
	}

	if err := data.Validate(); err != nil {
		logger.Get().Debug("failed to parse request", zap.Error(err))
		ctx.StopWithError(iris.StatusBadRequest, err)
		return nil, false
	}

	return data, true
}
