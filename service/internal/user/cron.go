package user

import (
	"go.lumeweb.com/portal/core"
	"go.uber.org/zap"
)

const CronTaskProcessAccountDeletionRequestsName = "ProcessAccountDeletionRequests"

func CronTaskProcessAccountDeletionRequests(_ *core.CronTaskNoArgs, ctx core.Context) error {
	logger := ctx.Logger()
	userService := core.GetService[core.UserService](ctx, core.USER_SERVICE)
	pinService := core.GetService[core.PinService](ctx, core.PIN_SERVICE)
	requestService := core.GetService[core.RequestService](ctx, core.REQUEST_SERVICE)

	// Get all deletion requests
	requests, err := userService.GetAccountsPendingDeletion()
	if err != nil {
		logger.Error("Failed to get account deletion requests", zap.Error(err))
		return err
	}

	for _, request := range requests {
		uploadRequests, err := requestService.ListRequestsByUser(ctx, request.ID, core.RequestFilter{})
		if err != nil {
			return err
		}

		for _, uploadRequest := range uploadRequests {
			err := requestService.DeleteRequest(ctx, uploadRequest.ID)
			if err != nil {
				logger.Error("Failed to delete request", zap.Uint("request_id", uploadRequest.ID), zap.Error(err))
				continue
			}
		}

		pins, err := pinService.AllAccountPins(request.ID)
		if err != nil {
			logger.Error("Failed to get account pins", zap.Uint("user_id", request.ID), zap.Error(err))
			continue
		}

		for _, pin := range pins {
			err = pinService.DeletePin(ctx, pin.ID)
			if err != nil {
				logger.Error("Failed to delete pin", zap.Uint("pin_id", pin.ID), zap.Error(err))
				continue
			}
		}

		err = userService.DeleteAccount(request.ID)
		if err != nil {
			logger.Error("Failed to delete account", zap.Uint("user_id", request.ID), zap.Error(err))
			continue
		}
	}

	return nil
}
