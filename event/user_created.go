package event

import (
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
)

const (
	EVENT_USER_CREATED = "user.created"
)

func init() {
	core.RegisterEvent(EVENT_USER_CREATED, &UserCreatedEvent{})
}

type UserCreatedEvent struct {
	core.Event
}

func (e *UserCreatedEvent) SetUser(user *models.User) {
	e.Set("user", user)
}

func (e UserCreatedEvent) User() *models.User {
	return e.Get("user").(*models.User)
}

func FireUserCreatedEvent(ctx core.Context, user *models.User) error {
	return Fire[*UserCreatedEvent](ctx, EVENT_USER_CREATED, func(evt *UserCreatedEvent) error {
		evt.SetUser(user)
		return nil
	})
}
