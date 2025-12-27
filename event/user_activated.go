package event

import (
	"context"

	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
)

const EVENT_USER_ACTIVATED = "user.activated"

type UserActivatedEvent struct {
	Ctx  context.Context
	User *models.User
}

func NewUserActivatedEvent(eventCtx context.Context, user *models.User) *UserActivatedEvent {
	if user == nil {
		panic("user cannot be nil in NewUserActivatedEvent")
	}
	return &UserActivatedEvent{
		Ctx:  eventCtx,
		User: user,
	}
}

// OnUserActivated registers a handler to run when a user is activated.
// This is a convenience wrapper around Listen for the EVENT_USER_ACTIVATED event.
func OnUserActivated(ctx core.Context, handler func(context.Context, *models.User) error, priority ...int) {
	core.Listen[UserActivatedEvent](ctx, EVENT_USER_ACTIVATED, func(e *core.CoreEvent[UserActivatedEvent]) error {
		return handler(e.Data.Ctx, e.Data.User)
	}, priority...)
}
