package event

import "go.lumeweb.com/portal/db/models"

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
