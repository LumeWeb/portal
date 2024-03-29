package controller

import (
	"git.lumeweb.com/LumeWeb/portal/controller/validators"
	"git.lumeweb.com/LumeWeb/portal/logger"
	"github.com/kataras/iris/v12"
	"go.uber.org/zap"
	"strconv"
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

func sendErrorCustom(ctx iris.Context, err error, customError error, irisError int) bool {
	if err != nil {
		if customError != nil {
			err = customError
		}
		ctx.StopWithError(irisError, err)
		return true
	}

	return false
}
func internalError(ctx iris.Context, err error) bool {
	return sendErrorCustom(ctx, err, nil, iris.StatusInternalServerError)
}
func internalErrorCustom(ctx iris.Context, err error, customError error) bool {
	return sendErrorCustom(ctx, err, customError, iris.StatusInternalServerError)
}
func sendError(ctx iris.Context, err error, irisError int) bool {
	return sendErrorCustom(ctx, err, nil, irisError)
}

type Controller struct {
	Ctx iris.Context
}

func (c Controller) respondJSON(data interface{}) {
	err := c.Ctx.JSON(data)
	if err != nil {
		logger.Get().Error("failed to generate response", zap.Error(err))
	}
}

func getCurrentUserId(ctx iris.Context) uint {
	usr := ctx.User()

	if usr == nil {
		return 0
	}

	sid, _ := usr.GetID()
	userID, _ := strconv.Atoi(sid)

	return uint(userID)
}
