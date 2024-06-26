package event

import (
	"fmt"
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
	evt, ok := ctx.Event().GetEvent(EVENT_USER_SUBDOMAIN_SET)

	if !ok {
		return fmt.Errorf("event %s not found", EVENT_USER_SUBDOMAIN_SET)
	}

	evt.(*UserSubdomainSetEvent).SetSubdomain(subdomain)

	err := ctx.Event().FireEvent(evt)
	if err != nil {
		return err
	}

	return nil
}
