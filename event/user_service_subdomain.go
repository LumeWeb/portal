package event

import (
	"context"

	"go.lumeweb.com/portal/core"
)

const EVENT_USER_SERVICE_SUBDOMAIN_SET = "user.subdomain.set"

type UserServiceSubdomainSetEvent struct {
	Subdomain string
	Ctx       context.Context
}

func NewUserServiceSubdomainSetEvent(subdomain string, eventCtx context.Context) *UserServiceSubdomainSetEvent {
	return &UserServiceSubdomainSetEvent{
		Subdomain: subdomain,
		Ctx:       eventCtx,
	}
}

// OnUserServiceSubdomainSet registers a handler to run when a user service subdomain is set.
// This is a convenience wrapper around Listen for the EVENT_USER_SERVICE_SUBDOMAIN_SET event.
func OnUserServiceSubdomainSet(ctx core.Context, handler func(string, context.Context) error, priority ...int) {
	core.Listen[UserServiceSubdomainSetEvent](ctx, EVENT_USER_SERVICE_SUBDOMAIN_SET, func(e *core.CoreEvent[UserServiceSubdomainSetEvent]) error {
		return handler(e.Data.Subdomain, e.Data.Ctx)
	}, priority...)
}
