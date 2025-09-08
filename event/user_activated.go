package event

import (
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
)

const EVENT_USER_ACTIVATED = "user.activated"

type UserActivatedEvent struct {
	User *models.User
}

func NewUserActivatedEvent(user *models.User) *UserActivatedEvent {
	if user == nil {
		panic("user cannot be nil in NewUserActivatedEvent")
	}
	return &UserActivatedEvent{
		User: user,
	}
}

// OnUserActivated registers a handler to run when a user is activated.
// This is a convenience wrapper around Listen for the EVENT_USER_ACTIVATED event.
func OnUserActivated(ctx core.Context, handler func(*models.User) error, priority ...int) {
	core.Listen[UserActivatedEvent](ctx, EVENT_USER_ACTIVATED, func(e *core.CoreEvent[UserActivatedEvent]) error {
		return handler(e.Data.User)
	}, priority...)
}
