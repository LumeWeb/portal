package event

import (
	"go.lumeweb.com/portal/core"
)

const (
	EVENT_USER_SERVICE_SUBDOMAIN_SET = "user.subdomain.set"
)

func init() {
	core.RegisterEvent(EVENT_USER_SERVICE_SUBDOMAIN_SET, &UserServiceSubdomainSetEvent{})
}

type UserServiceSubdomainSetEvent struct {
	core.Event
}

func (e *UserServiceSubdomainSetEvent) SetSubdomain(subdomain string) {
	e.Set("subdomain", subdomain)
}

func (e UserServiceSubdomainSetEvent) Subdomain() string {
	return e.Get("subdomain").(string)
}

func FireUseServicerSubdomainSetEvent(ctx core.Context, subdomain string) error {
	return Fire[*UserServiceSubdomainSetEvent](ctx, EVENT_USER_SERVICE_SUBDOMAIN_SET, func(evt *UserServiceSubdomainSetEvent) error {
		evt.SetSubdomain(subdomain)
		return nil
	})
}
