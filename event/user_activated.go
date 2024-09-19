package event

import (
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
)

const (
	EVENT_USER_ACTIVATED = "user.activated"
)

func init() {
	core.RegisterEvent(EVENT_USER_ACTIVATED, &UserActivatedEvent{})
}

type UserActivatedEvent struct {
	core.Event
}

func (e *UserActivatedEvent) SetUser(user *models.User) {
	e.Set("user", user)
}

func (e UserActivatedEvent) User() *models.User {
	return e.Get("user").(*models.User)
}

func FireUserActivatedEvent(ctx core.Context, user *models.User) error {
	return Fire[*UserActivatedEvent](ctx, EVENT_USER_ACTIVATED, func(evt *UserActivatedEvent) error {
		evt.SetUser(user)
		return nil
	})
}
