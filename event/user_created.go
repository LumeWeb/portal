package event

import (
	"context"

	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
)

const EVENT_USER_CREATED = "user.created"

type UserCreatedEvent struct {
	User *models.User
	Ctx  context.Context
}

func NewUserCreatedEvent(user *models.User, eventCtx context.Context) *UserCreatedEvent {
	if user == nil {
		panic("user cannot be nil in NewUserCreatedEvent")
	}
	return &UserCreatedEvent{
		User: user,
		Ctx:  eventCtx,
	}
}

// OnUserCreated registers a handler to run when a user is created.
// This is a convenience wrapper around Listen for the EVENT_USER_CREATED event.
func OnUserCreated(ctx core.Context, handler func(*models.User, context.Context) error, priority ...int) {
	core.Listen[UserCreatedEvent](ctx, EVENT_USER_CREATED, func(e *core.CoreEvent[UserCreatedEvent]) error {
		return handler(e.Data.User, e.Data.Ctx)
	}, priority...)
}
