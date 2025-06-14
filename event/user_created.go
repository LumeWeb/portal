package event

import (
	"go.lumeweb.com/portal/db/models"
)

const EVENT_USER_CREATED = "user.created"

type UserCreatedEvent struct {
	User *models.User
}

func NewUserCreatedEvent(user *models.User) *UserCreatedEvent {
	if user == nil {
		panic("user cannot be nil in NewUserCreatedEvent")
	}
	return &UserCreatedEvent{
		User: user,
	}
}
