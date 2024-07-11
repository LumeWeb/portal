package event

import (
	"go.lumeweb.com/portal/core"
)

const (
	EVENT_USER_SUBDOMAIN_SET = "user.subdomain.set"
)

func init() {
	core.RegisterEvent(EVENT_USER_SUBDOMAIN_SET, &UserSubdomainSetEvent{})
}

type UserSubdomainSetEvent struct {
	core.Event
}

func (e *UserSubdomainSetEvent) SetSubdomain(subdomain string) {
	e.Set("subdomain", subdomain)
}

func (e UserSubdomainSetEvent) Subdomain() string {
	return e.Get("subdomain").(string)
}

func FireUserSubdomainSetEvent(ctx core.Context, subdomain string) error {
	evt, err := GetEvent(ctx, EVENT_USER_SUBDOMAIN_SET)
	if err != nil {
		return err
	}

	configEvt, err := AssertEventType[*UserSubdomainSetEvent](evt, EVENT_USER_SUBDOMAIN_SET)
	if err != nil {
		return err
	}

	configEvt.SetSubdomain(subdomain)

	err = ctx.Event().FireEvent(configEvt)
	if err != nil {
		return err
	}

	return nil
}
