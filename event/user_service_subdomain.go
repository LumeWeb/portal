package event

import "go.lumeweb.com/portal/core"

const EVENT_USER_SERVICE_SUBDOMAIN_SET = "user.subdomain.set"

type UserServiceSubdomainSetEvent struct {
	Subdomain string
}

func NewUserServiceSubdomainSetEvent(subdomain string) *UserServiceSubdomainSetEvent {
	return &UserServiceSubdomainSetEvent{
		Subdomain: subdomain,
	}
}

// OnUserServiceSubdomainSet registers a handler to run when a user service subdomain is set.
// This is a convenience wrapper around Listen for the EVENT_USER_SERVICE_SUBDOMAIN_SET event.
func OnUserServiceSubdomainSet(ctx core.Context, handler func(string) error, priority ...int) {
	core.Listen[UserServiceSubdomainSetEvent](ctx, EVENT_USER_SERVICE_SUBDOMAIN_SET, func(e *core.CoreEvent[UserServiceSubdomainSetEvent]) error {
		return handler(e.Data.Subdomain)
	}, priority...)
}
